package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"models"
)

const (
	reportPublishedTopicDefault = "report.published"
	maxReconnectAttempts        = 3
	reconnectBackoff            = 500 * time.Millisecond
)

var DB *gorm.DB

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
		&models.ReportAssignment{},
		&models.User{},
		&models.Role{},
		&models.Department{},
	)
	if err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	log.Println("Database connected")
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
	if topic := os.Getenv("KAFKA_REPORT_PUBLISHED_TOPIC"); topic != "" {
		return topic
	}
	if topic := os.Getenv("KAFKA_TOPIC"); topic != "" {
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

func newKafkaReader() *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:     kafkaBrokers(),
		Topic:       kafkaTopic(),
		Partition:   0,
		StartOffset: kafka.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
	})
}

func handleReportPublishedEvent(event models.ReportPublishedEvent) error {
	if event.ReportID == 0 {
		return errors.New("report_id is required")
	}

	var assignedTo uint
	alreadyAssigned := false

	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing models.ReportAssignment
		if err := tx.Select("assigned_to").
			Where("report_id = ?", event.ReportID).
			Order("assigned_at desc, report_assignment_id desc").
			First(&existing).Error; err == nil {
			assignedTo = existing.AssignedTo
			alreadyAssigned = true
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var report models.Report
		if err := tx.Select("report_categories_id").
			Where("report_id = ?", event.ReportID).
			First(&report).Error; err != nil {
			return err
		}

		reportCategoryID := report.ReportCategoryID
		if reportCategoryID == 0 {
			reportCategoryID = event.ReportCategoryID
		}
		if reportCategoryID == 0 {
			return errors.New("report category is required")
		}

		var category models.ReportCategory
		if err := tx.Select("department_id").
			Where("report_categories_id = ?", reportCategoryID).
			First(&category).Error; err != nil {
			return err
		}

		if category.DepartmentID == 0 {
			return errors.New("report category department is required")
		}

		var staff models.User
		if err := tx.Select("user_id").
			Where("department_id = ? AND role_id <> ?", category.DepartmentID, 0).
			Order("RANDOM()").
			First(&staff).Error; err != nil {
			return err
		}

		assignment := models.ReportAssignment{
			Status:     "assigned",
			AssignedAt: time.Now(),
			AssignedTo: staff.UserID,
			ReportID:   event.ReportID,
		}

		if err := tx.Create(&assignment).Error; err != nil {
			return err
		}

		assignedTo = staff.UserID
		return nil
	})
	if err != nil {
		return err
	}

	if alreadyAssigned {
		log.Printf("Report already assigned (report_id=%d assigned_to=%d)", event.ReportID, assignedTo)
	} else {
		log.Printf("Assigned report (report_id=%d assigned_to=%d)", event.ReportID, assignedTo)
	}

	return nil
}

func consumeReportPublishedEvents(reader *kafka.Reader) {
	retryCount := 0
	for {
		msg, err := reader.ReadMessage(context.Background())
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			if errors.Is(err, io.EOF) {
				log.Printf("Kafka connection closed, retrying...")
			} else {
				log.Printf("Kafka read error: %v", err)
			}

			retryCount++
			if retryCount >= maxReconnectAttempts {
				log.Fatal("Failed to read from Kafka after retries:", err)
			}
			time.Sleep(time.Duration(retryCount) * reconnectBackoff)
			continue
		}

		retryCount = 0

		var event models.ReportPublishedEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			log.Printf("Kafka message parse error: %v (value=%s)", err, string(msg.Value))
			continue
		}

		if err := handleReportPublishedEvent(event); err != nil {
			log.Printf("Failed to assign report (report_id=%d): %v", event.ReportID, err)
		}
	}
}

func main() {
	ConnectDB()
	connectKafkaWithRetry()

	reader := newKafkaReader()
	defer func() {
		if err := reader.Close(); err != nil {
			log.Printf("Failed to close Kafka reader: %v", err)
		}
	}()

	consumeReportPublishedEvents(reader)
}
