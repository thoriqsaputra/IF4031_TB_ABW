package main

import (
	"database"
	"log"
	"middleware"
	"models"
	"os"
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

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=postgres dbname=agarthan port=5432 sslmode=disable"
	}
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	DB.AutoMigrate(&models.User{}, &models.Role{}, &models.Department{}, &models.ReportCategory{})
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

func GetCategories(c *fiber.Ctx) error {
	var categories []models.ReportCategory
	DB.Order("department_id, report_categories_id").Find(&categories)
	return c.JSON(categories)
}

func SeedDatabase() {
	roles := []models.Role{
		{RoleID: 1, Name: "citizen"},
		{RoleID: 2, Name: "government"},
		{RoleID: 3, Name: "admin"},
	}
	for _, role := range roles {
		DB.FirstOrCreate(&role, models.Role{RoleID: role.RoleID})
	}

	national := models.Department{DepartmentID: 10000, Name: "Kementerian Pusat", ParentID: nil}
	DB.FirstOrCreate(&national, models.Department{DepartmentID: 10000})

	parentIDNational := uint(10000)
	provincial := models.Department{DepartmentID: 1000, Name: "Dinas Provinsi", ParentID: &parentIDNational}
	DB.FirstOrCreate(&provincial, models.Department{DepartmentID: 1000})

	parentIDProvincial := uint(1000)
	city := models.Department{DepartmentID: 100, Name: "Dinas Kota", ParentID: &parentIDProvincial}
	DB.FirstOrCreate(&city, models.Department{DepartmentID: 100})

	parentIDCity := uint(100)
	district := models.Department{DepartmentID: 10, Name: "Kecamatan", ParentID: &parentIDCity}
	DB.FirstOrCreate(&district, models.Department{DepartmentID: 10})

	parentIDDistrict := uint(10)
	village := models.Department{DepartmentID: 1, Name: "Kelurahan", ParentID: &parentIDDistrict}
	DB.FirstOrCreate(&village, models.Department{DepartmentID: 1})

	// Seed Categories linked to Departments
	categories := []models.ReportCategory{
		// Kelurahan (Village) - Department ID: 1
		{ReportCategoryID: 1, Name: "Kebersihan Lingkungan", DepartmentID: 1},
		{ReportCategoryID: 2, Name: "Kerusakan Jalan", DepartmentID: 1},
		{ReportCategoryID: 3, Name: "Lampu Jalan Rusak", DepartmentID: 1},
		{ReportCategoryID: 4, Name: "Sampah Menumpuk", DepartmentID: 1},

		// Kecamatan (District) - Department ID: 10
		{ReportCategoryID: 5, Name: "Drainase Tersumbat", DepartmentID: 10},
		{ReportCategoryID: 6, Name: "Taman Tidak Terawat", DepartmentID: 10},
		{ReportCategoryID: 7, Name: "Fasilitas Umum Rusak", DepartmentID: 10},

		// Dinas Kota (City) - Department ID: 100
		{ReportCategoryID: 8, Name: "Macet Lalu Lintas", DepartmentID: 100},
		{ReportCategoryID: 9, Name: "Polusi Udara", DepartmentID: 100},
		{ReportCategoryID: 10, Name: "Banjir", DepartmentID: 100},
		{ReportCategoryID: 11, Name: "Penerangan Jalan Umum", DepartmentID: 100},

		// Dinas Provinsi (Provincial) - Department ID: 1000
		{ReportCategoryID: 12, Name: "Kerusakan Jalan Provinsi", DepartmentID: 1000},
		{ReportCategoryID: 13, Name: "Jembatan Rusak", DepartmentID: 1000},
		{ReportCategoryID: 14, Name: "Transportasi Publik", DepartmentID: 1000},
		{ReportCategoryID: 15, Name: "Pencemaran Sungai", DepartmentID: 1000},

		// Kementerian Pusat (National) - Department ID: 10000
		{ReportCategoryID: 16, Name: "Infrastruktur Nasional", DepartmentID: 10000},
		{ReportCategoryID: 17, Name: "Kebijakan Publik", DepartmentID: 10000},
		{ReportCategoryID: 18, Name: "Bencana Alam", DepartmentID: 10000},
		{ReportCategoryID: 19, Name: "Kesehatan Masyarakat", DepartmentID: 10000},
		{ReportCategoryID: 20, Name: "Pendidikan", DepartmentID: 10000},
	}

	// Use Updates instead of FirstOrCreate to ensure names are updated
	for _, category := range categories {
		DB.Where("report_categories_id = ?", category.ReportCategoryID).
			Assign(models.ReportCategory{
				Name:         category.Name,
				DepartmentID: category.DepartmentID,
			}).
			FirstOrCreate(&category)
	}

	log.Println("Database seeded: roles, departments, and categories")
}

// example of protected route
func Profile(c *fiber.Ctx) error {
	userID := c.Locals("user_id")

	var user models.User
	// Enable debug mode to see SQL queries
	result := DB.Debug().Preload("Role").Preload("Department.Parent").First(&user, userID)
	if result.Error != nil {
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}

	// Debug logging
	log.Printf("=== Profile Debug ===")
	log.Printf("UserID from token: %v", userID)
	log.Printf("User loaded: UserID=%d, Email=%s, Name=%s", user.UserID, user.Email, user.Name)
	log.Printf("User.RoleID: %d", user.RoleID)
	log.Printf("User.Role: %+v", user.Role)
	log.Printf("User.DepartmentID: %v", user.DepartmentID)
	if user.Department != nil {
		log.Printf("User.Department: %+v", *user.Department)
	}

	return c.JSON(user)
}

func main() {
	ConnectDB()
	database.ConnectRedis()
	SeedDatabase()

	app := fiber.New()

	// app.Use(cors.New(cors.Config{
	// 	AllowOrigins: "http://localhost:3000",
	//     AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	// }))

	app.Post("/api/auth/register", Register)
	app.Post("/api/auth/login", Login)
	app.Get("/api/departments", GetDepartments)
	app.Get("/api/categories", GetCategories)

	app.Get("/api/auth/profile", middleware.Protected(), Profile)
	app.Post("/api/auth/logout", middleware.Protected(), Logout)

	log.Fatal(app.Listen(":3001"))
}
