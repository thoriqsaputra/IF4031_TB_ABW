package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"models"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/websocket/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/segmentio/kafka-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Global connections
var (
	DB              *gorm.DB
	KafkaReaders    []*kafka.Reader
	NotificationHub *Hub
)

// Hub maintains active WebSocket connections
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

// Client represents a WebSocket client
type Client struct {
	hub          *Hub
	conn         *websocket.Conn
	send         chan []byte
	userID       uint
	userRole     string // "citizen", "government", "admin"
	departmentID uint   // For department-scoped filtering
}

// Prometheus metrics
var (
	notificationsProcessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifications_processed_total",
			Help: "Total number of notifications processed",
		},
		[]string{"type", "status"},
	)
	notificationsDelivered = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifications_delivered_total",
			Help: "Total number of notifications delivered via WebSocket",
		},
		[]string{"status"},
	)
	activeWebSocketConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "notification_websocket_connections_active",
			Help: "Number of active WebSocket connections",
		},
	)
	kafkaConsumerLag = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "notification_kafka_consumer_lag",
			Help: "Kafka consumer lag by topic",
		},
		[]string{"topic"},
	)
	eventProcessingDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "notification_event_processing_duration_seconds",
			Help:    "Duration of event processing",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"event_type"},
	)
)

func init() {
	prometheus.MustRegister(notificationsProcessed)
	prometheus.MustRegister(notificationsDelivered)
	prometheus.MustRegister(activeWebSocketConnections)
	prometheus.MustRegister(kafkaConsumerLag)
	prometheus.MustRegister(eventProcessingDuration)
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			activeWebSocketConnections.Inc()
			log.Printf("Client registered. Total clients: %d\n", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				activeWebSocketConnections.Dec()
			}
			h.mu.Unlock()
			log.Printf("Client unregistered. Total clients: %d\n", len(h.clients))

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
					notificationsDelivered.WithLabelValues("success").Inc()
				default:
					h.mu.RUnlock()
					h.unregister <- client
					h.mu.RLock()
					notificationsDelivered.WithLabelValues("failed").Inc()
				}
			}
			h.mu.RUnlock()
		}
	}
}

// SendToUser sends notification to specific user
func (h *Hub) SendToUser(userID uint, message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if client.userID == userID {
			select {
			case client.send <- message:
				notificationsDelivered.WithLabelValues("success").Inc()
			default:
				notificationsDelivered.WithLabelValues("failed").Inc()
			}
		}
	}
}

// SendToRole sends notification to all users with specific role
func (h *Hub) SendToRole(role string, message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if client.userRole == role {
			select {
			case client.send <- message:
				notificationsDelivered.WithLabelValues("success").Inc()
			default:
				notificationsDelivered.WithLabelValues("failed").Inc()
			}
		}
	}
}

// SendToDepartment sends notification to all users in specific department
func (h *Hub) SendToDepartment(departmentID uint, message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if client.userRole == "government" && client.departmentID == departmentID {
			select {
			case client.send <- message:
				notificationsDelivered.WithLabelValues("success").Inc()
			default:
				notificationsDelivered.WithLabelValues("failed").Inc()
			}
		}
	}
}

// WritePump sends messages to WebSocket client
func (c *Client) WritePump() {
	defer func() {
		c.conn.Close()
	}()

	for message := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			log.Printf("WebSocket write error: %v\n", err)
			return
		}
	}
}

// ReadPump reads messages from WebSocket client
func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v\n", err)
			}
			break
		}
	}
}

// ConnectDB establishes database connection
func ConnectDB() error {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=postgres dbname=agarthan port=5432 sslmode=disable"
	}

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Auto-migrate models
	if err := DB.AutoMigrate(
		&models.Notification{},
	); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	log.Println("✓ Database connected and migrated")
	return nil
}

// ConnectKafka establishes Kafka connections for multiple topics
func ConnectKafka() error {
	brokers := strings.Split(os.Getenv("KAFKA_BROKERS"), ",")
	if len(brokers) == 0 || brokers[0] == "" {
		brokers = []string{"localhost:9092"}
	}

	groupID := os.Getenv("KAFKA_GROUP_ID")
	if groupID == "" {
		groupID = "notification_service"
	}

	// Topics to subscribe
	topics := []string{
		"submission.events",
		"media.events",
		"report.status_change",
		"report.assignment",
		"report.escalation",
	}

	for _, topic := range topics {
		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers:        brokers,
			Topic:          topic,
			GroupID:        groupID,
			MinBytes:       1,
			MaxBytes:       10e6, // 10MB
			CommitInterval: time.Second,
			StartOffset:    kafka.FirstOffset, // Start from beginning for new consumer groups
		})

		KafkaReaders = append(KafkaReaders, reader)
		log.Printf("✓ Kafka reader created for topic: %s\n", topic)
	}

	return nil
}

// ProcessKafkaEvents processes events from Kafka topics
func ProcessKafkaEvents(ctx context.Context, reader *kafka.Reader) {
	topic := reader.Config().Topic
	log.Printf("Starting event consumer for topic: %s\n", topic)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Stopping consumer for topic: %s\n", topic)
			return
		default:
			m, err := reader.ReadMessage(ctx)
			if err != nil {
				if err == context.Canceled {
					return
				}
				// EOF or timeout is normal when no messages available
				if err.Error() != "EOF" && err.Error() != "context deadline exceeded" {
					log.Printf("Error reading message from %s: %v\n", topic, err)
				}
				continue
			}

			go handleKafkaMessage(ctx, reader, m, topic)
		}
	}
}

// handleKafkaMessage processes individual Kafka message
func handleKafkaMessage(ctx context.Context, reader *kafka.Reader, m kafka.Message, topic string) {
	timer := prometheus.NewTimer(eventProcessingDuration.WithLabelValues(topic))
	defer timer.ObserveDuration()

	log.Printf("Received message from %s: %s\n", topic, string(m.Value))

	var notification *models.Notification

	switch topic {
	case "submission.events":
		notification = handleSubmissionEvent(m.Value)
	case "media.events":
		notification = handleMediaEvent(m.Value)
	case "report.status_change":
		notification = handleStatusChangeEvent(m.Value)
	case "report.assignment":
		notification = handleAssignmentEvent(m.Value)
	case "report.escalation":
		notification = handleEscalationEvent(m.Value)
	default:
		log.Printf("Unknown topic: %s\n", topic)
		notificationsProcessed.WithLabelValues(topic, "ignored").Inc()
	}

	if notification != nil {
		if err := DB.Create(notification).Error; err != nil {
			log.Printf("Failed to save notification: %v\n", err)
			notificationsProcessed.WithLabelValues(topic, "error").Inc()
		} else {
			log.Printf("✓ Notification saved: %s\n", notification.Title)
			notificationsProcessed.WithLabelValues(topic, "success").Inc()

			notifJSON, _ := json.Marshal(notification)
			NotificationHub.SendToUser(notification.UserID, notifJSON)

			// Also notify government users in the relevant department for new reports
			if topic == "submission.events" && notification.Type == "report_success" && notification.ReportID != nil {
				var report models.Report
				if err := DB.Preload("ReportCategory").First(&report, *notification.ReportID).Error; err == nil {
					if report.ReportCategory.DepartmentID > 0 {
						deptNotification := models.Notification{
							Type:      "new_report",
							Title:     "Laporan Baru Masuk",
							Message:   fmt.Sprintf("Laporan baru: %s", report.Title),
							ReportID:  notification.ReportID,
							CreatedAt: time.Now(),
						}
						deptNotifJSON, _ := json.Marshal(deptNotification)
						NotificationHub.SendToDepartment(report.ReportCategory.DepartmentID, deptNotifJSON)
						log.Printf("✓ Department notification sent to department %d\n", report.ReportCategory.DepartmentID)
					}
				}
			}
		}
	}

	if err := reader.CommitMessages(ctx, m); err != nil {
		log.Printf("Failed to commit message: %v\n", err)
	}
}

// handleSubmissionEvent processes submission events
func handleSubmissionEvent(data []byte) *models.Notification {
	var event struct {
		RequestID string `json:"request_id"`
		Status    string `json:"status"`
		Message   string `json:"message"`
		UserID    uint   `json:"user_id,omitempty"`
		ReportID  uint   `json:"report_id,omitempty"`
	}

	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("Failed to parse submission event: %v\n", err)
		return nil
	}

	userID := event.UserID
	if userID == 0 {
		userID = 1
	}

	var title, message string
	var notifType string

	// Handle different status types
	switch event.Status {
	case "created":
		title = "Laporan Berhasil Dibuat"
		message = "Laporan Anda telah berhasil dibuat dan sedang menunggu verifikasi."
		notifType = "report_created"
	case "success":
		title = "Laporan Berhasil Dikirim"
		message = "Laporan Anda telah berhasil dikirim dan akan segera diproses."
		if event.Message != "" {
			message = event.Message
		}
		notifType = "report_success"
	case "error":
		title = "Laporan Gagal Dikirim"
		message = "Maaf, terjadi kesalahan saat mengirim laporan Anda."
		if event.Message != "" {
			message = event.Message
		}
		notifType = "report_error"
	default:
		return nil
	}

	var reportID *uint
	if event.ReportID > 0 {
		reportID = &event.ReportID
	}

	return &models.Notification{
		UserID:    userID,
		Type:      notifType,
		Title:     title,
		Message:   message,
		ReportID:  reportID,
		CreatedAt: time.Now(),
	}
}

// handleMediaEvent processes media events
func handleMediaEvent(data []byte) *models.Notification {
	var event models.MediaProcessingEvent

	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("Failed to parse media event: %v\n", err)
		return nil
	}

	if event.Status != "completed" && event.Status != "failed" {
		return nil
	}

	var media models.ReportMedia
	if err := DB.Where("object_key = ?", event.OriginalKey).Preload("Report").First(&media).Error; err != nil {
		log.Printf("Failed to find media: %v\n", err)
		return nil
	}

	title := "Media Processed"
	message := fmt.Sprintf("Thumbnail generated for your media")
	if event.Status == "failed" {
		title = "Media Processing Failed"
		message = fmt.Sprintf("Failed to process media: %s", event.ErrorMessage)
	}

	var reportID *uint
	if media.ReportID > 0 {
		reportID = &media.ReportID
	}

	userID := uint(1)
	if media.Report.UserID > 0 {
		userID = media.Report.UserID
	}

	return &models.Notification{
		UserID:    userID,
		Type:      "media_" + event.Status,
		Title:     title,
		Message:   message,
		ReportID:  reportID,
		CreatedAt: time.Now(),
	}
}

// handleStatusChangeEvent handles report status change events
// Notifies the citizen who reported about progress on their report
func handleStatusChangeEvent(data []byte) *models.Notification {
	var event models.ReportStatusChangeEvent

	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("Failed to parse status change event: %v\n", err)
		return nil
	}

	// Generate citizen-friendly status messages
	statusMessages := map[string]string{
		"pending":     "telah diterima dan sedang menunggu peninjauan",
		"in_progress": "sedang ditangani oleh petugas terkait",
		"resolved":    "telah diselesaikan",
		"rejected":    "telah ditolak",
	}

	message := fmt.Sprintf("Laporan Anda '%s' %s", event.Title, statusMessages[event.NewStatus])

	// Respect anonymous privacy - don't expose reporter identity in any logs or database records
	reportID := event.ReportID
	return &models.Notification{
		UserID:    event.UserID, // Notification sent to original reporter
		Type:      "status_update",
		Title:     "Status Laporan Diperbarui",
		Message:   message,
		ReportID:  &reportID,
		CreatedAt: time.Now(),
	}
}

// handleAssignmentEvent handles report assignment events
// Notifies the government official assigned to handle a report
func handleAssignmentEvent(data []byte) *models.Notification {
	var event models.ReportAssignmentEvent

	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("Failed to parse assignment event: %v\n", err)
		return nil
	}

	message := fmt.Sprintf("Anda telah ditugaskan menangani laporan: %s (Tingkat: %s)", event.Title, event.Severity)

	reportID := event.ReportID
	return &models.Notification{
		UserID:    event.AssignedTo, // Notification sent to assigned official
		Type:      "report_assigned",
		Title:     "Laporan Baru Ditugaskan",
		Message:   message,
		ReportID:  &reportID,
		CreatedAt: time.Now(),
	}
}

// Notifies government officials in the higher department about escalated reports
func handleEscalationEvent(data []byte) *models.Notification {
	var event models.ReportEscalationEvent

	if err := json.Unmarshal(data, &event); err != nil {
		log.Printf("Failed to parse escalation event: %v\n", err)
		return nil
	}

	message := fmt.Sprintf("Laporan telah dieskalasi ke departemen Anda: %s (Tingkat: %s). Alasan: %s",
		event.Title, event.Severity, event.Reason)

	reportID := event.ReportID

	// Get all government officials in the target department
	var users []models.User
	if err := DB.Where("role_id = ? AND department_id = ?", 2, event.ToDepartmentID).Find(&users).Error; err != nil {
		log.Printf("Failed to get users in department %d: %v\n", event.ToDepartmentID, err)
		return nil
	}

	// Create notification for each user in the department
	for _, user := range users {
		notification := models.Notification{
			UserID:    user.UserID,
			Type:      "report_escalated",
			Title:     "Laporan Dieskalasi",
			Message:   message,
			ReportID:  &reportID,
			CreatedAt: time.Now(),
		}

		if err := DB.Create(&notification).Error; err != nil {
			log.Printf("Failed to save escalation notification for user %d: %v\n", user.UserID, err)
			continue
		}

		// Send via WebSocket if connected
		notifJSON, _ := json.Marshal(notification)
		NotificationHub.SendToUser(user.UserID, notifJSON)
	}

	log.Printf("✓ Escalation notification sent to %d users in department %d\n", len(users), event.ToDepartmentID)

	return nil
}

// GetNotificationsHandler returns notifications for a user
func GetNotificationsHandler(c *fiber.Ctx) error {
	userID := c.Query("user_id", "1")
	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)

	var notifications []models.Notification
	query := DB.Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset)

	if err := query.Find(&notifications).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to fetch notifications",
		})
	}

	var stats models.NotificationStats
	DB.Model(&models.Notification{}).Where("user_id = ?", userID).Count(&stats.TotalNotifications)
	DB.Model(&models.Notification{}).Where("user_id = ? AND is_read = ?", userID, false).Count(&stats.UnreadNotifications)

	return c.JSON(fiber.Map{
		"notifications": notifications,
		"stats":         stats,
	})
}

// MarkAsReadHandler marks notification as read
func MarkAsReadHandler(c *fiber.Ctx) error {
	id := c.Params("id")

	if err := DB.Model(&models.Notification{}).Where("notification_id = ?", id).Update("is_read", true).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to update notification",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
	})
}

// WebSocketHandler handles WebSocket connections
func WebSocketHandler(c *websocket.Conn) {
	userID := c.Query("user_id", "1")
	var uid uint
	fmt.Sscanf(userID, "%d", &uid)

	// Fetch user role and department
	var user models.User
	DB.Preload("Role").Preload("Department").First(&user, uid)

	var deptID uint
	if user.DepartmentID != nil {
		deptID = *user.DepartmentID
	}

	client := &Client{
		hub:          NotificationHub,
		conn:         c,
		send:         make(chan []byte, 256),
		userID:       uid,
		userRole:     user.Role.Name,
		departmentID: deptID,
	}

	NotificationHub.register <- client

	go client.WritePump()
	client.ReadPump()
}

// HealthHandler returns service health
func HealthHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":    "healthy",
		"service":   "notification_service",
		"timestamp": time.Now(),
	})
}

func main() {
	log.Println("Starting Notification Service...")

	if err := ConnectDB(); err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}

	NotificationHub = NewHub()
	go NotificationHub.Run()

	if err := ConnectKafka(); err != nil {
		log.Fatalf("Kafka connection failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, reader := range KafkaReaders {
		go ProcessKafkaEvents(ctx, reader)
	}

	app := fiber.New()

	app.Use(recover.New())
	app.Use(cors.New())

	app.Get("/health", HealthHandler)
	app.Get("/notifications", GetNotificationsHandler)
	app.Put("/notifications/:id/read", MarkAsReadHandler)

	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	app.Get("/ws", websocket.New(WebSocketHandler))

	app.Get("/metrics", func(c *fiber.Ctx) error {
		metrics, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}

		c.Set("Content-Type", string(expfmt.FmtText))
		encoder := expfmt.NewEncoder(c.Response().BodyWriter(), expfmt.FmtText)
		for _, mf := range metrics {
			if err := encoder.Encode(mf); err != nil {
				return err
			}
		}
		return nil
	})

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down gracefully...")
		cancel()

		for _, reader := range KafkaReaders {
			reader.Close()
		}

		app.Shutdown()
	}()

	port := os.Getenv("PORT")
	if port == "" {
		port = "3003"
	}

	log.Printf("Notification Service listening on port %s\n", port)
	if err := app.Listen(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
