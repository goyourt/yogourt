package test

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/services/providers"
)

func TestGetCurrentUserMissing(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)

	if user := providers.GetCurrentUser(c); user != nil {
		t.Errorf("expected nil user when none is set, got %v", user)
	}
}

func TestGetCurrentUserReturnsStoredValue(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	c.Set(providers.ContextCurrentUser, "any-value")

	if user := providers.GetCurrentUser(c); user != "any-value" {
		t.Errorf("expected the stored value back, got %v", user)
	}
}
