package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/segmentio/kafka-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"models"
)

const (
	reportQueueName        = "report_requests"
	submissionTopicDefault = "submission.events"
	maxReconnectAttempts   = 3
	reconnectBackoff       = 500 * time.Millisecond
)

var DB *gorm.DB

type submissionEvent struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
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

func ConnectRabbit() (*amqp.Connection, *amqp.Channel) {
	amqpURL := os.Getenv("RABBITMQ_URL")
	if amqpURL == "" {
		amqpURL = "amqp://guest:guest@localhost:5672/"
	}

	var lastErr error
	for attempt := 1; attempt <= maxReconnectAttempts; attempt++ {
		conn, err := amqp.Dial(amqpURL)
		if err == nil {
			ch, err := conn.Channel()
			if err == nil {
				_, err = ch.QueueDeclare(
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
				return conn, ch
			}
			if ch != nil {
				_ = ch.Close()
			}
		}
		if conn != nil {
			_ = conn.Close()
		}

		lastErr = err
		log.Printf("RabbitMQ connection failed (attempt %d/%d): %v", attempt, maxReconnectAttempts, err)
		if attempt < maxReconnectAttempts {
			time.Sleep(time.Duration(attempt) * reconnectBackoff)
		}
	}

	log.Fatal("Failed to connect to RabbitMQ after retries:", lastErr)
	return nil, nil
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

func dialKafkaBroker() (*kafka.Conn, error) {
	brokers := kafkaBrokers()
	if len(brokers) == 0 {
		return nil, errors.New("no kafka brokers configured")
	}

	var lastErr error
	for _, broker := range brokers {
		conn, err := kafka.Dial("tcp", broker)
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}

	return nil, lastErr
}

func connectKafkaWithRetry() {
	var lastErr error
	for attempt := 1; attempt <= maxReconnectAttempts; attempt++ {
		conn, err := dialKafkaBroker()
		if err == nil {
			_ = conn.Close()
			log.Println("Kafka connected")
			return
		}

		lastErr = err
		log.Printf("Kafka connection failed (attempt %d/%d): %v", attempt, maxReconnectAttempts, err)
		if attempt < maxReconnectAttempts {
			time.Sleep(time.Duration(attempt) * reconnectBackoff)
		}
	}

	log.Fatal("Failed to connect to Kafka after retries:", lastErr)
}

func newKafkaWriter() *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(kafkaBrokers()...),
		Topic:        kafkaTopic(),
		RequiredAcks: kafka.RequireOne,
		Balancer:     &kafka.LeastBytes{},
	}
}

func sendSubmissionEvent(writer *kafka.Writer, event submissionEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to serialize submission event: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := writer.WriteMessages(ctx, kafka.Message{Value: payload}); err != nil {
		log.Printf("Failed to publish submission event: %v", err)
	} else {
		log.Printf("Successful publish submission event")
	}
}

func main() {
	ConnectDB()

	conn, ch := ConnectRabbit()
	defer conn.Close()
	defer ch.Close()

	connectKafkaWithRetry()
	kafkaWriter := newKafkaWriter()
	defer func() {
		if err := kafkaWriter.Close(); err != nil {
			log.Printf("Failed to close Kafka writer: %v", err)
		}
	}()

	if err := ch.Qos(1, 0, false); err != nil {
		log.Fatal("Failed to set QoS:", err)
	}

	msgs, err := ch.Consume(
		reportQueueName,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatal("Failed to register consumer:", err)
	}

	log.Println("Report worker waiting for messages")

	for msg := range msgs {
		var request models.ReportRequestMessage
		if err := json.Unmarshal(msg.Body, &request); err != nil {
			log.Printf("Invalid message payload: %v", err)
			_ = msg.Reject(false)
			continue
		}

		if request.RequestID == "" {
			log.Printf("Missing request_id in payload")
			_ = msg.Reject(false)
			continue
		}

		report := request.Report
		report.ReportID = 0

		if err := DB.Omit("User", "ReportCategory").Create(&report).Error; err != nil {
			log.Printf("Failed to store report: %v", err)
			sendSubmissionEvent(kafkaWriter, submissionEvent{
				RequestID: request.RequestID,
				Status:    "error",
				Message:   err.Error(),
			})
			_ = msg.Nack(false, true)
			continue
		}

		sendSubmissionEvent(kafkaWriter, submissionEvent{
			RequestID: request.RequestID,
			Status:    "success",
			Message:   "stored",
		})
		_ = msg.Ack(false)
	}
}
