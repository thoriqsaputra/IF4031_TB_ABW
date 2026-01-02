package database

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

var Rdb *redis.Client
var Ctx = context.Background()

func ConnectRedis() {
	Rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   0,
	})
}

func BlacklistToken(token string, duration time.Duration) error {
	return Rdb.Set(Ctx, token, "blacklisted", duration).Err()
}

func IsTokenBlacklisted(token string) bool {
	val, err := Rdb.Get(Ctx, token).Result()
	return err == nil && val == "blacklisted"
}
