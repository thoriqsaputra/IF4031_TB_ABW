package models

import "time"

// Notification represents a user notification
type Notification struct {
	NotificationID uint      `gorm:"primaryKey;column:notification_id" json:"notification_id"`
	UserID         uint      `gorm:"not null;index" json:"user_id"`
	Type           string    `gorm:"not null" json:"type"` // report_created, report_assigned, report_updated, comment_added, etc.
	Title          string    `gorm:"not null" json:"title"`
	Message        string    `gorm:"type:text" json:"message"`
	IsRead         bool      `gorm:"default:false;index" json:"is_read"`
	CreatedAt      time.Time `json:"created_at"`
	ReportID *uint `json:"report_id,omitempty"`
	User   User    `gorm:"foreignKey:UserID"`
	Report *Report `gorm:"foreignKey:ReportID"`
}

// NotificationEvent represents incoming Kafka event for notifications
type NotificationEvent struct {
	Type      string                 `json:"type"`
	UserID    uint                   `json:"user_id"`
	ReportID  *uint                  `json:"report_id,omitempty"`
	Title     string                 `json:"title"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// NotificationStats represents notification statistics
type NotificationStats struct {
	TotalNotifications   int64 `json:"total_notifications"`
	UnreadNotifications  int64 `json:"unread_notifications"`
}
