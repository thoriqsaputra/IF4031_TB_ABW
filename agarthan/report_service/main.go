package main

import (
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
	app.Get("/reports", middleware.Protected(), GetReports)
	app.Get("/reports/:id", middleware.Protected(), GetReportDetails)
	app.Get("/api/reports/:id", middleware.Protected(), GetReportDetails)

	log.Fatal(app.Listen(":3001"))
}
