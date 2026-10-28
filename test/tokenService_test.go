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
	parsed, err := services.ValidToken(token)
	if err != nil {
		t.Errorf("Token is not valid: %v", err)
	}
	config := providers.GetMainConfig()
	for claim, want := range map[string]string{
		"iss": config.Security.TokenIssuer,
		"aud": config.Security.TokenAudience,
	} {
		got, claimErr := services.GetStringClaim(parsed, claim)
		if claimErr != nil || got != want {
			t.Errorf("claim %s = %q, %v; want %q", claim, got, claimErr, want)
		}
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
		"iss":  config.Security.TokenIssuer,
		"aud":  config.Security.TokenAudience,
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

func TestValidateTokenConfig(t *testing.T) {
	valid := *providers.GetMainConfig()
	if err := services.ValidateTokenConfig(&valid); err != nil {
		t.Fatalf("expected test JWT config to be valid, got: %v", err)
	}

	tests := map[string]func(*providers.MainConfig){
		"missing issuer":   func(config *providers.MainConfig) { config.Security.TokenIssuer = "" },
		"missing audience": func(config *providers.MainConfig) { config.Security.TokenAudience = "" },
		"zero expiration":  func(config *providers.MainConfig) { config.Security.TokenExpires = 0 },
	}
	for name, invalidate := range tests {
		t.Run(name, func(t *testing.T) {
			config := valid
			invalidate(&config)
			if err := services.ValidateTokenConfig(&config); err == nil {
				t.Fatal("expected invalid JWT config to be rejected")
			}
		})
	}
}

func TestValidTokenRequiresExpirationIssuerAndAudience(t *testing.T) {
	config := providers.GetMainConfig()
	base := jwt.MapClaims{
		"sub": "subject-1",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iss": config.Security.TokenIssuer,
		"aud": config.Security.TokenAudience,
	}

	tests := map[string]func(jwt.MapClaims){
		"missing expiration": func(claims jwt.MapClaims) { delete(claims, "exp") },
		"wrong issuer":       func(claims jwt.MapClaims) { claims["iss"] = "https://other.example" },
		"wrong audience":     func(claims jwt.MapClaims) { claims["aud"] = "other-api" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			claims := make(jwt.MapClaims, len(base))
			for key, value := range base {
				claims[key] = value
			}
			mutate(claims)
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
			signed, err := token.SignedString([]byte(config.Security.SecretKey))
			if err != nil {
				t.Fatalf("sign token: %v", err)
			}
			if _, err := services.ValidToken(signed); err == nil {
				t.Fatal("expected token with invalid registered claims to be rejected")
			}
		})
	}
}

// The subject claim is an opaque string: a non-UUID public id must go
// through the whole chain untouched.
func TestGetStringClaimAcceptsOpaqueSubject(t *testing.T) {
	subject := "cus_a8x3k42"
	token, err := services.CreateToken(subject)
	if err != nil {
		t.Fatalf("Error creating token: %v", err)
	}

	parsedToken, err := services.ValidToken(token)
	if err != nil {
		t.Fatalf("Error validating token: %v", err)
	}

	got, err := services.GetStringClaim(parsedToken, "sub")
	if err != nil {
		t.Errorf("expected an opaque subject to be accepted, got: %v", err)
	}
	if got != subject {
		t.Errorf("expected subject %v, got %v", subject, got)
	}
}

func TestGetStringClaimRejectsNonString(t *testing.T) {
	config := providers.GetMainConfig()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": 42,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iss": config.Security.TokenIssuer,
		"aud": config.Security.TokenAudience,
	})
	signed, err := token.SignedString([]byte(config.Security.SecretKey))
	if err != nil {
		t.Fatalf("Error signing token: %v", err)
	}

	parsedToken, err := services.ValidToken(signed)
	if err != nil {
		t.Fatalf("Error validating token: %v", err)
	}

	if _, err := services.GetStringClaim(parsedToken, "sub"); err == nil {
		t.Error("expected a non-string sub claim to be rejected")
	}
}

func TestCreateTokenRejectsEmptySubject(t *testing.T) {
	if _, err := services.CreateToken(""); err == nil {
		t.Error("expected an empty subject to be rejected")
	}
}
