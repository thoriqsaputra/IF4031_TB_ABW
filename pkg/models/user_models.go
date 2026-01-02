package models

import "time"

type Role struct {
	RoleID uint   `gorm:"primaryKey;column:role_id" json:"role_id"`
	Name   string `gorm:"type:varchar(50);not null" json:"name"`
}

type Department struct {
	DepartmentID uint   `gorm:"primaryKey;column:department_id" json:"department_id"`
	Name         string `gorm:"type:varchar(100);not null" json:"name"`

	// hierarki
	ParentID *uint       `json:"parent_id"`
	Parent   *Department `gorm:"foreignKey:ParentID" json:"parent,omitempty"`
}

type User struct {
	UserID    uint      `gorm:"primaryKey;column:user_id" json:"user_id"`
	Email     string    `gorm:"unique;not null" json:"email"`
	Password  string    `gorm:"not null" json:"-"`
	Name      string    `json:"name"`
	IsActive  bool      `gorm:"default:true" json:"is_active"`
	CreatedAt time.Time `json:"created_at"`

	RoleID       uint `json:"role_id"`
	DepartmentID *uint `json:"department_id"`

	Role       Role        `gorm:"foreignKey:RoleID;references:RoleID" json:"role"`
	Department *Department `gorm:"foreignKey:DepartmentID;references:DepartmentID" json:"department,omitempty"`
}

type Performance struct {
	PerformanceID   uint      `gorm:"primaryKey;column:performance_id" json:"performance_id"`
	HandledReports  int       `json:"handled_reports"`
	AvgResponseTime float64   `json:"avg_response_time"`
	Start           time.Time `json:"start"`
	End             time.Time `json:"end"`

	UserID uint `json:"user_id"`
}
