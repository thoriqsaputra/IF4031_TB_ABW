package database

import (
	"context"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var Rdb *redis.Client
var Ctx = context.Background()

func ConnectRedis() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "redis:6379" // Default to Docker service name
	}

	Rdb = redis.NewClient(&redis.Options{
		Addr: redisAddr,
		DB:   0,
	})
}

func BlacklistToken(token string, duration time.Duration) error {
	if Rdb == nil {
		return nil // Gracefully handle when Redis is not available
	}
	return Rdb.Set(Ctx, token, "blacklisted", duration).Err()
}

func IsTokenBlacklisted(token string) bool {
	if Rdb == nil {
		return false // If Redis is not available, don't block the request
	}
	val, err := Rdb.Get(Ctx, token).Result()
	return err == nil && val == "blacklisted"
}
