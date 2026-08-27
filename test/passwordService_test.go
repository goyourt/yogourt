package test

import (
	"errors"
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

// TestCheckPasswordRoundTrip is the reason CheckPassword exists: an
// application must be able to verify a password without importing bcrypt
// itself.
func TestCheckPasswordRoundTrip(t *testing.T) {
	pwd := "password123"

	hashed, err := services.GetHashedPassword(pwd)
	if err != nil {
		t.Fatalf("Error hashing password: %v", err)
	}

	if err := services.CheckPassword(hashed, pwd); err != nil {
		t.Errorf("CheckPassword must accept the password it hashed, got %v", err)
	}
}

// TestCheckPasswordMatch uses a hash produced outside of the framework: a
// stored credential predating a hash_cost change must keep working.
func TestCheckPasswordMatch(t *testing.T) {
	// bcrypt hash of "password123", cost 10.
	const hashed = "$2a$10$3sn3CylFZm0PNnN3eUqzje2B2Bv9EgF3/GAxmUgTqS/TizJc8Idxq"

	if err := services.CheckPassword(hashed, "password123"); err != nil {
		t.Errorf("matching password must return nil, got %v", err)
	}
}

func TestCheckPasswordMismatch(t *testing.T) {
	hashed, err := services.GetHashedPassword("password123")
	if err != nil {
		t.Fatalf("Error hashing password: %v", err)
	}

	err = services.CheckPassword(hashed, "password124")
	if err == nil {
		t.Fatal("a wrong password must return an error")
	}
	if !errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		t.Errorf("expected a mismatch error, got %v", err)
	}
}

func TestCheckPasswordEmptyPassword(t *testing.T) {
	hashed, err := services.GetHashedPassword("password123")
	if err != nil {
		t.Fatalf("Error hashing password: %v", err)
	}

	if err := services.CheckPassword(hashed, ""); err == nil {
		t.Error("an empty password must never match a hash")
	}

	// An empty password is a legitimate input of the hash function; the round
	// trip must stay consistent.
	emptyHash, err := services.GetHashedPassword("")
	if err != nil {
		t.Fatalf("Error hashing empty password: %v", err)
	}
	if err := services.CheckPassword(emptyHash, ""); err != nil {
		t.Errorf("hash of an empty password must match it, got %v", err)
	}
	if err := services.CheckPassword(emptyHash, "password123"); err == nil {
		t.Error("hash of an empty password must not match another password")
	}
}

// TestCheckPasswordMalformedHash documents why the error must never reach the
// client: it tells a malformed hash apart from a wrong password.
func TestCheckPasswordMalformedHash(t *testing.T) {
	cases := map[string]string{
		"empty":       "",
		"plain text":  "password123",
		"truncated":   "$2a$12$4kJvXPZzP8Q0dCqJ7iRz1e",
		"bad version": "$9z$12$4kJvXPZzP8Q0dCqJ7iRz1e3ijMbaVYJmnk1yEXsBLtvlxCd7Mv4tK",
	}

	for name, hashed := range cases {
		err := services.CheckPassword(hashed, "password123")
		if err == nil {
			t.Errorf("%s hash must return an error", name)
			continue
		}
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			t.Errorf("%s hash must not be reported as a password mismatch", name)
		}
	}
}

// The test configuration declares no password policy, which is the state of
// every configuration that never wrote the security keys: IsPasswordValid
// then rejects the empty password and nothing else. The flags are documented
// as cumulative and off by default — this locks the "off" half.
func TestIsPasswordValidWithoutPolicy(t *testing.T) {
	if services.IsPasswordValid("") {
		t.Error("an empty password must be refused whatever the policy declares")
	}

	for _, pwd := range []string{"a", "password", "É"} {
		if !services.IsPasswordValid(pwd) {
			t.Errorf("IsPasswordValid(%q) = false, want true: no policy key is set", pwd)
		}
	}
}
