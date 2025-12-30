package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/segmentio/kafka-go"
)

const (
	submissionTopicDefault = "submission.events"
	waitTimeoutDefault     = 5 * time.Second
	maxReconnectAttempts   = 3
	reconnectBackoff       = 500 * time.Millisecond
)

// submissionEvent mirrors the worker payload. It also accepts legacy casing.
type submissionEvent struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

type legacySubmissionEvent struct {
	RequestID string `json:"RequestID"`
	Status    string `json:"Status"`
	Message   string `json:"Message"`
}

type waiterHub struct {
	mu      sync.Mutex
	waiters map[string]chan submissionEvent
}

func newWaiterHub() *waiterHub {
	return &waiterHub{waiters: make(map[string]chan submissionEvent)}
}

func (h *waiterHub) register(requestID string) chan submissionEvent {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan submissionEvent, 1)
	h.waiters[requestID] = ch
	return ch
}

func (h *waiterHub) unregister(requestID string) {
	h.mu.Lock()
	ch, ok := h.waiters[requestID]
	if ok {
		delete(h.waiters, requestID)
	}
	h.mu.Unlock()

	if ok {
		close(ch)
	}
}

func (h *waiterHub) deliver(ev submissionEvent) {
	h.mu.Lock()
	ch, ok := h.waiters[ev.RequestID]
	if ok {
		delete(h.waiters, ev.RequestID)
	}
	h.mu.Unlock()

	if !ok {
		return
	}

	select {
	case ch <- ev:
	default:
	}
	close(ch)
}

var notificationHub = newWaiterHub()

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

func startKafkaNotificationListener() *kafka.Reader {
	connectKafkaWithRetry()

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     kafkaBrokers(),
		Topic:       kafkaTopic(),
		Partition:   0,
		StartOffset: kafka.LastOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
	})

	go func() {
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
			var ev submissionEvent
			if err := json.Unmarshal(msg.Value, &ev); err != nil {
				log.Printf("Kafka message parse error: %v (value=%s)", err, string(msg.Value))
				continue
			}
			if ev.RequestID == "" {
				var legacy legacySubmissionEvent
				if err := json.Unmarshal(msg.Value, &legacy); err == nil && legacy.RequestID != "" {
					ev.RequestID = legacy.RequestID
					ev.Status = legacy.Status
					ev.Message = legacy.Message
				}
			}
			if ev.RequestID == "" {
				log.Printf("Kafka message missing request_id (value=%s)", string(msg.Value))
				continue
			}

			notificationHub.deliver(ev)
		}
	}()

	return reader
}

func waitTimeout() time.Duration {
	if raw := os.Getenv("NOTIFICATION_WAIT_MS"); raw != "" {
		if ms, err := strconv.Atoi(raw); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return waitTimeoutDefault
}

func WaitForNotification(c *fiber.Ctx) error {
	requestID := strings.TrimSpace(c.Query("request_id"))
	if requestID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "request_id is required"})
	}

	ttl := waitTimeout()

	ch := notificationHub.register(requestID)
	defer notificationHub.unregister(requestID)

	select {
	case ev, ok := <-ch:
		if ok {
			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"status":     "done",
				"request_id": requestID,
				"event":      ev,
			})
		}
	case <-time.After(ttl):
	case <-c.Context().Done():
	}

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"status":     "queued",
		"request_id": requestID,
		"timeout_ms": int(ttl.Milliseconds()),
	})
}

func main() {
	kafkaReader := startKafkaNotificationListener()
	defer func() {
		if err := kafkaReader.Close(); err != nil {
			log.Printf("Failed to close Kafka reader: %v", err)
		}
	}()

	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "*",
		AllowMethods: "*",
	}))

	app.Get("/notifications/wait", WaitForNotification)

	log.Fatal(app.Listen(":3002"))
}
