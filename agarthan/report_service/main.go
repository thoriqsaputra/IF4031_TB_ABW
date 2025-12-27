package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/segmentio/kafka-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"models"
)

const (
	reportQueueName        = "report_requests"
	submissionTopicDefault = "submission.events"
	kafkaGroupDefault      = "report_service_notifications"
)

var DB *gorm.DB
var RabbitConn *amqp.Connection
var RabbitChannel *amqp.Channel

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

	RabbitConn, err = amqp.Dial(amqpURL)
	if err != nil {
		log.Fatal("Failed to connect to RabbitMQ:", err)
	}

	RabbitChannel, err = RabbitConn.Channel()
	if err != nil {
		log.Fatal("Failed to open RabbitMQ channel:", err)
	}

	_, err = RabbitChannel.QueueDeclare(
		reportQueueName,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatal("Failed to declare RabbitMQ queue:", err)
	}

	log.Println("RabbitMQ connected")
}

func kafkaBrokers() []string {
	brokersEnv := os.Getenv("KAFKA_BROKERS")
	if brokersEnv == "" {
		brokersEnv = "localhost:9092"
	}

	parts := strings.Split(brokersEnv, ",")
	brokers := make([]string, 0, len(parts))
	for _, part := range parts {
		broker := strings.TrimSpace(part)
		if broker != "" {
			brokers = append(brokers, broker)
		}
	}

	return brokers
}

func kafkaTopic() string {
	if topic := os.Getenv("KAFKA_TOPIC"); topic != "" {
		return topic
	}

	return submissionTopicDefault
}

func kafkaGroupID() string {
	if groupID := os.Getenv("KAFKA_GROUP_ID"); groupID != "" {
		return groupID
	}

	return kafkaGroupDefault
}

func startKafkaNotificationListener() *kafka.Reader {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  kafkaBrokers(),
		Topic:    kafkaTopic(),
		GroupID:  kafkaGroupID(),
		MinBytes: 1,
		MaxBytes: 10e6,
	})

	go func() {
		for {
			msg, err := reader.ReadMessage(context.Background())
			if err != nil {
				if errors.Is(err, kafka.ErrClosed) {
					return
				}
				log.Printf("Kafka read error: %v", err)
				time.Sleep(time.Second)
				continue
			}

			fmt.Println("Notification:", string(msg.Value))
		}
	}()

	return reader
}

func CreateReport(c *fiber.Ctx) error {
	report := new(models.Report)

	if err := c.BodyParser(report); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

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

	if err := DB.Find(&reports).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(reports)
}

func main() {
	ConnectRabbit()
	ConnectDB()

	kafkaReader := startKafkaNotificationListener()
	defer func() {
		if err := kafkaReader.Close(); err != nil {
			log.Printf("Failed to close Kafka reader: %v", err)
		}
	}()

	app := fiber.New()

	app.Post("/reports", CreateReport)
	app.Get("/reports", GetReports)

	log.Fatal(app.Listen(":3001"))
}
