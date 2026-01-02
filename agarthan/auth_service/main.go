package main

import (
	"database"
	"log"
	"middleware"
	"models"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	// "github.com/gofiber/fiber/v2/middleware/cors"
)

var DB *gorm.DB

// ganti nanti pake env
var SecretKey = []byte("i_love_furina_so_much")

func ConnectDB() {
	var err error
	dsn := "host=localhost user=postgres password=postgres dbname=agarthan port=5432 sslmode=disable"
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	DB.AutoMigrate(&models.User{}, &models.Role{}, &models.Department{})
}

type RegisterInput struct {
	Name         string `json:"name"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	RoleID       uint   `json:"role_id"`
	DepartmentID uint   `json:"department_id"`
}

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func HashPassword(password string) (string, error) {
	bcryptedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bcryptedPassword), nil
}

func Register(c *fiber.Ctx) error {
	var input RegisterInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	hashedPassword, err := HashPassword(input.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	user := models.User{
		Name:     input.Name,
		Email:    input.Email,
		Password: string(hashedPassword),
		RoleID:   input.RoleID,
		// DepartmentID: input.DepartmentID,
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	// if 0 (citizen) then nil, otherwise they a government
	if input.DepartmentID > 0 {
		user.DepartmentID = &input.DepartmentID
	}

	if err := DB.Create(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "User registered successfully", "user": user})
}

func Login(c *fiber.Ctx) error {
	var input LoginInput
	var user models.User

	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	if err := DB.Preload("Role").Where("email = ?", input.Email).First(&user).Error; err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid email or password"})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid email or password"})
	}

	claims := jwt.MapClaims{
		"user_id": user.UserID,
		"role":    user.Role.Name,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(SecretKey)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "Login successful", "token": signedToken})
}

func Logout(c *fiber.Ctx) error {
	authHeader := c.Get("Authorization")
	if len(authHeader) < 8 {
		return c.Status(400).JSON(fiber.Map{"error": "Token tidak valid"})
	}
	tokenString := authHeader[7:] // gtfo bearer

	err := database.BlacklistToken(tokenString, 24*time.Hour)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal logout (Redis Error)"})
	}

	return c.JSON(fiber.Map{"message": "Berhasil logout"})
}

func GetDepartments(c *fiber.Ctx) error {
	var depts []models.Department
	DB.Preload("Parent").Find(&depts)
	return c.JSON(depts)
}

// example of protected route
func Profile(c *fiber.Ctx) error {
	userID := c.Locals("user_id")

	var user models.User
	if err := DB.Preload("Role").Preload("Department.Parent").First(&user, userID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}

	return c.JSON(user)
}

func main() {
	ConnectDB()
	database.ConnectRedis()

	app := fiber.New()

	// app.Use(cors.New(cors.Config{
	// 	AllowOrigins: "http://localhost:3000",
	//     AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	// }))

	app.Post("/api/auth/register", Register)
	app.Post("/api/auth/login", Login)
	app.Get("/api/departments", GetDepartments)

	app.Get("/api/auth/profile", middleware.Protected(), Profile)
	app.Post("/api/auth/logout", middleware.Protected(), Logout)

	log.Fatal(app.Listen(":3001"))
}
