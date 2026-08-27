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
	cfg := providers.GetMainConfig().Security
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

// CheckPassword compares a bcrypt hash produced by GetHashedPassword with a
// clear-text password. It returns nil when they match, and bcrypt's error
// otherwise.
//
// The returned error MUST NEVER be forwarded to the client, not even as a
// message: it distinguishes a wrong password
// (bcrypt.ErrMismatchedHashAndPassword) from a malformed, truncated or
// wrongly-versioned hash (bcrypt.ErrHashTooShort,
// bcrypt.HashVersionTooNewError, bcrypt.InvalidCostError…). Exposing that
// difference tells an attacker whether the account exists and how its
// credential is stored. Log it server-side if useful, and answer the caller a
// single generic message such as "Invalid credentials" for every failure.
func CheckPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

func GetPasswordFailureCount(username string) (int, error) {
	ctx := context.Background()
	cache, err := providers.GetCache()
	if err != nil {
		return 0, err
	}
	since := float64(time.Now().Add(-24 * time.Hour).Unix())

	attempts, err := cache.ZRangeByScore(ctx, username, &redis.ZRangeBy{
		Min: fmt.Sprintf("%f", since),
		Max: "+inf",
	}).Result()

	return len(attempts), err
}

func SavePasswordFailure(username string) error {
	ctx := context.Background()
	cache, err := providers.GetCache()
	if err != nil {
		return err
	}
	now := float64(time.Now().Unix())

	return cache.ZAdd(ctx, username, redis.Z{
		Score:  now,
		Member: now,
	}).Err()
}

func IsPasswordValid(pwd string) bool {
	cfg := providers.GetMainConfig().Security

	if len(pwd) == 0 {
		return false
	}
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
