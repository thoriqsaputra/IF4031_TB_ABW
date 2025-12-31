package main

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"models"
)

var DB *gorm.DB

func ConnectDB() {
	var err error

	// pindahin somewhere secure maybe?
	dsn := "host=localhost user=postgres password=postgres dbname=agarthan port=5432 sslmode=disable"
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	err = DB.AutoMigrate(&models.User{}, &models.Role{}, &models.Department{},
		&models.Report{}, &models.ReportCategory{}, &models.ReportMedia{},
		&models.ReportAssignment{}, &models.Upvote{}, &models.Escalation{},
		&models.Performance{})
	if err != nil {
		log.Fatal("Failed to migrate database:", err)
	}
	log.Println("Database ceonnected")
}

func main() {
	ConnectDB()

	app := fiber.New()

	log.Fatal(app.Listen(":3000"))
}
