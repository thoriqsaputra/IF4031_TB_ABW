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
	reportQueueName           = "report_requests"
	reportResponseQueueName   = "report_response"
	reportResponseMaxPriority = 10
	submissionTopicDefault    = "submission.events"
	reportPublishedTopicDefault = "report.published"
	maxReconnectAttempts      = 3
	reconnectBackoff          = 500 * time.Millisecond
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
		&models.ReportResponse{},
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
				_, err = ch.QueueDeclare(
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

func kafkaReportPublishedTopic() string {
	if topic := os.Getenv("KAFKA_REPORT_PUBLISHED_TOPIC"); topic != "" {
		return topic
	}

	return reportPublishedTopicDefault
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

func newKafkaWriter(topic string) *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(kafkaBrokers()...),
		Topic:        topic,
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

func sendReportPublishedEvent(writer *kafka.Writer, report models.Report) {
	publishedAt := report.CreatedAt
	if publishedAt.IsZero() {
		publishedAt = time.Now()
	}

	event := models.ReportPublishedEvent{
		ReportID:         report.ReportID,
		ReportCategoryID: report.ReportCategoryID,
		PublishedAt:      publishedAt,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("Failed to serialize report published event: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := writer.WriteMessages(ctx, kafka.Message{Value: payload}); err != nil {
		log.Printf("Failed to publish report published event: %v", err)
	} else {
		log.Printf("Report published event sent (report_id=%d)", report.ReportID)
	}
}

func consumeReportRequests(msgs <-chan amqp.Delivery, kafkaWriter *kafka.Writer, reportPublishedWriter *kafka.Writer) {
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

		sendReportPublishedEvent(reportPublishedWriter, report)
		sendSubmissionEvent(kafkaWriter, submissionEvent{
			RequestID: request.RequestID,
			Status:    "success",
			Message:   "stored",
		})
		_ = msg.Ack(false)
	}
}

func consumeReportResponses(msgs <-chan amqp.Delivery, kafkaWriter *kafka.Writer) {
	log.Println("Report response worker waiting for messages")

	for msg := range msgs {
		var request models.ReportResponseRequestMessage
		if err := json.Unmarshal(msg.Body, &request); err != nil {
			log.Printf("Invalid report_response payload: %v", err)
			_ = msg.Reject(false)
			continue
		}

		if request.RequestID == "" {
			log.Printf("Missing request_id in report_response payload")
			_ = msg.Reject(false)
			continue
		}

		response := request.Response
		if response.ReportID == 0 || response.CreatedBy == 0 || strings.TrimSpace(response.Message) == "" {
			log.Printf("Incomplete report_response payload (request_id=%s)", request.RequestID)
			_ = msg.Reject(false)
			continue
		}

		if response.CreatedAt.IsZero() {
			response.CreatedAt = time.Now()
		}
		response.ReportResponseID = 0

		if err := DB.Select("report_id").Where("report_id = ?", response.ReportID).First(&models.Report{}).Error; err != nil {
			log.Printf("Report not found for response (request_id=%s report_id=%d): %v", request.RequestID, response.ReportID, err)
			sendSubmissionEvent(kafkaWriter, submissionEvent{
				RequestID: request.RequestID,
				Status:    "error",
				Message:   "report not found",
			})
			_ = msg.Reject(false)
			continue
		}

		if err := DB.Create(&response).Error; err != nil {
			log.Printf("Failed to store report response (request_id=%s): %v", request.RequestID, err)
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
		log.Printf("Report response stored (request_id=%s report_id=%d created_by=%d)", request.RequestID, response.ReportID, response.CreatedBy)

		// Fetch the report to get the reporter ID and title for notification
		var report models.Report
		if err := DB.Select("report_id", "user_id", "title", "is_anonymous").Where("report_id = ?", response.ReportID).First(&report).Error; err == nil {
			// Publish response event to Kafka for notification
			responseEvent := models.ReportResponseEvent{
				ReportID:    response.ReportID,
				ResponseID:  response.ReportResponseID,
				RespondedBy: response.CreatedBy,
				ReporterID:  report.UserID,
				Title:       report.Title,
				IsAnonymous: report.IsAnonymous,
				Timestamp:   time.Now().Format(time.RFC3339),
			}

			eventJSON, _ := json.Marshal(responseEvent)
			// Create a dedicated writer for report.response topic
			responseWriter := newKafkaWriter("report.response")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := responseWriter.WriteMessages(ctx, kafka.Message{
				Key:   []byte(string(response.ReportID)),
				Value: eventJSON,
			}); err != nil {
				log.Printf("Failed to publish response event: %v", err)
			} else {
				log.Printf("Published response event for report %d", response.ReportID)
			}
			cancel()
			responseWriter.Close()
		}

		_ = msg.Ack(false)
	}
}

func main() {
	ConnectDB()

	conn, ch := ConnectRabbit()
	defer conn.Close()
	defer ch.Close()

	connectKafkaWithRetry()
	kafkaWriter := newKafkaWriter(kafkaTopic())
	reportPublishedWriter := newKafkaWriter(kafkaReportPublishedTopic())
	defer func() {
		if err := kafkaWriter.Close(); err != nil {
			log.Printf("Failed to close Kafka writer: %v", err)
		}
	}()
	defer func() {
		if err := reportPublishedWriter.Close(); err != nil {
			log.Printf("Failed to close report published writer: %v", err)
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

	responseMsgs, err := ch.Consume(
		reportResponseQueueName,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatal("Failed to register report_response consumer:", err)
	}

	log.Println("Starting response consumer goroutine...")
	go consumeReportResponses(responseMsgs, kafkaWriter)
	log.Println("Response consumer goroutine started")

	consumeReportRequests(msgs, kafkaWriter, reportPublishedWriter)
}
