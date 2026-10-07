package test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/goyourt/yogourt/services"
	"github.com/goyourt/yogourt/services/providers"
)

func TestTokenProvider(t *testing.T) {
	stringForToken := "test"
	token, err := services.CreateToken(stringForToken)

	if err != nil {
		t.Errorf("Error creating token: %v", err)
	}
	if token == "" {
		t.Error("Token is empty")
	}
	if _, err = services.ValidToken(token); err != nil {
		t.Errorf("Token is not valid: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	extractedToken, err := services.GetRequestToken(c)
	if err != nil {
		t.Errorf("Error extracting token from request: %v", err)
	}
	if extractedToken != token {
		t.Errorf("Miss match between created and extracted token : exceped %v, got %v", token, extractedToken)
	}
}

func TestValidTokenRejectsOtherAlgorithms(t *testing.T) {
	config := providers.GetMainConfig()

	forgedToken := jwt.NewWithClaims(jwt.SigningMethodHS384, jwt.MapClaims{
		"uuid": "11111111-1111-1111-1111-111111111111",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})

	signed, err := forgedToken.SignedString([]byte(config.Security.SecretKey))
	if err != nil {
		t.Fatalf("Error signing forged token: %v", err)
	}

	if _, err := services.ValidToken(signed); err == nil {
		t.Error("expected a token signed with a non-HS256 algorithm to be rejected")
	}
}

func TestValidateSecretKey(t *testing.T) {
	if err := services.ValidateSecretKey(""); err == nil {
		t.Error("expected an empty secret key to be rejected")
	}

	if err := services.ValidateSecretKey("too-short"); err == nil {
		t.Error("expected a secret key shorter than 32 bytes to be rejected")
	}

	if err := services.ValidateSecretKey("this-secret-key-is-at-least-32-bytes-long"); err != nil {
		t.Errorf("expected a 32+ byte secret key to be accepted, got: %v", err)
	}
}

func TestGetUUIDClaim(t *testing.T) {
	validUuid := "11111111-1111-1111-1111-111111111111"
	token, err := services.CreateToken(validUuid)
	if err != nil {
		t.Fatalf("Error creating token: %v", err)
	}

	parsedToken, err := services.ValidToken(token)
	if err != nil {
		t.Fatalf("Error validating token: %v", err)
	}

	uuid, err := services.GetUUIDClaim(parsedToken, "uuid")
	if err != nil {
		t.Errorf("expected a valid uuid claim to be accepted, got: %v", err)
	}
	if uuid != validUuid {
		t.Errorf("expected uuid %v, got %v", validUuid, uuid)
	}
}

func TestGetUUIDClaimRejectsInvalidUUID(t *testing.T) {
	token, err := services.CreateToken("not-a-uuid")
	if err != nil {
		t.Fatalf("Error creating token: %v", err)
	}

	parsedToken, err := services.ValidToken(token)
	if err != nil {
		t.Fatalf("Error validating token: %v", err)
	}

	if _, err := services.GetUUIDClaim(parsedToken, "uuid"); err == nil {
		t.Error("expected a non-UUID uuid claim to be rejected")
	}
}
