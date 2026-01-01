package main

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
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

func userIDFromLocals(c *fiber.Ctx) (uint, error) {
	raw := c.Locals("userID")
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
	} else if user.DepartmentID != 0 || user.RoleID != 0 {
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

	//DB Find should only return reports that are flagged as public or is reported by user
	if err := DB.Where("is_public = ? OR user_id = ?", true, userID).Find(&reports).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
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

func main() {
	ConnectRabbit()
	ConnectDB()

	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "*",
		AllowMethods: "*",
	}))

	app.Post("/reports", middleware.Protected(), CreateReport)
	app.Post("/report_response", middleware.Protected(), CreateReportResponse)
	app.Get("/reports", middleware.Protected(), GetReports)
	app.Get("/reports/assigned", middleware.Protected(), GetAssignedReports)
	app.Get("/reports/:id", middleware.Protected(), GetReportDetails)
	app.Get("/reports/:id/status", middleware.Protected(), GetReportStatus)
	app.Get("/report/:id/status", middleware.Protected(), GetReportStatus)
	app.Get("/api/reports/:id", middleware.Protected(), GetReportDetails)
	app.Get("/api/reports/assigned", middleware.Protected(), GetAssignedReports)
	app.Get("/api/reports/:id/status", middleware.Protected(), GetReportStatus)
	app.Get("/api/report/:id/status", middleware.Protected(), GetReportStatus)
	app.Post("/api/report_response", middleware.Protected(), CreateReportResponse)

	log.Fatal(app.Listen(":3001"))
}
