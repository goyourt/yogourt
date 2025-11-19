package services

import (
	"context"
	"fmt"
	"time"

	"github.com/goyourt/yogourt/services/providers"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

func GetHashedPassword(pwd string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(pwd), 14)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

func GetPasswordFailureCount(username string) (int, error) {
	ctx := context.Background()
	cache := providers.GetCache()
	since := float64(time.Now().Add(-24 * time.Hour).Unix())

	attempts, err := cache.ZRangeByScore(ctx, username, &redis.ZRangeBy{
		Min: fmt.Sprintf("%f", since),
		Max: "+inf",
	}).Result()

	return len(attempts), err
}

func SavePasswordFailure(username string) error {
	ctx := context.Background()
	cache := providers.GetCache()
	now := float64(time.Now().Unix())

	err := cache.ZAdd(ctx, username, redis.Z{
		Score:  now,
		Member: now,
	}).Err()
	return err
}
