package services

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/interfaces"
	"gorm.io/gorm"
)

// testUser is the minimal model Authenticate needs; the token is refused
// before it is ever loaded.
type testUser struct {
	interfaces.Base
}

// authenticateWithHeader runs Authenticate on a request carrying the given
// Authorization header (none when empty) and returns the recorder.
func authenticateWithHeader(t *testing.T, header string) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	// Keep the server-side detail out of the test output.
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if header != "" {
		c.Request.Header.Set("Authorization", header)
	}

	Authenticate(c, &testUser{})

	return w
}

// A refused token must never describe the internal error of the framework:
// every rejection of the Authorization header answers the same generic body
// as the rest of the authorization chain.
func TestAuthenticateRejectsHeaderWithGenericBody(t *testing.T) {
	cases := map[string]string{
		"missing header":   "",
		"malformed header": "Basic dXNlcjpwYXNz",
		"no bearer prefix": "Bearer",
	}

	const wantBody = `{"error":"Unauthorized"}`

	for name, header := range cases {
		w := authenticateWithHeader(t, header)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401, got %d", name, w.Code)
		}
		if got := w.Body.String(); got != wantBody {
			t.Errorf("%s: expected body %s, got %s", name, wantBody, got)
		}
	}
}

// AUTHZ-014: an unknown user is an authentication refusal, a database outage
// is a technical failure — the two must never swap statuses.
func TestRespondUserLookupFailureUnknownUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	respondUserLookupFailure(c, gorm.ErrRecordNotFound)

	// A validly signed token whose subject has no row must not be told apart
	// from any other refusal.
	if body := w.Body.String(); body != `{"error":"Unauthorized"}` {
		t.Errorf("expected the generic refusal body, got %s", body)
	}

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
