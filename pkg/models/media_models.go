package models

import "time"

// MediaProcessingJob represents an async media processing task
type MediaProcessingJob struct {
	JobID         uint      `gorm:"primaryKey;column:job_id" json:"job_id"`
	OriginalKey   string    `gorm:"not null" json:"original_key"`
	ThumbnailKey  string    `json:"thumbnail_key"`
	Status        string    `gorm:"default:'pending'" json:"status"` // pending, processing, completed, failed
	ProcessedAt   *time.Time `json:"processed_at"`
	ErrorMessage  string    `gorm:"type:text" json:"error_message"`
	CreatedAt     time.Time `json:"created_at"`
	ReportMediaID uint      `json:"report_media_id"`
}

// MediaUploadRequest represents the upload request payload
type MediaUploadRequest struct {
	ReportID  uint   `json:"report_id" form:"report_id"`
	MediaType string `json:"media_type" form:"media_type"`
}

// MediaUploadResponse represents the upload response
type MediaUploadResponse struct {
	ReportMediaID uint   `json:"report_media_id"`
	ObjectKey     string `json:"object_key"`
	MediaType     string `json:"media_type"`
	URL           string `json:"url"`
	JobID         uint   `json:"job_id,omitempty"`
}

// MediaProcessingEvent represents Kafka event for media processing
type MediaProcessingEvent struct {
	JobID        uint   `json:"job_id"`
	Status       string `json:"status"`
	OriginalKey  string `json:"original_key"`
	ThumbnailKey string `json:"thumbnail_key,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}
