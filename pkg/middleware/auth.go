package middleware

import (
	"database"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// ganti nanti pake env
var SecretKey = []byte("i_love_furina_so_much")

func Protected() fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		log.Printf("DEBUG Middleware: Authorization header = '%s'", authHeader)
		if authHeader == "" {
			log.Printf("DEBUG Middleware: Missing token")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing token"})
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			log.Printf("DEBUG Middleware: Invalid token format")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token"})
		}

		tokenString := parts[1]

		if database.IsTokenBlacklisted(tokenString) {
			log.Printf("DEBUG Middleware: Token blacklisted")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "You are logged out"})
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fiber.ErrUnauthorized
			}
			return SecretKey, nil
		})

		if err != nil || !token.Valid {
			log.Printf("DEBUG Middleware: Invalid token - %v", err)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token"})
		}

		claims := token.Claims.(jwt.MapClaims)

		c.Locals("user_id", claims["user_id"])
		c.Locals("role", claims["role"])
		log.Printf("DEBUG Middleware: Token valid, user_id=%v, role=%v", claims["user_id"], claims["role"])

		return c.Next()
	}
}
