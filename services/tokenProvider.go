package services

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func CreateToken(username string) (string, error) {
	config := GetConfig()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{
			"username": username,
			"exp":      time.Now().Add(time.Minute * time.Duration(config.Security.TokenExpires)).Unix(),
		})

	tokenString, err := token.SignedString([]byte(config.Security.SecretKey))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func ValidToken(tokenString string) error {
	config := GetConfig()
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(config.Security.SecretKey), nil
	})

	if err != nil {
		return err
	}

	if !token.Valid {
		return fmt.Errorf("invalid token")
	}

	return nil
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
