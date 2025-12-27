package models

import "time"

type ReportCategory struct {
	ReportCategoryID uint   `gorm:"primaryKey;column:report_categories_id" json:"report_categories_id"`
	Name             string `json:"name"`
	DepartmentID     uint   `json:"department_id"`
}

type ReportMedia struct {
	ReportMediaID uint      `gorm:"primaryKey;column:report_media_id" json:"report_media_id"`
	MediaType     string    `json:"media_type"`
	ObjectKey     string    `json:"object_key"`
	CreatedAt     time.Time `json:"created_at"`
	ReportID      uint      `json:"report_id"`
}

type ReportAssignment struct {
	AssignmentID uint      `gorm:"primaryKey;column:report_assignment_id" json:"assignment_id"`
	Status       string    `json:"status"`
	AssignedAt   time.Time `json:"assigned_at"`
	AssignedTo   uint      `json:"assigned_to"`
	ReportID     uint      `json:"report_id"`
}

type Report struct {
	ReportID    uint      `gorm:"primaryKey;column:report_id" json:"report_id"`
	Title       string    `json:"title"`
	Description string    `gorm:"type:text" json:"description"`
	IsPublic    bool      `json:"is_public"`
	IsAnonymous bool      `json:"is_anon"`
	Location    string    `json:"location"`
	Severity    string    `json:"severity"`
	CreatedAt   time.Time `json:"created_at"`

	// foreign keys
	ReportCategoryID uint `gorm:"column:report_categories_id" json:"report_categories_id"`
	UserID           uint `json:"user_id"`

	// relations
	User           User           `gorm:"foreignKey:UserID"`
	ReportCategory ReportCategory `gorm:"foreignKey:ReportCategoryID"`
}

type Upvote struct {
	UpvoteID  uint      `gorm:"primaryKey;column:upvote_id" json:"upvote_id"`
	CreatedAt time.Time `json:"created_at"`
	UserID    uint      `json:"user_id"`
	ReportID  uint      `json:"report_id"`
}

type Escalation struct {
	EscalationID     uint      `gorm:"primaryKey;column:escalation_id" json:"escalation_id"`
	FromDepartmentID uint      `json:"from_department_id"`
	ToDepartmentID   uint      `json:"to_department_id"`
	EscalatedAt      time.Time `json:"escalated_at"`
	Reason           string    `json:"reason"`
	ReportID         uint      `json:"report_id"`
}
