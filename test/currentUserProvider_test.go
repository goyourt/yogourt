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

func TestGetCurrentUserUnexpectedType(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	c.Set(providers.ContextCurrentUser, "not-a-base-interface")

	if user := providers.GetCurrentUser(c); user != nil {
		t.Errorf("expected nil user for an unexpected type, got %v", user)
	}
}
