package main

import (
	"encoding/json"
	"log"
	"os"

	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"models"
)

const reportQueueName = "report_requests"

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

	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		log.Fatal("Failed to connect to RabbitMQ:", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		log.Fatal("Failed to open RabbitMQ channel:", err)
	}

	_, err = ch.QueueDeclare(
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
	return conn, ch
}

func main() {
	ConnectDB()

	conn, ch := ConnectRabbit()
	defer conn.Close()
	defer ch.Close()

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
		var report models.Report
		if err := json.Unmarshal(msg.Body, &report); err != nil {
			log.Printf("Invalid message payload: %v", err)
			_ = msg.Reject(false)
			continue
		}

		report.ReportID = 0

		if err := DB.Omit("User", "ReportCategory").Create(&report).Error; err != nil {
			log.Printf("Failed to store report: %v", err)
			_ = msg.Nack(false, true)
			continue
		}

		_ = msg.Ack(false)
	}
}
