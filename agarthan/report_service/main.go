package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/segmentio/kafka-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"middleware"
	"models"
)

const (
	reportQueueName            = "report_requests"
	reportResponseQueueName    = "report_response"
	reportResponseMaxPriority  = 10
	reportResponseHighPriority = 10
	maxReconnectAttempts       = 3
	reconnectBackoff           = 500 * time.Millisecond
)

var DB *gorm.DB
var RabbitConn *amqp.Connection
var RabbitChannel *amqp.Channel
var KafkaWriter *kafka.Writer

func userIDFromLocals(c *fiber.Ctx) (uint, error) {
	raw := c.Locals("user_id")
	switch v := raw.(type) {
	case float64:
		if v <= 0 {
			return 0, errors.New("invalid user id")
		}
		return uint(v), nil
	case float32:
		if v <= 0 {
			return 0, errors.New("invalid user id")
		}
		return uint(v), nil
	case int:
		if v <= 0 {
			return 0, errors.New("invalid user id")
		}
		return uint(v), nil
	case int64:
		if v <= 0 {
			return 0, errors.New("invalid user id")
		}
		return uint(v), nil
	case uint:
		if v == 0 {
			return 0, errors.New("invalid user id")
		}
		return v, nil
	case uint64:
		if v == 0 {
			return 0, errors.New("invalid user id")
		}
		return uint(v), nil
	case string:
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil || parsed == 0 {
			return 0, errors.New("invalid user id")
		}
		return uint(parsed), nil
	case json.Number:
		parsed, err := v.Int64()
		if err != nil || parsed <= 0 {
			return 0, errors.New("invalid user id")
		}
		return uint(parsed), nil
	default:
		return 0, errors.New("invalid user id")
	}
}

func ConnectDB() {
	var err error

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=postgres dbname=agarthan port=5432 sslmode=disable"
	}

	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	err = DB.AutoMigrate(
		&models.Report{},
		&models.ReportCategory{},
		&models.ReportMedia{},
		&models.ReportAssignment{},
		&models.ReportResponse{},
		&models.Upvote{},
		&models.Escalation{},
	)
	if err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	log.Println("Database connected")
}

func ConnectRabbit() {
	var err error

	amqpURL := os.Getenv("RABBITMQ_URL")
	if amqpURL == "" {
		amqpURL = "amqp://guest:guest@localhost:5672/"
	}

	var lastErr error
	for attempt := 1; attempt <= maxReconnectAttempts; attempt++ {
		RabbitConn, err = amqp.Dial(amqpURL)
		if err == nil {
			RabbitChannel, err = RabbitConn.Channel()
		}
		if err == nil {
			_, err = RabbitChannel.QueueDeclare(
				reportQueueName,
				true,
				false,
				false,
				false,
				nil,
			)
		}
		if err == nil {
			_, err = RabbitChannel.QueueDeclare(
				reportResponseQueueName,
				true,
				false,
				false,
				false,
				amqp.Table{"x-max-priority": reportResponseMaxPriority},
			)
		}
		if err == nil {
			log.Println("RabbitMQ connected")
			return
		}

		if RabbitChannel != nil {
			_ = RabbitChannel.Close()
			RabbitChannel = nil
		}
		if RabbitConn != nil {
			_ = RabbitConn.Close()
			RabbitConn = nil
		}

		lastErr = err
		log.Printf("RabbitMQ connection failed (attempt %d/%d): %v", attempt, maxReconnectAttempts, err)
		if attempt < maxReconnectAttempts {
			time.Sleep(time.Duration(attempt) * reconnectBackoff)
		}
	}

	log.Fatal("Failed to connect to RabbitMQ after retries:", lastErr)
}

func ConnectKafka() {
	kafkaBroker := os.Getenv("KAFKA_BROKER")
	if kafkaBroker == "" {
		kafkaBroker = "localhost:9092"
	}

	// Generic writer for all report events
	KafkaWriter = &kafka.Writer{
		Addr:                   kafka.TCP(kafkaBroker),
		Balancer:               &kafka.LeastBytes{},
		AllowAutoTopicCreation: true,
		Async:                  false, // Synchronous for reliability
	}

	log.Println("Kafka writer initialized")
}

// PublishKafkaEvent publishes an event to a Kafka topic
func PublishKafkaEvent(topic string, key string, value []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := KafkaWriter.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: value,
	})

	if err != nil {
		log.Printf("Failed to publish to topic %s: %v", topic, err)
		return err
	}

	log.Printf("Published event to topic %s with key %s", topic, key)
	return nil
}

type reportResponsePayload struct {
	ReportID uint   `json:"report_id"`
	Message  string `json:"message"`
}

type assignedReportSummary struct {
	ReportID          uint      `json:"report_id"`
	ReportTitle       string    `json:"report_title"`
	ReportDescription string    `json:"report_description"`
	PosterName        string    `json:"poster_name"`
	IsPublic          bool      `json:"is_public"`
	Severity          string    `json:"severity"`
	Location          string    `json:"location"`
	Status            string    `json:"status"`
	AssignedAt        time.Time `json:"assigned_at"`
}

func CreateReport(c *fiber.Ctx) error {
	report := new(models.Report)

	if err := c.BodyParser(report); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}

	report.UserID = userID
	report.Status = "pending" // Set default status

	if err := DB.Omit("User", "ReportCategory", "AssignedUser").Create(&report).Error; err != nil {
		log.Printf("Failed to create report: %v\n", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create report"})
	}

	log.Printf("Report created successfully with ID: %d\n", report.ReportID)

	go func() {
		requestID := uuid.NewString()
		event := map[string]interface{}{
			"request_id": requestID,
			"report_id":  report.ReportID,
			"user_id":    report.UserID,
			"status":     "created",
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
		}

		eventJSON, err := json.Marshal(event)
		if err != nil {
			log.Printf("Failed to marshal submission event: %v\n", err)
			return
		}

		if err := PublishKafkaEvent("submission.events", fmt.Sprintf("%d", report.ReportID), eventJSON); err != nil {
			log.Printf("Failed to publish submission event: %v\n", err)
		} else {
			log.Printf("Published submission event for report %d\n", report.ReportID)
		}
	}()

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"status":    "created",
		"report_id": report.ReportID,
		"message":   "Report created successfully",
	})
}

func CreateReportResponse(c *fiber.Ctx) error {
	var payload reportResponsePayload

	if err := c.BodyParser(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	payload.Message = strings.TrimSpace(payload.Message)
	if payload.ReportID == 0 || payload.Message == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "report_id and message are required"})
	}

	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}

	var report models.Report
	if err := DB.Where("report_id = ?", payload.ReportID).First(&report).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "report not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	assignedToUser := false
	if err := DB.Where("report_id = ? AND assigned_to = ?", payload.ReportID, userID).First(&models.ReportAssignment{}).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	} else {
		assignedToUser = true
	}

	if report.UserID != userID && !assignedToUser {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}

	isStaff := false
	var user models.User
	if err := DB.Select("user_id", "role_id", "department_id").Where("user_id = ?", userID).First(&user).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	} else if (user.DepartmentID != nil && *user.DepartmentID != 0) || user.RoleID != 0 {
		isStaff = true
	}

	priority := uint8(0)
	if isStaff {
		priority = uint8(reportResponseHighPriority)
	}

	requestID := uuid.NewString()
	response := models.ReportResponse{
		Message:   payload.Message,
		CreatedAt: time.Now(),
		CreatedBy: userID,
		ReportID:  payload.ReportID,
	}

	message := models.ReportResponseRequestMessage{
		RequestID: requestID,
		Response:  response,
	}

	body, err := json.Marshal(message)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	if err := RabbitChannel.Publish(
		"",
		reportResponseQueueName,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Priority:     priority,
			Body:         body,
		},
	); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"status":     "queued",
		"request_id": requestID,
	})
}

func GetReports(c *fiber.Ctx) error {
	var reports []models.Report

	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}

	// Get user role and department for RBAC
	userRole, ok := c.Locals("role").(string)
	if !ok {
		userRole = ""
	}

	var user models.User
	if err := DB.Preload("Role").Preload("Department").First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch user"})
	}

	// RBAC: Government users can only see reports in their department
	if userRole == "government" || user.Role.Name == "government" {
		if user.DepartmentID == nil || *user.DepartmentID == 0 {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "government user must have a department assigned"})
		}
		
		// Get reports that are public AND belong to their department
		if err := DB.Joins("JOIN report_categories ON reports.report_categories_id = report_categories.report_categories_id").
			Where("(reports.is_public = ? OR reports.user_id = ?) AND report_categories.department_id = ?", 
				true, userID, *user.DepartmentID).
			Find(&reports).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	} else {
		// Admin and regular users can see all public reports or their own
		if err := DB.Where("is_public = ? OR user_id = ?", true, userID).Find(&reports).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	}
	
	return c.JSON(reports)
}

func GetAssignedReports(c *fiber.Ctx) error {
	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}

	items := make([]assignedReportSummary, 0)
	query := DB.Table("report_assignments").
		Select(strings.Join([]string{
			"report_assignments.report_id",
			"report_assignments.status",
			"report_assignments.assigned_at",
			"reports.title as report_title",
			"reports.description as report_description",
			"reports.is_public as is_public",
			"reports.severity as severity",
			"reports.location as location",
			"users.name as poster_name",
		}, ", ")).
		Joins("JOIN reports ON reports.report_id = report_assignments.report_id").
		Joins("LEFT JOIN users ON users.user_id = reports.user_id").
		Where("report_assignments.assigned_to = ?", userID).
		Order("report_assignments.assigned_at desc, report_assignments.report_id desc")

	if err := query.Scan(&items).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	for i := range items {
		if strings.TrimSpace(items[i].PosterName) == "" {
			items[i].PosterName = "Unknown"
		}
	}

	return c.JSON(fiber.Map{
		"count": len(items),
		"items": items,
	})
}

func GetReportStatus(c *fiber.Ctx) error {
	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}

	reportIDRaw, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || reportIDRaw == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid report id"})
	}
	reportID := uint(reportIDRaw)

	var report models.Report
	if err := DB.Where("report_id = ?", reportID).First(&report).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "report not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	var assignment models.ReportAssignment
	assignedToUser := false
	if err := DB.Where("report_id = ? AND assigned_to = ?", reportID, userID).First(&assignment).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	} else {
		assignedToUser = true
	}

	if !report.IsPublic && report.UserID != userID && !assignedToUser {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}

	var response models.ReportResponse
	if err := DB.Where("report_id = ?", reportID).
		Order("created_at desc, report_response_id desc").
		First(&response).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(fiber.Map{
				"report_id":       reportID,
				"status":          "pending",
				"latest_response": nil,
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	status := strings.TrimSpace(response.JobStatus)
	if status == "" {
		status = strings.TrimSpace(response.Message)
	}
	if status == "" {
		status = "unknown"
	}

	return c.JSON(fiber.Map{
		"report_id":       reportID,
		"status":          status,
		"latest_response": response,
	})
}

func GetReportDetails(c *fiber.Ctx) error {
	var report models.Report

	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}

	reportIDRaw, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || reportIDRaw == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid report id"})
	}
	reportID := uint(reportIDRaw)

	if err := DB.Where("report_id = ?", reportID).First(&report).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "report not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	var assignment models.ReportAssignment
	assignedToUser := false
	if err := DB.Where("report_id = ? AND assigned_to = ?", reportID, userID).First(&assignment).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	} else {
		assignedToUser = true
	}

	// Allow access to public reports, the owner, or the assigned user.
	if !report.IsPublic && report.UserID != userID && !assignedToUser {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}

	return c.JSON(report)
}

// CreateUpvote allows citizens to upvote public reports
func CreateUpvote(c *fiber.Ctx) error {
	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}

	reportID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || reportID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid report id"})
	}

	// Check if report exists and is public
	var report models.Report
	if err := DB.First(&report, reportID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "report not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	if !report.IsPublic {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "cannot upvote private reports"})
	}

	// Check if user already upvoted
	var existingUpvote models.Upvote
	if err := DB.Where("user_id = ? AND report_id = ?", userID, reportID).First(&existingUpvote).Error; err == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "you have already upvoted this report"})
	}

	// Create upvote
	upvote := models.Upvote{
		UserID:    userID,
		ReportID:  uint(reportID),
		CreatedAt: time.Now(),
	}

	if err := DB.Create(&upvote).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Get total upvote count
	var count int64
	DB.Model(&models.Upvote{}).Where("report_id = ?", reportID).Count(&count)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":       "upvote created successfully",
		"upvote_count": count,
	})
}

// DeleteUpvote allows citizens to remove their upvote
func DeleteUpvote(c *fiber.Ctx) error {
	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}

	reportID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || reportID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid report id"})
	}

	result := DB.Where("user_id = ? AND report_id = ?", userID, reportID).Delete(&models.Upvote{})
	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": result.Error.Error()})
	}

	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "upvote not found"})
	}

	// Get total upvote count
	var count int64
	DB.Model(&models.Upvote{}).Where("report_id = ?", reportID).Count(&count)

	return c.JSON(fiber.Map{
		"message":       "upvote removed successfully",
		"upvote_count": count,
	})
}

// GetAnalytics returns analytics data with RBAC filtering
func GetAnalytics(c *fiber.Ctx) error {
	log.Println("[GetAnalytics] START")
	userRole, ok := c.Locals("role").(string)
	if !ok {
		userRole = ""
	}
	log.Printf("[GetAnalytics] User role from locals: %s\n", userRole)

	userID, err := userIDFromLocals(c)
	if err != nil {
		log.Printf("[GetAnalytics] ERROR: Failed to get user ID: %v\n", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}
	log.Printf("[GetAnalytics] User ID: %d\n", userID)

	var user models.User
	if err := DB.Preload("Role").First(&user, userID).Error; err != nil {
		log.Printf("[GetAnalytics] ERROR: Failed to fetch user: %v\n", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to fetch user"})
	}
	log.Printf("[GetAnalytics] User loaded: Role=%s, DepartmentID=%v\n", user.Role.Name, user.DepartmentID)

	departmentID := c.QueryInt("department_id", 0)
	log.Printf("[GetAnalytics] Department ID from query: %d\n", departmentID)

	// RBAC: government users can only see their department
	if userRole == "government" || user.Role.Name == "government" {
		if user.DepartmentID != nil {
			departmentID = int(*user.DepartmentID)
			log.Printf("[GetAnalytics] Government user - using their department: %d\n", departmentID)
		}
	}
	// admin can see all or specific department

	// Reports by category
	type CategoryCount struct {
		Category string `json:"category"`
		Count    int64  `json:"count"`
	}
	var byCategory []CategoryCount
	query := DB.Model(&models.Report{}).
		Select("report_categories.name as category, COUNT(*) as count").
		Joins("JOIN report_categories ON reports.report_categories_id = report_categories.report_categories_id")

	if departmentID > 0 {
		query = query.Where("report_categories.department_id = ?", departmentID)
	}
	query.Group("report_categories.name").Scan(&byCategory)

	// Reports by severity
	type SeverityCount struct {
		Severity string `json:"severity"`
		Count    int64  `json:"count"`
	}
	var bySeverity []SeverityCount
	severityQuery := DB.Model(&models.Report{}).
		Select("severity, COUNT(*) as count")

	if departmentID > 0 {
		severityQuery = severityQuery.
			Joins("JOIN report_categories ON reports.report_categories_id = report_categories.report_categories_id").
			Where("report_categories.department_id = ?", departmentID)
	}
	severityQuery.Group("severity").Scan(&bySeverity)

	// KPIs (placeholder - currently only total count as status field doesn't exist)
	var total int64
	countQuery := DB.Model(&models.Report{})
	if departmentID > 0 {
		countQuery = countQuery.
			Joins("JOIN report_categories ON reports.report_categories_id = report_categories.report_categories_id").
			Where("report_categories.department_id = ?", departmentID)
	}
	countQuery.Count(&total)

	// Recent activity
	type Activity struct {
		Title     string    `json:"title"`
		Timestamp time.Time `json:"timestamp"`
		Type      string    `json:"type"`
	}
	var recentActivity []Activity
	activityQuery := DB.Model(&models.Report{}).
		Select("title, created_at as timestamp, 'report_created' as type").
		Order("created_at DESC").
		Limit(10)

	if departmentID > 0 {
		activityQuery = activityQuery.
			Joins("JOIN report_categories ON reports.report_categories_id = report_categories.report_categories_id").
			Where("report_categories.department_id = ?", departmentID)
	}
	activityQuery.Scan(&recentActivity)

	// Now with Status field, calculate actual KPIs
	var completed, pending, inProgress int64
	kpiQuery := DB.Model(&models.Report{})
	if departmentID > 0 {
		kpiQuery = kpiQuery.
			Joins("JOIN report_categories ON reports.report_categories_id = report_categories.report_categories_id").
			Where("report_categories.department_id = ?", departmentID)
	}
	kpiQuery.Where("status = ?", "resolved").Count(&completed)

	kpiQuery2 := DB.Model(&models.Report{})
	if departmentID > 0 {
		kpiQuery2 = kpiQuery2.
			Joins("JOIN report_categories ON reports.report_categories_id = report_categories.report_categories_id").
			Where("report_categories.department_id = ?", departmentID)
	}
	kpiQuery2.Where("status = ?", "pending").Count(&pending)

	kpiQuery3 := DB.Model(&models.Report{})
	if departmentID > 0 {
		kpiQuery3 = kpiQuery3.
			Joins("JOIN report_categories ON reports.report_categories_id = report_categories.report_categories_id").
			Where("report_categories.department_id = ?", departmentID)
	}
	kpiQuery3.Where("status = ?", "in_progress").Count(&inProgress)

	log.Printf("[GetAnalytics] SUCCESS - Returning analytics data\n")
	log.Printf("[GetAnalytics] - Categories: %d, Severity: %d, Total: %d\n", len(byCategory), len(bySeverity), total)

	return c.JSON(fiber.Map{
		"reportsByCategory": byCategory,
		"reportsBySeverity": bySeverity,
		"kpis": fiber.Map{
			"total":      total,
			"completed":  completed,
			"pending":    pending,
			"inProgress": inProgress,
		},
		"recentActivity": recentActivity,
	})
}

// UpdateReportStatus updates report status and sends notification to reporter (citizen)
// Only government officials and admins can update status
func UpdateReportStatus(c *fiber.Ctx) error {
	// RBAC: Only government and admin can update status
	userRole, ok := c.Locals("role").(string)
	log.Printf("DEBUG: UpdateReportStatus - role from locals: '%v', ok: %v", userRole, ok)
	if !ok || (userRole != "government" && userRole != "admin") {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "only government officials and admins can update report status"})
	}

	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}

	reportID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || reportID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid report id"})
	}

	var input struct {
		Status string `json:"status"` // pending, in_progress, resolved, rejected
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Validate status
	validStatuses := map[string]bool{"pending": true, "in_progress": true, "resolved": true, "rejected": true}
	if !validStatuses[input.Status] {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid status. Must be pending, in_progress, resolved, or rejected"})
	}

	// Fetch report
	var report models.Report
	if err := DB.First(&report, reportID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "report not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	oldStatus := report.Status
	report.Status = input.Status
	report.UpdatedAt = time.Now()

	if err := DB.Save(&report).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Publish Kafka event for notification
	event := models.ReportStatusChangeEvent{
		ReportID:    report.ReportID,
		OldStatus:   oldStatus,
		NewStatus:   input.Status,
		ChangedBy:   userID,
		UserID:      report.UserID,
		IsAnonymous: report.IsAnonymous,
		Title:       report.Title,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	eventJSON, _ := json.Marshal(event)
	if err := PublishKafkaEvent("report.status_change", strconv.FormatUint(uint64(report.ReportID), 10), eventJSON); err != nil {
		log.Printf("Failed to publish status change event: %v", err)
		// Don't fail the request if notification fails
	}

	return c.JSON(fiber.Map{
		"message": "report status updated successfully",
		"report":  report,
	})
}

// AssignReport assigns a report to a government official
// Only admins can assign reports
func AssignReport(c *fiber.Ctx) error {
	// RBAC: Only admin can assign reports
	userRole, ok := c.Locals("role").(string)
	if !ok || userRole != "admin" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "only admins can assign reports"})
	}

	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}

	reportID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || reportID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid report id"})
	}

	var input struct {
		AssignedTo uint `json:"assigned_to"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if input.AssignedTo == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "assigned_to is required"})
	}

	// Verify assigned user exists and is government official
	var assignedUser models.User
	if err := DB.Preload("Role").Preload("Department").First(&assignedUser, input.AssignedTo).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "assigned user not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	if assignedUser.Role.Name != "government" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "can only assign to government officials"})
	}

	// Fetch report
	var report models.Report
	if err := DB.Preload("ReportCategory").First(&report, reportID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "report not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	report.AssignedTo = &input.AssignedTo
	report.Status = "in_progress" // Auto-update status when assigned
	report.UpdatedAt = time.Now()

	if err := DB.Save(&report).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Mark any existing assignments as reassigned (reassignment scenario)
	if err := DB.Model(&models.ReportAssignment{}).
		Where("report_id = ? AND status = ?", report.ReportID, "assigned").
		Update("status", "reassigned").Error; err != nil {
		log.Printf("Failed to mark old assignments as reassigned: %v", err)
	}

	// Create new ReportAssignment record
	assignment := models.ReportAssignment{
		ReportID:   report.ReportID,
		AssignedTo: input.AssignedTo,
		AssignedAt: time.Now(),
		Status:     "assigned",
	}
	if err := DB.Create(&assignment).Error; err != nil {
		log.Printf("Failed to create report assignment record: %v", err)
		// Don't fail the whole operation, but log the error
	}

	// Publish Kafka event for notification to assigned official
	var deptID uint
	if assignedUser.DepartmentID != nil {
		deptID = *assignedUser.DepartmentID
	}

	event := models.ReportAssignmentEvent{
		ReportID:     report.ReportID,
		AssignedTo:   input.AssignedTo,
		AssignedBy:   userID,
		DepartmentID: deptID,
		Title:        report.Title,
		Severity:     report.Severity,
		Timestamp:    time.Now().Format(time.RFC3339),
	}

	eventJSON, _ := json.Marshal(event)
	if err := PublishKafkaEvent("report.assignment", strconv.FormatUint(uint64(report.ReportID), 10), eventJSON); err != nil {
		log.Printf("Failed to publish assignment event: %v", err)
	}

	return c.JSON(fiber.Map{
		"message": "report assigned successfully",
		"report":  report,
	})
}

// EscalateReport escalates a report to a higher department
// Government officials and admins can escalate
func EscalateReport(c *fiber.Ctx) error {
	// RBAC: Only government and admin can escalate
	userRole, ok := c.Locals("role").(string)
	if !ok || (userRole != "government" && userRole != "admin") {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "only government officials and admins can escalate reports"})
	}

	userID, err := userIDFromLocals(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid token"})
	}

	reportID, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || reportID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid report id"})
	}

	var input struct {
		ToDepartmentID uint   `json:"to_department_id"`
		Reason         string `json:"reason"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if input.ToDepartmentID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "to_department_id is required"})
	}

	// Fetch report with category
	var report models.Report
	if err := DB.Preload("ReportCategory").First(&report, reportID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "report not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	fromDepartmentID := report.ReportCategory.DepartmentID

	// Create escalation record
	escalation := models.Escalation{
		ReportID:         report.ReportID,
		FromDepartmentID: fromDepartmentID,
		ToDepartmentID:   input.ToDepartmentID,
		Reason:           input.Reason,
		EscalatedAt:      time.Now(),
	}

	if err := DB.Create(&escalation).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Update report category to new department
	report.ReportCategory.DepartmentID = input.ToDepartmentID
	report.UpdatedAt = time.Now()
	if err := DB.Save(&report.ReportCategory).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Publish Kafka event for notification to higher department
	event := models.ReportEscalationEvent{
		ReportID:         report.ReportID,
		FromDepartmentID: fromDepartmentID,
		ToDepartmentID:   input.ToDepartmentID,
		Reason:           input.Reason,
		EscalatedBy:      userID,
		Title:            report.Title,
		Severity:         report.Severity,
		Timestamp:        time.Now().Format(time.RFC3339),
	}

	eventJSON, _ := json.Marshal(event)
	if err := PublishKafkaEvent("report.escalation", strconv.FormatUint(uint64(report.ReportID), 10), eventJSON); err != nil {
		log.Printf("Failed to publish escalation event: %v", err)
	}

	return c.JSON(fiber.Map{
		"message":    "report escalated successfully",
		"escalation": escalation,
	})
}

func main() {
	ConnectRabbit()
	ConnectDB()
	ConnectKafka()
	// database.ConnectRedis()

	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "*",
		AllowMethods: "*",
	}))

	// Report CRUD
	app.Post("/api/reports", middleware.Protected(), CreateReport)
	app.Post("/api/report_response", middleware.Protected(), CreateReportResponse)
	app.Get("/api/reports", middleware.Protected(), GetReports)
	app.Get("/api/reports/assigned", middleware.Protected(), GetAssignedReports)
	app.Get("/api/reports/:id", middleware.Protected(), GetReportDetails)
	app.Get("/api/reports/:id/status", middleware.Protected(), GetReportStatus)

	// Upvotes (Citizens only)
	app.Post("/api/reports/:id/upvote", middleware.Protected(), CreateUpvote)
	app.Delete("/api/reports/:id/upvote", middleware.Protected(), DeleteUpvote)

	// Report Management (with notifications)
	app.Patch("/api/reports/:id/status", middleware.Protected(), UpdateReportStatus)
	app.Post("/api/reports/:id/assign", middleware.Protected(), AssignReport)
	app.Post("/api/reports/:id/escalate", middleware.Protected(), EscalateReport)

	// Analytics
	app.Get("/api/analytics", middleware.Protected(), GetAnalytics)

	log.Fatal(app.Listen(":3001"))
}
