package services

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/goyourt/yogourt/services/providers"
)

// minSecretKeyLength is the minimum acceptable length, in bytes, for the JWT
// signing secret. A short secret makes HMAC signatures brute-forceable.
const minSecretKeyLength = 32

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidateSecretKey rejects an empty or too short JWT secret.
func ValidateSecretKey(secret string) error {
	if secret == "" {
		return fmt.Errorf("JWT secret key must not be empty")
	}
	if len(secret) < minSecretKeyLength {
		return fmt.Errorf("JWT secret key must be at least %d bytes long", minSecretKeyLength)
	}
	return nil
}

func CreateToken(uuid string) (string, error) {
	config := providers.GetMainConfig()

	if err := ValidateSecretKey(config.Security.SecretKey); err != nil {
		return "", err
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{
			"uuid": uuid,
			"exp":  time.Now().Add(time.Minute * time.Duration(config.Security.TokenExpires)).Unix(),
		})

	tokenString, err := token.SignedString([]byte(config.Security.SecretKey))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func ValidToken(tokenString string) (*jwt.Token, error) {
	config := providers.GetMainConfig()

	if err := ValidateSecretKey(config.Security.SecretKey); err != nil {
		return nil, err
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(config.Security.SecretKey), nil
	}, jwt.WithValidMethods([]string{"HS256"}))

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return token, nil
}

func GetRequestToken(c *gin.Context) (string, error) {
	const prefix = "Bearer "
	authHeader := c.GetHeader("Authorization")

	if authHeader == "" {
		return "", fmt.Errorf("missing authorization header")
	}

	if !strings.HasPrefix(authHeader, prefix) {
		return "", fmt.Errorf("invalid token format")
	}

	return strings.TrimPrefix(authHeader, prefix), nil
}

func GetClaim(token *jwt.Token, claimKey string) (any, error) {
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		if claimValue, exists := claims[claimKey]; exists {

			return claimValue, nil
		}
		return nil, fmt.Errorf("Value not found in token : %s", claimKey)
	}
	return nil, fmt.Errorf("invalid token claims")
}

// GetUUIDClaim extracts claimKey as a string and validates that it is
// formatted as a UUID before returning it.
func GetUUIDClaim(token *jwt.Token, claimKey string) (string, error) {
	claimValue, err := GetClaim(token, claimKey)
	if err != nil {
		return "", err
	}

	uuidValue, ok := claimValue.(string)
	if !ok {
		return "", fmt.Errorf("claim %s is not a string", claimKey)
	}

	if !uuidPattern.MatchString(uuidValue) {
		return "", fmt.Errorf("claim %s is not a valid UUID", claimKey)
	}

	return uuidValue, nil
}
