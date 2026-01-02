package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"mime/multipart"
	"models"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/nfnt/resize"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/segmentio/kafka-go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Global connections
var (
	DB          *gorm.DB
	MinioClient *minio.Client
	KafkaWriter *kafka.Writer
)

// Configuration
const (
	MaxFileSize      = 50 * 1024 * 1024 // 50MB
	ThumbnailWidth   = 300
	ThumbnailQuality = 80
	BucketName       = "media-uploads"
)

// Prometheus metrics
var (
	uploadCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "media_uploads_total",
			Help: "Total number of media uploads",
		},
		[]string{"status", "media_type"},
	)
	uploadDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "media_upload_duration_seconds",
			Help:    "Duration of media upload operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"media_type"},
	)
	thumbnailCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "media_thumbnail_processed_total",
			Help: "Total number of thumbnails processed",
		},
		[]string{"status"},
	)
	activeThumbnailJobs = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "media_thumbnail_jobs_active",
			Help: "Number of active thumbnail processing jobs",
		},
	)
)

func init() {
	prometheus.MustRegister(uploadCounter)
	prometheus.MustRegister(uploadDuration)
	prometheus.MustRegister(thumbnailCounter)
	prometheus.MustRegister(activeThumbnailJobs)
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

	if err := DB.AutoMigrate(
		&models.ReportMedia{},
		&models.MediaProcessingJob{},
	); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	log.Println("✓ Database connected and migrated")
	return nil
}

// ConnectMinIO establishes MinIO connection
func ConnectMinIO() error {
	endpoint := os.Getenv("MINIO_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:9000"
	}

	accessKey := os.Getenv("MINIO_ACCESS_KEY")
	if accessKey == "" {
		accessKey = "minioadmin"
	}

	secretKey := os.Getenv("MINIO_SECRET_KEY")
	if secretKey == "" {
		secretKey = "minioadmin"
	}

	useSSL := os.Getenv("MINIO_USE_SSL") == "true"

	var err error
	MinioClient, err = minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize MinIO client: %w", err)
	}

	ctx := context.Background()
	exists, err := MinioClient.BucketExists(ctx, BucketName)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if !exists {
		err = MinioClient.MakeBucket(ctx, BucketName, minio.MakeBucketOptions{})
		if err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
		log.Printf("✓ Created bucket: %s\n", BucketName)
	}

	log.Println("✓ MinIO connected")
	return nil
}

// ConnectKafka establishes Kafka connection
func ConnectKafka() error {
	brokers := strings.Split(os.Getenv("KAFKA_BROKERS"), ",")
	if len(brokers) == 0 || brokers[0] == "" {
		brokers = []string{"localhost:9092"}
	}

	topic := os.Getenv("KAFKA_MEDIA_TOPIC")
	if topic == "" {
		topic = "media.events"
	}

	KafkaWriter = &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Topic:                  topic,
		Balancer:               &kafka.LeastBytes{},
		AllowAutoTopicCreation: true,
		Async:                  true,
	}

	log.Println("✓ Kafka writer initialized")
	return nil
}

// validateFile validates uploaded file
func validateFile(fileHeader *multipart.FileHeader) error {
	if fileHeader.Size > MaxFileSize {
		return fmt.Errorf("file size exceeds maximum limit of %d bytes", MaxFileSize)
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	allowedExtensions := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".webp": true,
		".mp4":  true,
		".mov":  true,
		".avi":  true,
	}

	if !allowedExtensions[ext] {
		return fmt.Errorf("file type %s not allowed", ext)
	}

	allowedMimes := map[string]bool{
		"image/jpeg":      true,
		"image/png":       true,
		"image/gif":       true,
		"image/webp":      true,
		"video/mp4":       true,
		"video/quicktime": true,
		"video/x-msvideo": true,
	}

	contentType := fileHeader.Header.Get("Content-Type")
	if !allowedMimes[contentType] {
		return fmt.Errorf("MIME type %s not allowed", contentType)
	}

	return nil
}

// uploadToMinIO uploads file to MinIO and returns object key
func uploadToMinIO(ctx context.Context, file io.Reader, filename string, contentType string, size int64) (string, error) {
	objectKey := fmt.Sprintf("%s-%s", uuid.New().String(), filename)

	_, err := MinioClient.PutObject(
		ctx,
		BucketName,
		objectKey,
		file,
		size,
		minio.PutObjectOptions{
			ContentType: contentType,
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to upload to MinIO: %w", err)
	}

	return objectKey, nil
}

// processThumbnailAsync processes thumbnail generation asynchronously
func processThumbnailAsync(jobID uint, originalKey string) {
	activeThumbnailJobs.Inc()
	defer activeThumbnailJobs.Dec()

	ctx := context.Background()
	log.Printf("Starting thumbnail processing for job %d (key: %s)\n", jobID, originalKey)

	DB.Model(&models.MediaProcessingJob{}).
		Where("job_id = ?", jobID).
		Updates(map[string]interface{}{
			"status": "processing",
		})

	obj, err := MinioClient.GetObject(ctx, BucketName, originalKey, minio.GetObjectOptions{})
	if err != nil {
		handleThumbnailError(jobID, originalKey, fmt.Sprintf("failed to download original: %v", err))
		return
	}
	defer obj.Close()

	img, format, err := image.Decode(obj)
	if err != nil {
		handleThumbnailError(jobID, originalKey, fmt.Sprintf("failed to decode image: %v", err))
		return
	}

	thumbnail := resize.Resize(ThumbnailWidth, 0, img, resize.Lanczos3)

	var buf bytes.Buffer
	switch format {
	case "jpeg", "jpg":
		err = jpeg.Encode(&buf, thumbnail, &jpeg.Options{Quality: ThumbnailQuality})
	case "png":
		err = png.Encode(&buf, thumbnail)
	default:
		err = jpeg.Encode(&buf, thumbnail, &jpeg.Options{Quality: ThumbnailQuality})
	}

	if err != nil {
		handleThumbnailError(jobID, originalKey, fmt.Sprintf("failed to encode thumbnail: %v", err))
		return
	}

	thumbnailKey := fmt.Sprintf("thumb_%s", originalKey)
	_, err = MinioClient.PutObject(
		ctx,
		BucketName,
		thumbnailKey,
		bytes.NewReader(buf.Bytes()),
		int64(buf.Len()),
		minio.PutObjectOptions{
			ContentType: "image/jpeg",
		},
	)
	if err != nil {
		handleThumbnailError(jobID, originalKey, fmt.Sprintf("failed to upload thumbnail: %v", err))
		return
	}

	now := time.Now()
	DB.Model(&models.MediaProcessingJob{}).
		Where("job_id = ?", jobID).
		Updates(map[string]interface{}{
			"status":        "completed",
			"thumbnail_key": thumbnailKey,
			"processed_at":  &now,
		})

	event := models.MediaProcessingEvent{
		JobID:        jobID,
		Status:       "completed",
		OriginalKey:  originalKey,
		ThumbnailKey: thumbnailKey,
	}

	publishKafkaEvent(event)

	thumbnailCounter.WithLabelValues("success").Inc()
	log.Printf("✓ Thumbnail processing completed for job %d\n", jobID)
}

// handleThumbnailError handles thumbnail processing errors
func handleThumbnailError(jobID uint, originalKey, errorMsg string) {
	log.Printf("✗ Thumbnail processing failed for job %d: %s\n", jobID, errorMsg)

	DB.Model(&models.MediaProcessingJob{}).
		Where("job_id = ?", jobID).
		Updates(map[string]interface{}{
			"status":        "failed",
			"error_message": errorMsg,
		})

	event := models.MediaProcessingEvent{
		JobID:        jobID,
		Status:       "failed",
		OriginalKey:  originalKey,
		ErrorMessage: errorMsg,
	}

	publishKafkaEvent(event)
	thumbnailCounter.WithLabelValues("error").Inc()
}

// publishKafkaEvent publishes event to Kafka
func publishKafkaEvent(event models.MediaProcessingEvent) {
	msg := kafka.Message{
		Key:   []byte(fmt.Sprintf("%d", event.JobID)),
		Value: []byte(fmt.Sprintf(`{"job_id":%d,"status":"%s","original_key":"%s","thumbnail_key":"%s","error_message":"%s"}`, event.JobID, event.Status, event.OriginalKey, event.ThumbnailKey, event.ErrorMessage)),
	}

	err := KafkaWriter.WriteMessages(context.Background(), msg)
	if err != nil {
		log.Printf("Failed to publish Kafka event: %v\n", err)
	}
}

// UploadHandler handles file upload
func UploadHandler(c *fiber.Ctx) error {
	timer := prometheus.NewTimer(uploadDuration.WithLabelValues("unknown"))
	defer timer.ObserveDuration()

	file, err := c.FormFile("file")
	if err != nil {
		uploadCounter.WithLabelValues("error", "unknown").Inc()
		return c.Status(400).JSON(fiber.Map{
			"error": "No file uploaded",
		})
	}

	reportID := c.FormValue("report_id")
	if reportID == "" {
		uploadCounter.WithLabelValues("error", "unknown").Inc()
		return c.Status(400).JSON(fiber.Map{
			"error": "report_id is required",
		})
	}

	if err := validateFile(file); err != nil {
		uploadCounter.WithLabelValues("error", "validation").Inc()
		return c.Status(400).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	mediaType := "image"
	if strings.HasPrefix(file.Header.Get("Content-Type"), "video/") {
		mediaType = "video"
	}

	timer = prometheus.NewTimer(uploadDuration.WithLabelValues(mediaType))

	fileContent, err := file.Open()
	if err != nil {
		uploadCounter.WithLabelValues("error", mediaType).Inc()
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to open file",
		})
	}
	defer fileContent.Close()

	objectKey, err := uploadToMinIO(
		c.Context(),
		fileContent,
		file.Filename,
		file.Header.Get("Content-Type"),
		file.Size,
	)
	if err != nil {
		uploadCounter.WithLabelValues("error", mediaType).Inc()
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var reportIDUint uint
	fmt.Sscanf(reportID, "%d", &reportIDUint)

	reportMedia := models.ReportMedia{
		MediaType: mediaType,
		ObjectKey: objectKey,
		CreatedAt: time.Now(),
		ReportID:  reportIDUint,
	}

	if err := DB.Create(&reportMedia).Error; err != nil {
		uploadCounter.WithLabelValues("error", mediaType).Inc()
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to save media metadata",
		})
	}

	var jobID uint
	if mediaType == "image" {
		job := models.MediaProcessingJob{
			OriginalKey:   objectKey,
			Status:        "pending",
			CreatedAt:     time.Now(),
			ReportMediaID: reportMedia.ReportMediaID,
		}

		if err := DB.Create(&job).Error; err != nil {
			log.Printf("Failed to create thumbnail job: %v\n", err)
		} else {
			jobID = job.JobID
			go processThumbnailAsync(job.JobID, objectKey)
		}
	}

	uploadCounter.WithLabelValues("success", mediaType).Inc()

	presignedURL, _ := MinioClient.PresignedGetObject(
		c.Context(),
		BucketName,
		objectKey,
		time.Hour,
		nil,
	)

	response := models.MediaUploadResponse{
		ReportMediaID: reportMedia.ReportMediaID,
		ObjectKey:     objectKey,
		MediaType:     mediaType,
		URL:           presignedURL.String(),
	}

	if jobID > 0 {
		response.JobID = jobID
	}

	return c.JSON(response)
}

// GetMediaHandler retrieves media info by ID
func GetMediaHandler(c *fiber.Ctx) error {
	id := c.Params("id")

	var media models.ReportMedia
	if err := DB.First(&media, id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Media not found",
		})
	}

	presignedURL, err := MinioClient.PresignedGetObject(
		c.Context(),
		BucketName,
		media.ObjectKey,
		time.Hour,
		nil,
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to generate download URL",
		})
	}

	return c.JSON(fiber.Map{
		"report_media_id": media.ReportMediaID,
		"media_type":      media.MediaType,
		"object_key":      media.ObjectKey,
		"url":             presignedURL.String(),
		"created_at":      media.CreatedAt,
	})
}

// GetReportMediaHandler returns all media for a specific report
func GetReportMediaHandler(c *fiber.Ctx) error {
	reportID := c.Params("report_id")

	var mediaList []models.ReportMedia
	if err := DB.Where("report_id = ?", reportID).Find(&mediaList).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to fetch media",
		})
	}

	// Generate presigned URLs for each media
	results := make([]fiber.Map, 0)
	for _, media := range mediaList {
		presignedURL, err := MinioClient.PresignedGetObject(
			c.Context(),
			BucketName,
			media.ObjectKey,
			time.Hour,
			nil,
		)
		if err != nil {
			log.Printf("Failed to generate URL for media %d: %v\n", media.ReportMediaID, err)
			continue
		}

		results = append(results, fiber.Map{
			"report_media_id": media.ReportMediaID,
			"media_type":      media.MediaType,
			"object_key":      media.ObjectKey,
			"url":             presignedURL.String(),
			"created_at":      media.CreatedAt,
		})
	}

	return c.JSON(fiber.Map{
		"media": results,
		"count": len(results),
	})
}

// HealthHandler returns service health status
func HealthHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "healthy",
		"service": "media_service",
		"timestamp": time.Now(),
	})
}

func main() {
	log.Println("Starting Media Service...")

	if err := ConnectDB(); err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}

	if err := ConnectMinIO(); err != nil {
		log.Fatalf("MinIO connection failed: %v", err)
	}

	if err := ConnectKafka(); err != nil {
		log.Fatalf("Kafka connection failed: %v", err)
	}

	app := fiber.New(fiber.Config{
		BodyLimit: MaxFileSize,
	})

	app.Use(recover.New())
	app.Use(cors.New())

	app.Get("/health", HealthHandler)
	app.Post("/upload", UploadHandler)
	app.Get("/media/:id", GetMediaHandler)
	app.Get("/reports/:report_id/media", GetReportMediaHandler)

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

	port := os.Getenv("PORT")
	if port == "" {
		port = "3002"
	}

	log.Printf("Media Service listening on port %s\n", port)
	if err := app.Listen(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
