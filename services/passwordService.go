package services

import (
	"context"
	"fmt"
	"time"
	"unicode"

	"github.com/goyourt/yogourt/services/providers"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const defaultCost = 12

func GetHashedPassword(pwd string) (string, error) {
	cfg := providers.GetConfig().Security
	cost := cfg.HashCost

	if cost == 0 {
		cost = defaultCost
	}

	bytes, err := bcrypt.GenerateFromPassword([]byte(pwd), cost)
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

func IsPasswordValid(pwd string) bool {
	cfg := providers.GetConfig().Security

	if len(pwd) < cfg.PasswordMinimumLength {
		return false
	}
	if cfg.PasswordNumberRequired && !containsNumber(pwd) {
		return false
	}
	if cfg.PasswordSpacialCharRequired && !containsSpecialChar(pwd) {
		return false
	}
	if cfg.PasswordUpperCaseRequired && !containsUppercase(pwd) {
		return false
	}
	if cfg.PasswordLowerCaseRequired && !containsLowercase(pwd) {
		return false
	}

	return true
}

func containsNumber(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func containsSpecialChar(s string) bool {
	for _, r := range s {
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return true
		}
	}
	return false
}

func containsUppercase(s string) bool {
	for _, r := range s {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

func containsLowercase(s string) bool {
	for _, r := range s {
		if unicode.IsLower(r) {
			return true
		}
	}
	return false
}
