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

func CreateUser(c *fiber.Ctx) error {
	user := new(models.User)

	if err := c.BodyParser(user); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := DB.Create(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(user)
}

func GetUsers(c *fiber.Ctx) error {
	var users []models.User

	DB.Find(&users)

	return c.JSON(users)
}

func main() {
	ConnectDB()

	app := fiber.New()

	app.Post("/users", CreateUser)
	app.Get("/users", GetUsers)

	log.Fatal(app.Listen(":3000"))
}
