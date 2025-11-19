package services

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/goyourt/yogourt/services/providers"
)

func CreateToken(uuid string) (string, error) {
	config := providers.GetConfig()

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
	config := providers.GetConfig()
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(config.Security.SecretKey), nil
	})

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
