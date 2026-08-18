package services

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AUTHZ-014: an unknown user is an authentication refusal, a database outage
// is a technical failure — the two must never swap statuses.
func TestRespondUserLookupFailureUnknownUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	respondUserLookupFailure(c, gorm.ErrRecordNotFound)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("unknown user must get 401, got %d", w.Code)
	}
}

func TestRespondUserLookupFailureDatabaseOutage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	respondUserLookupFailure(c, errors.New("connection refused"))

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("database outage must get 503, got %d", w.Code)
	}
}
