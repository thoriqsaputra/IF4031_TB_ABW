package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"models"
)

var DB *gorm.DB

func ConnectDB() {
	var err error

	// pindahin somewhere secure maybe?
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=postgres dbname=agarthan port=5432 sslmode=disable"
	}
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

type createUserInput struct {
	Name         string `json:"name"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	RoleID       uint   `json:"role_id"`
	DepartmentID uint   `json:"department_id"`
	IsActive     *bool  `json:"is_active"`
}

func CreateUser(c *fiber.Ctx) error {
	var input createUserInput

	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	user := models.User{
		Name:         input.Name,
		Email:        input.Email,
		Password:     input.Password,
		RoleID:       input.RoleID,
		DepartmentID: input.DepartmentID,
		IsActive:     isActive,
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
