package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
	"unicode"

	"github.com/goyourt/yogourt/services/providers"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

const defaultCost = 12

const (
	passwordFailureWindow     = 24 * time.Hour
	passwordFailureTTLSeconds = int(passwordFailureWindow / time.Second)
	passwordFailureKeyPrefix  = "yogourt:password-failures:sha256:"
)

var (
	recordPasswordFailureScript = redis.NewScript(`
local now = redis.call("TIME")
local score = tonumber(now[1]) + tonumber(now[2]) / 1000000
local cutoff = string.format("%.6f", score - tonumber(ARGV[1]))

redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", "(" .. cutoff)
redis.call("ZADD", KEYS[1], score, ARGV[2])
redis.call("EXPIRE", KEYS[1], ARGV[1])

return redis.call("ZCARD", KEYS[1])
`)

	getPasswordFailureCountScript = redis.NewScript(`
local now = redis.call("TIME")
local score = tonumber(now[1]) + tonumber(now[2]) / 1000000
local cutoff = string.format("%.6f", score - tonumber(ARGV[1]))

redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", "(" .. cutoff)
local count = redis.call("ZCARD", KEYS[1])
if count == 0 then
	redis.call("DEL", KEYS[1])
end

return count
`)
)

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

// GetPasswordFailureCount returns the number of failures recorded in the last
// 24 hours for username. It retains the original background-context API; new
// callers that have a request context should use GetPasswordFailureCountContext.
func GetPasswordFailureCount(username string) (int, error) {
	return GetPasswordFailureCountContext(context.Background(), username)
}

// GetPasswordFailureCountContext removes expired events and counts the
// remaining events on Redis. It never downloads the event members to count
// them, and does not extend the key TTL.
func GetPasswordFailureCountContext(ctx context.Context, username string) (int, error) {
	cache, err := providers.GetCache()
	if err != nil {
		return 0, err
	}

	return getPasswordFailureCount(ctx, cache, username)
}

// SavePasswordFailure records one failed password attempt. It retains the
// original background-context API; callers that also need the resulting count
// should use RecordPasswordFailure with their request context.
func SavePasswordFailure(username string) error {
	_, err := RecordPasswordFailure(context.Background(), username)
	return err
}

// RecordPasswordFailure atomically prunes failures outside the rolling 24-hour
// window, records one unique event, sets the key TTL, and returns the count.
// The returned count is a measurement, not an application admission decision.
func RecordPasswordFailure(ctx context.Context, username string) (int, error) {
	cache, err := providers.GetCache()
	if err != nil {
		return 0, err
	}

	return recordPasswordFailure(ctx, cache, username)
}

func recordPasswordFailure(ctx context.Context, cache redis.Scripter, username string) (int, error) {
	eventID, err := passwordFailureEventID()
	if err != nil {
		return 0, err
	}

	count, err := recordPasswordFailureScript.Run(ctx, cache, []string{passwordFailureKey(username)}, passwordFailureTTLSeconds, eventID).Int()
	if err != nil {
		return 0, err
	}

	return count, nil
}

func getPasswordFailureCount(ctx context.Context, cache redis.Scripter, username string) (int, error) {
	count, err := getPasswordFailureCountScript.Run(ctx, cache, []string{passwordFailureKey(username)}, passwordFailureTTLSeconds).Int()
	if err != nil {
		return 0, err
	}

	return count, nil
}

func passwordFailureKey(username string) string {
	sum := sha256.Sum256([]byte(username))
	return passwordFailureKeyPrefix + hex.EncodeToString(sum[:])
}

func passwordFailureEventID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate password failure event ID: %w", err)
	}

	return hex.EncodeToString(id[:]), nil
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
