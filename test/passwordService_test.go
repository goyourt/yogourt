package test

import (
	"testing"

	"github.com/goyourt/yogourt/services"
	"golang.org/x/crypto/bcrypt"
)

func TestPasswordService(t *testing.T) {
	pwd := "password123"
	hashed, err := services.GetHashedPassword(pwd)
	if err != nil {
		t.Errorf("Error hashing password: %v", err)
		return
	}

	if err = bcrypt.CompareHashAndPassword([]byte(hashed), []byte(pwd)); err != nil {
		t.Errorf("Password comparison failed: %v", err)
	}
}
