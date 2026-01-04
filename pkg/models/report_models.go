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
	Report        Report    `gorm:"foreignKey:ReportID" json:"report,omitempty"`
}

type ReportAssignment struct {
	AssignmentID uint       `gorm:"primaryKey;column:report_assignment_id" json:"assignment_id"`
	Status       string     `json:"status"`
	AssignedAt   time.Time  `json:"assigned_at"`
	AssignedTo   uint       `json:"assigned_to"`
	ReportID     uint       `json:"report_id"`
	Response     string     `gorm:"type:text" json:"response"`
	RespondedAt  *time.Time `json:"responded_at"`
}

type StatusChange struct {
	StatusChangeID uint      `gorm:"primaryKey;column:status_change_id;autoIncrement" json:"status_change_id"`
	ReportID       uint      `json:"report_id"`
	OldStatus      string    `json:"old_status"`
	NewStatus      string    `json:"new_status"`
	ChangedBy      uint      `json:"changed_by"`
	ChangedAt      time.Time `json:"changed_at"`
	Notes          string    `gorm:"type:text" json:"notes"`
}

type ReportResponse struct {
	ReportResponseID uint      `gorm:"primaryKey;column:report_response_id" json:"report_response_id"`
	Message          string    `gorm:"type:text" json:"message"`
	CreatedAt        time.Time `json:"created_at"`
	CreatedBy        uint      `json:"created_by"`
	ReportID         uint      `json:"report_id"`
	JobStatus		 string    `json:"job_status"`
}

type Report struct {
	ReportID            uint      `gorm:"primaryKey;column:report_id" json:"report_id"`
	Title               string    `json:"title"`
	Description         string    `gorm:"type:text" json:"description"`
	IsPublic            bool      `json:"is_public"`
	IsAnonymous         bool      `json:"is_anon"`
	Location            string    `json:"location"`
	Severity            string    `json:"severity"`
	Status              string    `gorm:"default:'pending'" json:"status"` // pending, in_progress, resolved, rejected
	AssignedTo          *uint     `json:"assigned_to"`                     // government official assigned to handle
	CurrentDepartmentID *uint     `gorm:"column:current_department_id" json:"current_department_id"` // current department handling the report (used for escalation)
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`

	// foreign keys
	ReportCategoryID uint `gorm:"column:report_categories_id" json:"report_categories_id"`
	UserID           uint `json:"user_id"`

	// relations
	User           User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	ReportCategory ReportCategory `gorm:"foreignKey:ReportCategoryID" json:"report_category,omitempty"`
	AssignedUser   *User          `gorm:"foreignKey:AssignedTo" json:"assigned_user,omitempty"`
}

type ReportRequestMessage struct {
	RequestID string `json:"request_id"`
	Report    Report `json:"report"`
}

type ReportResponseRequestMessage struct {
	RequestID string         `json:"request_id"`
	Response  ReportResponse `json:"response"`
}

type ReportPublishedEvent struct {
	ReportID         uint      `json:"report_id"`
	ReportCategoryID uint      `json:"report_categories_id"`
	PublishedAt      time.Time `json:"published_at"`
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

// Kafka Event Models for Notifications
type ReportStatusChangeEvent struct {
	ReportID    uint   `json:"report_id"`
	OldStatus   string `json:"old_status"`
	NewStatus   string `json:"new_status"`
	ChangedBy   uint   `json:"changed_by"`
	UserID      uint   `json:"user_id"`      // Original reporter
	IsAnonymous bool   `json:"is_anonymous"` // For privacy
	Title       string `json:"title"`
	Timestamp   string `json:"timestamp"`
}

type ReportAssignmentEvent struct {
	ReportID     uint   `json:"report_id"`
	AssignedTo   uint   `json:"assigned_to"`
	AssignedBy   uint   `json:"assigned_by"`
	DepartmentID uint   `json:"department_id"`
	Title        string `json:"title"`
	Severity     string `json:"severity"`
	Timestamp    string `json:"timestamp"`
}

type ReportEscalationEvent struct {
	ReportID         uint   `json:"report_id"`
	FromDepartmentID uint   `json:"from_department_id"`
	ToDepartmentID   uint   `json:"to_department_id"`
	Reason           string `json:"reason"`
	EscalatedBy      uint   `json:"escalated_by"`
	Title            string `json:"title"`
	Severity         string `json:"severity"`
	Timestamp        string `json:"timestamp"`
}

type ReportResponseEvent struct {
	ReportID     uint   `json:"report_id"`
	ResponseID   uint   `json:"response_id"`
	RespondedBy  uint   `json:"responded_by"`
	ReporterID   uint   `json:"reporter_id"` // The original report creator
	Title        string `json:"title"`
	IsAnonymous  bool   `json:"is_anonymous"`
	Timestamp    string `json:"timestamp"`
}
