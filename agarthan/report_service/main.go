package main

import (
	"context"
	"database"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"
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
	reportQueueName      = "report_requests"
	maxReconnectAttempts = 3
	reconnectBackoff     = 500 * time.Millisecond
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

	requestID := uuid.NewString()
	message := models.ReportRequestMessage{
		RequestID: requestID,
		Report:    *report,
	}

	payload, err := json.Marshal(message)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	err = RabbitChannel.Publish(
		"",
		reportQueueName,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         payload,
		},
	)
	if err != nil {
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

	//DB Find should only return reports that are flagged as public or is reported by user
	if err := DB.Where("is_public = ? OR user_id = ?", true, userID).Find(&reports).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(reports)
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
	userRole, ok := c.Locals("userRole").(string)
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
	userRole, ok := c.Locals("userRole").(string)
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
	userRole, ok := c.Locals("userRole").(string)
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
	database.ConnectRedis()

	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "*",
		AllowMethods: "*",
	}))

	// Report CRUD
	app.Post("/reports", middleware.Protected(), CreateReport)
	app.Get("/reports", middleware.Protected(), GetReports)
	app.Get("/reports/:id", middleware.Protected(), GetReportDetails)
	app.Get("/api/reports/:id", middleware.Protected(), GetReportDetails)

	// Report Management (with notifications)
	app.Patch("/reports/:id/status", middleware.Protected(), UpdateReportStatus)
	app.Post("/reports/:id/assign", middleware.Protected(), AssignReport)
	app.Post("/reports/:id/escalate", middleware.Protected(), EscalateReport)

	// Analytics
	app.Get("/api/analytics", middleware.Protected(), GetAnalytics)

	log.Fatal(app.Listen(":3001"))
}
