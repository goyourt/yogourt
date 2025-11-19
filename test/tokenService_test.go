package test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/services"
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
	if err = services.ValidToken(token); err != nil {
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
