package integration_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/interfaces"
	"github.com/goyourt/yogourt/routing"
	"github.com/goyourt/yogourt/services"
	"github.com/goyourt/yogourt/services/providers"
)

const integrationSecret = "this-secret-key-is-at-least-32-bytes-long"
const integrationIssuer = "https://auth.integration.example"
const integrationAudience = "yogourt-integration-api"

// authSubjectUser deliberately does not use the conventional uuid column.
// Authentication has to load it using PublicIdColumn, not a hard-coded name.
type authSubjectUser struct {
	interfaces.WithID
	Subject *string `gorm:"column:subject_key;type:uuid;default:gen_random_uuid();not null;unique" json:"subjectKey"`
	Name    string
}

func (authSubjectUser) TableName() string { return "v2_auth_subject_users" }

func (u *authSubjectUser) GetPublicId() string {
	if u.Subject == nil {
		return ""
	}
	return *u.Subject
}

func (u *authSubjectUser) PublicIdColumn() string { return "subject_key" }

type hydratedRelation struct {
	interfaces.WithUuid
	Name string
}

func (hydratedRelation) TableName() string { return "v2_hydrated_relations" }

type hydrationRequest struct {
	Relation *hydratedRelation `json:"relation"`
}

// configureIntegrationDatabase gives this otherwise independent test package
// a normal application configuration pointed at the opted-in test database.
func configureIntegrationDatabase(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("YOGOURT_TEST_DSN")
	if dsn == "" {
		t.Skip("YOGOURT_TEST_DSN not set; skipping PostgreSQL integration tests")
	}

	values := make(map[string]string)
	for _, field := range strings.Fields(dsn) {
		key, value, ok := strings.Cut(field, "=")
		if ok {
			values[key] = value
		}
	}
	for _, key := range []string{"host", "port", "user", "dbname"} {
		if values[key] == "" {
			t.Fatalf("YOGOURT_TEST_DSN must provide %s as a keyword-value DSN", key)
		}
	}
	port, err := strconv.Atoi(values["port"])
	if err != nil {
		t.Fatalf("parse test database port: %v", err)
	}

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "configs"), 0o755); err != nil {
		t.Fatalf("make configs directory: %v", err)
	}
	config := fmt.Sprintf(`database:
  type: postgres
  host: %q
  port: %d
  user: %q
  password: %q
  db: %q
  ssl_mode: %q
security:
  secret_key: %q
  token_issuer: %q
  token_audience: %q
  token_expires: 60
`, values["host"], port, values["user"], values["password"], values["dbname"], values["sslmode"], integrationSecret, integrationIssuer, integrationAudience)
	if err := os.WriteFile(filepath.Join(dir, "configs", "yogourt.yaml"), []byte(config), 0o600); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	t.Chdir(dir)
}

func requestContext(method, body string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestV2IdentityAndRequestInvariants(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureIntegrationDatabase(t)

	db, err := providers.GetDB()
	if err != nil {
		t.Fatalf("open configured PostgreSQL database: %v", err)
	}
	if err := db.AutoMigrate(&authSubjectUser{}, &hydratedRelation{}); err != nil {
		t.Fatalf("migrate fixtures: %v", err)
	}
	for _, table := range []string{"v2_auth_subject_users", "v2_hydrated_relations"} {
		if err := db.Exec(`TRUNCATE ` + table + ` RESTART IDENTITY`).Error; err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}

	user := &authSubjectUser{Name: "loaded user"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed authenticated user: %v", err)
	}
	if user.Subject == nil {
		t.Fatal("seed user did not receive its generated public id")
	}

	// A legacy v1-shaped HS256 token can be correctly signed and have a UUID,
	// yet still lack the canonical subject. It must not reach the database.
	legacy := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uuid": *user.Subject,
		"exp":  time.Now().Add(time.Hour).Unix(),
		"iss":  integrationIssuer,
		"aud":  integrationAudience,
	})
	legacyText, err := legacy.SignedString([]byte(integrationSecret))
	if err != nil {
		t.Fatalf("sign legacy token: %v", err)
	}
	legacyContext, legacyResponse := requestContext(http.MethodGet, "")
	legacyContext.Request.Header.Set("Authorization", "Bearer "+legacyText)
	services.Authenticate(legacyContext, &authSubjectUser{})
	if legacyResponse.Code != http.StatusUnauthorized || legacyResponse.Body.String() != `{"error":"Unauthorized"}` {
		t.Errorf("legacy token response = %d %s, want generic 401", legacyResponse.Code, legacyResponse.Body.String())
	}

	token, err := services.CreateToken(*user.Subject)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	authContext, authResponse := requestContext(http.MethodGet, "")
	authContext.Request.Header.Set("Authorization", "Bearer "+token)
	currentUser := &authSubjectUser{}
	services.Authenticate(authContext, currentUser)
	if authResponse.Code != http.StatusOK || authContext.IsAborted() {
		t.Fatalf("authentication response = %d %s", authResponse.Code, authResponse.Body.String())
	}
	stored, ok := providers.GetCurrentUser(authContext).(*authSubjectUser)
	if !ok || stored != currentUser || stored.Subject == nil || *stored.Subject != *user.Subject {
		t.Fatalf("current user = %#v, want loaded user with subject %q", stored, *user.Subject)
	}
	subject, ok := authorization.SubjectFromContext(authContext.Request.Context())
	if !ok || subject.ID != *user.Subject {
		t.Errorf("authorization subject = %#v, want id %q", subject, *user.Subject)
	}

	relation := &hydratedRelation{Name: "stored relation"}
	if err := db.Create(relation).Error; err != nil {
		t.Fatalf("seed relation: %v", err)
	}
	if relation.Uuid == nil {
		t.Fatal("seed relation did not receive its UUID")
	}

	hydrateContext, hydrateResponse := requestContext(http.MethodPost, fmt.Sprintf(`{"relation":{"uuid":%q,"Name":"untrusted"}}`, *relation.Uuid))
	hydrated := hydrationRequest{}
	if !routing.HandleRequest(hydrateContext, &hydrated) {
		t.Fatalf("hydrate known relation: status %d body %s", hydrateResponse.Code, hydrateResponse.Body.String())
	}
	if hydrated.Relation == nil || hydrated.Relation.Name != "stored relation" {
		t.Fatalf("known relation was not replaced with database state: %#v", hydrated.Relation)
	}

	unknown := "11111111-1111-1111-1111-111111111111"
	unknownContext, unknownResponse := requestContext(http.MethodPost, fmt.Sprintf(`{"relation":{"uuid":%q,"Name":"caller value"}}`, unknown))
	unknownRequest := hydrationRequest{}
	if !routing.HandleRequest(unknownContext, &unknownRequest) {
		t.Fatalf("unknown relation must pass without an existence oracle: status %d body %s", unknownResponse.Code, unknownResponse.Body.String())
	}
	if unknownRequest.Relation == nil || unknownRequest.Relation.Uuid == nil || *unknownRequest.Relation.Uuid != unknown || unknownRequest.Relation.Name != "caller value" {
		t.Errorf("unknown relation was changed: %#v", unknownRequest.Relation)
	}

	// A closed pool makes the next hydration lookup a real technical failure;
	// unlike an unknown id it must stop with 503.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get SQL database: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close SQL database: %v", err)
	}
	failureContext, failureResponse := requestContext(http.MethodPost, fmt.Sprintf(`{"relation":{"uuid":%q}}`, *relation.Uuid))
	failureRequest := hydrationRequest{}
	if routing.HandleRequest(failureContext, &failureRequest) {
		t.Fatal("technical database failure must abort hydration")
	}
	if failureResponse.Code != http.StatusServiceUnavailable {
		t.Errorf("technical hydration failure status = %d, want 503", failureResponse.Code)
	}
}

var _ interfaces.Resource = (*authSubjectUser)(nil)
