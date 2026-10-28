package routing

// This file holds the only test exercising the real plugin seam of the
// framework end to end: fixture route files are compiled into actual Go
// plugins with "go build -buildmode=plugin", then the real loader
// (loadAPIHandlers) opens them with plugin.Open, extracts their symbols,
// adapts their handlers and registers them on a real *gin.Engine, and requests
// are served through net/http/httptest.
//
// Everything else in the test suite either tests pure functions or handler
// chains assembled by hand; only this test proves that a route written in a
// plugin actually answers, that the four accepted handler signatures survive
// the plugin boundary, and that the authorization decisions taken at load time
// (derived permissions, Permissions overrides, D1 without authorizer) hold for
// real plugins.
//
// Running it:
//
//	go test ./routing -run TestPluginRoutesEndToEnd -v   # this test only
//	go test ./...                                        # included by default
//	go test -short ./...                                 # skipped
//
// It compiles five plugins, so it costs a few seconds per plugin — much more
// on a cold build cache, since every dependency (Gin included) has to be
// recompiled for -buildmode=plugin the first time. A plugin must be built
// exactly like its host, so pluginBuildFlags adds -race when the test binary
// itself is race-enabled; a run with build flags that change the packages the
// plugin shares with the test binary (-coverpkg over the whole module, custom
// -gcflags or -tags) would be reported by plugin.Open as a version mismatch.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/authorization/memory"
	"github.com/goyourt/yogourt/compiler"
	"github.com/goyourt/yogourt/middleware"
)

const (
	// testSubjectHeader carries the identity the inherited middleware callback
	// attaches to the request, standing in for a real authentication.
	testSubjectHeader = "X-Yogourt-Test-Subject"

	readerSubjectID   = "11111111-1111-1111-1111-111111111111"
	editorSubjectID   = "22222222-2222-2222-2222-222222222222"
	strangerSubjectID = "33333333-3333-3333-3333-333333333333"

	// pluginBuildBudget bounds the total time spent compiling the fixtures, so
	// a machine too slow for this test skips it with a clear message instead of
	// being killed by the go test timeout.
	pluginBuildBudget = 6 * time.Minute
)

var (
	// errPluginUnsupported means the platform cannot build Go plugins at all.
	errPluginUnsupported = errors.New("go build -buildmode=plugin is not supported here")
	// errBuildBudget means the fixtures could not be compiled in time.
	errBuildBudget = errors.New("compiling the route plugins exceeded the time budget")
)

// pluginFixture is one fixture file, compiled into the plugin the loader must
// find for the API file it is copied to.
type pluginFixture struct {
	// fixture is the path under testdata/pluginapi of the source file.
	fixture string
	// apiFile is the path, relative to the temporary root, of the API file the
	// framework scans. Its plugin lands where compiler.PluginPath says.
	apiFile string
	// compile is false for an API file deliberately left without a plugin.
	compile bool
}

var pluginFixtures = []pluginFixture{
	// The first fixture is compiled alone and fills the build cache for the
	// others (see buildPlugins), so it is the one importing the most.
	{fixture: "widgets/id_/routes.go", apiFile: "api/widgets/id_/routes.go", compile: true},
	{fixture: "widgets/list.go", apiFile: "api/widgets/list.go", compile: true},
	{fixture: "widgets/create.go", apiFile: "api/widgets/create.go", compile: true},
	{fixture: "public/index.go", apiFile: "api/public/index.go", compile: true},
	{fixture: "orphan/index.go", apiFile: "api_broken/widgets/index.go", compile: true},
	{fixture: "widgets/list.go", apiFile: "api_missing/widgets/index.go", compile: false},
}

// pluginWorld is a temporary application tree: API folders holding the scanned
// route files, and the compiled plugins where the framework resolves them. The
// process working directory is the root of that tree for the whole test, since
// compiler.PluginPath resolves a plugin relative to os.Getwd().
type pluginWorld struct {
	apiDir     string
	brokenDir  string
	missingDir string
}

func TestPluginRoutesEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles real Go plugins with go build -buildmode=plugin: too slow for -short")
	}

	gin.SetMode(gin.TestMode)
	world := newPluginWorld(t)

	t.Run("WithoutAuthorizer", world.testWithoutAuthorizer)
	t.Run("WithAuthorizer", world.testWithAuthorizer)
	t.Run("CustomPrefix", world.testCustomPrefix)
	t.Run("RootPrefix", world.testRootPrefix)
	t.Run("InconsistentPermissions", world.testInconsistentPermissions)
	t.Run("MissingCompiledPlugin", world.testMissingCompiledPlugin)
}

// testWithoutAuthorizer covers the D1 non-regression: an application without
// authorizer boots and behaves identically whether its plugins export a
// Permissions symbol or not — the symbol is ignored, never fatal — and every
// route stays reachable by an anonymous caller.
func (w *pluginWorld) testWithoutAuthorizer(t *testing.T) {
	logs := captureStandardLog(t)

	r := gin.New()
	declared, err := loadAPIHandlers(r, "/api", w.apiDir, nil)
	if err != nil {
		t.Fatalf("loading the API without authorizer failed: %v", err)
	}
	if len(declared) != 0 {
		t.Errorf("no permission may be declared without an authorizer, got %v", declared)
	}

	assertRegisteredRoutes(t, r, []string{
		"GET /api/public",
		"GET /api/widgets",
		"POST /api/widgets",
		"GET /api/widgets/:id",
		"PATCH /api/widgets/:id",
	})

	// Every method of every route answers an anonymous caller, whatever the
	// handler signature: the plugin exporting a Permissions symbol
	// (/api/public) behaves exactly like those that do not.
	body := requireJSON(t, w.request(t, r, http.MethodGet, "/api/widgets", ""), http.StatusOK)
	requireFields(t, body, map[string]any{"handler": "widgets.list", "signature": "gin"})

	body = requireJSON(t, w.request(t, r, http.MethodPost, "/api/widgets", ""), http.StatusCreated)
	requireFields(t, body, map[string]any{"handler": "widgets.create", "signature": "core"})

	body = requireJSON(t, w.request(t, r, http.MethodGet, "/api/widgets/42", ""), http.StatusOK)
	requireFields(t, body, map[string]any{"signature": "core+params", "id": float64(42), "idType": "int"})

	body = requireJSON(t, w.request(t, r, http.MethodPatch, "/api/widgets/7", ""), http.StatusOK)
	requireFields(t, body, map[string]any{"signature": "core+*params", "id": float64(7), "idType": "int64"})

	body = requireJSON(t, w.request(t, r, http.MethodGet, "/api/public", ""), http.StatusOK)
	requireFields(t, body, map[string]any{"handler": "public.index"})

	logged := logs.String()
	if !strings.Contains(logged, "public/index.go exports a Permissions symbol") {
		t.Errorf("the ignored Permissions symbol must be reported, logs:\n%s", logged)
	}
	if !strings.Contains(logged, "the symbol is ignored") {
		t.Errorf("the warning must say the symbol is ignored, logs:\n%s", logged)
	}
	if count := strings.Count(logged, "exports a Permissions symbol"); count != 1 {
		t.Errorf("only the file exporting the symbol may be reported, got %d warnings:\n%s", count, logged)
	}
	if strings.Contains(logged, "authorization:") {
		t.Errorf("no authorization surface may be logged without an authorizer, logs:\n%s", logged)
	}
}

// testWithAuthorizer covers the authorization behaviour of real plugins: the
// permission derived from the folder and the HTTP method protects the route,
// a Permissions entry declaring authorization.Public lets an anonymous caller
// through, the inherited middleware callback attaching the subject runs before
// the RBAC check, and the typed parameters of a dynamic route reach the
// handler.
func (w *pluginWorld) testWithAuthorizer(t *testing.T) {
	engine := authorization.NewEngine(authorization.WithProvider(grantingProvider(t)))

	// The inherited callback stands for the authentication middleware of an
	// application: it attaches the subject to the request context, and the
	// loader must place it before the RBAC middleware (D3) — otherwise every
	// authenticated request would be answered 401.
	callbacks := &callbackProbe{}
	middleware.SetMiddlewares(map[string]func(*gin.Context){"/": callbacks.attachSubject})
	t.Cleanup(func() { middleware.SetMiddlewares(nil) })

	logs := captureStandardLog(t)

	r := gin.New()
	declared, err := loadAPIHandlers(r, "/api", w.apiDir, engine)
	if err != nil {
		t.Fatalf("loading the API with an authorizer failed: %v", err)
	}

	// The permissions returned for the boot synchronization are the derived
	// ones; the public method contributes none.
	assertDeclaredPermissions(t, declared, []authorization.Action{
		"widgets.create",
		"widgets.read",
		"widgets.update",
	})

	logged := logs.String()
	for _, want := range []string{
		"authorization: GET /api/public -> @public (declared)",
		"authorization: GET /api/widgets -> widgets.read (derived)",
		"authorization: POST /api/widgets -> widgets.create (derived)",
		"authorization: PATCH /api/widgets/:id -> widgets.update (derived)",
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("missing authorization surface line %q, logs:\n%s", want, logged)
		}
	}

	t.Run("AnonymousIsUnauthorized", func(t *testing.T) {
		before := callbacks.calls()
		requireJSON(t, w.request(t, r, http.MethodGet, "/api/widgets", ""), http.StatusUnauthorized)
		// The inherited callback ran even though the RBAC middleware aborted:
		// authentication comes first, authorization second.
		if callbacks.calls() != before+1 {
			t.Error("the inherited middleware callback must run before the RBAC middleware")
		}
	})

	t.Run("SubjectWithoutTheRoleIsForbidden", func(t *testing.T) {
		requireJSON(t, w.request(t, r, http.MethodGet, "/api/widgets", strangerSubjectID), http.StatusForbidden)
	})

	t.Run("GrantedSubjectReachesTheHandler", func(t *testing.T) {
		body := requireJSON(t, w.request(t, r, http.MethodGet, "/api/widgets", readerSubjectID), http.StatusOK)
		requireFields(t, body, map[string]any{"handler": "widgets.list"})
	})

	t.Run("PublicMethodStaysAnonymous", func(t *testing.T) {
		body := requireJSON(t, w.request(t, r, http.MethodGet, "/api/public", ""), http.StatusOK)
		requireFields(t, body, map[string]any{"handler": "public.index"})
	})

	t.Run("PermissionsAreDerivedPerMethod", func(t *testing.T) {
		// The reader holds widgets.read only: the derived permissions of POST
		// and PATCH are enforced separately, from the same folder.
		requireJSON(t, w.request(t, r, http.MethodPost, "/api/widgets", readerSubjectID), http.StatusForbidden)
		requireJSON(t, w.request(t, r, http.MethodPatch, "/api/widgets/7", readerSubjectID), http.StatusForbidden)

		body := requireJSON(t, w.request(t, r, http.MethodPost, "/api/widgets", editorSubjectID), http.StatusCreated)
		requireFields(t, body, map[string]any{"handler": "widgets.create", "signature": "core"})
	})

	t.Run("TypedParamsByValue", func(t *testing.T) {
		body := requireJSON(t, w.request(t, r, http.MethodGet, "/api/widgets/42", readerSubjectID), http.StatusOK)
		requireFields(t, body, map[string]any{
			"handler":   "widgets.detail",
			"signature": "core+params",
			"id":        float64(42),
			"idType":    "int",
		})
	})

	t.Run("TypedParamsByPointer", func(t *testing.T) {
		body := requireJSON(t, w.request(t, r, http.MethodPatch, "/api/widgets/7", editorSubjectID), http.StatusOK)
		requireFields(t, body, map[string]any{
			"handler":   "widgets.patch",
			"signature": "core+*params",
			"id":        float64(7),
			"idType":    "int64",
			// The field tagged `param:"-"` is never hydrated.
			"label": "",
		})
	})

	t.Run("UnconvertibleParamIsRejected", func(t *testing.T) {
		body := requireJSON(t, w.request(t, r, http.MethodGet, "/api/widgets/abc", readerSubjectID), http.StatusBadRequest)
		requireFields(t, body, map[string]any{"error": "Invalid request parameters"})
	})
}

// testCustomPrefix covers the configurable HTTP prefix: the same route tree
// served under "/v1" instead of "/api". The prefix moves every URL and nothing
// else — in particular the permissions derived by convention are identical,
// because a prefix names a mount point, not a resource.
func (w *pluginWorld) testCustomPrefix(t *testing.T) {
	engine := authorization.NewEngine(authorization.WithProvider(grantingProvider(t)))

	r := gin.New()
	declared, err := loadAPIHandlers(r, "/v1", w.apiDir, engine)
	if err != nil {
		t.Fatalf("loading the API under a custom prefix failed: %v", err)
	}

	assertRegisteredRoutes(t, r, []string{
		"GET /v1/public",
		"GET /v1/widgets",
		"POST /v1/widgets",
		"GET /v1/widgets/:id",
		"PATCH /v1/widgets/:id",
	})

	// Same permissions as under "/api": "v1" never becomes a resource.
	assertDeclaredPermissions(t, declared, []authorization.Action{
		"widgets.create",
		"widgets.read",
		"widgets.update",
	})

	body := requireJSON(t, w.request(t, r, http.MethodGet, "/v1/public", ""), http.StatusOK)
	requireFields(t, body, map[string]any{"handler": "public.index"})

	// The old prefix is gone, not aliased.
	if recorder := w.request(t, r, http.MethodGet, "/api/public", ""); recorder.Code != http.StatusNotFound {
		t.Errorf("GET /api/public = %d, want 404 once the prefix is /v1", recorder.Code)
	}
}

// testRootPrefix covers the extreme case of the same setting: an empty prefix
// serves the route tree at the root, and the root route file answers on "/".
func (w *pluginWorld) testRootPrefix(t *testing.T) {
	r := gin.New()
	if _, err := loadAPIHandlers(r, "", w.apiDir, nil); err != nil {
		t.Fatalf("loading the API at the root failed: %v", err)
	}

	assertRegisteredRoutes(t, r, []string{
		"GET /public",
		"GET /widgets",
		"POST /widgets",
		"GET /widgets/:id",
		"PATCH /widgets/:id",
	})

	body := requireJSON(t, w.request(t, r, http.MethodGet, "/widgets", ""), http.StatusOK)
	requireFields(t, body, map[string]any{"handler": "widgets.list", "signature": "gin"})
}

// testInconsistentPermissions covers the boot report: a Permissions entry
// naming no exported handler must fail the load, with the violation named.
func (w *pluginWorld) testInconsistentPermissions(t *testing.T) {
	engine := authorization.NewEngine(authorization.WithProvider(memory.NewProvider()))

	_, err := loadAPIHandlers(gin.New(), "/api", w.brokenDir, engine)
	if err == nil {
		t.Fatal("an orphan Permissions entry must fail the load")
	}
	for _, want := range []string{
		"route permission validation failed (1 violation(s))",
		"widgets/index.go: DELETE: permission declared but no exported handler with this name",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected the report to contain %q, got:\n%v", want, err)
		}
	}
}

// testMissingCompiledPlugin covers a scanned route file whose plugin was never
// compiled: the load must fail naming the plugin it looked for.
func (w *pluginWorld) testMissingCompiledPlugin(t *testing.T) {
	_, err := loadAPIHandlers(gin.New(), "/api", w.missingDir, nil)
	if err == nil {
		t.Fatal("a route file without compiled plugin must fail the load")
	}
	wantPlugin := filepath.Join(".yogourt", "api_missing", "widgets", "index.go.so")
	for _, want := range []string{
		"resolve plugin error",
		fmt.Sprintf("compiled plugin %s does not exist", wantPlugin),
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected the error to contain %q, got:\n%v", want, err)
		}
	}
}

// request serves one request through the engine, attaching the subject header
// the inherited middleware callback turns into an authorization subject.
func (w *pluginWorld) request(t *testing.T, r *gin.Engine, method, path, subjectID string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	if subjectID != "" {
		req.Header.Set(testSubjectHeader, subjectID)
	}
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, req)

	return recorder
}

// newPluginWorld lays out the temporary application, compiles the fixtures
// into real plugins and makes its root the working directory of the test.
func newPluginWorld(t *testing.T) *pluginWorld {
	t.Helper()

	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolving the package directory failed: %v", err)
	}
	// The plugins are built from the module itself: same toolchain, same
	// dependency versions, same package import paths as the test binary — the
	// conditions plugin.Open enforces.
	moduleDir := filepath.Dir(packageDir)
	fixturesDir := filepath.Join(packageDir, "testdata", "pluginapi")

	// compiler.PluginPath computes the plugin location with filepath.Rel
	// against os.Getwd(), which is symlink-free; t.TempDir() is not on macOS,
	// so the root has to be resolved before anything derives a path from it.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolving the temporary root failed: %v", err)
	}

	for _, fixture := range pluginFixtures {
		copyFile(t, filepath.Join(fixturesDir, fixture.fixture), filepath.Join(root, fixture.apiFile))
	}

	t.Chdir(root)
	buildPlugins(t, moduleDir, fixturesDir, root)

	return &pluginWorld{
		apiDir:     filepath.Join(root, "api"),
		brokenDir:  filepath.Join(root, "api_broken"),
		missingDir: filepath.Join(root, "api_missing"),
	}
}

// buildPlugins compiles every fixture needing a plugin. The first one is built
// alone: it fills the build cache with all the shared dependencies compiled for
// -buildmode=plugin, which leaves the others short enough to build
// concurrently.
func buildPlugins(t *testing.T, moduleDir, fixturesDir, root string) {
	t.Helper()

	goTool := goToolPath(t)
	ctx, cancel := context.WithTimeout(context.Background(), pluginBuildBudget)
	defer cancel()

	jobs := make([]pluginFixture, 0, len(pluginFixtures))
	for _, fixture := range pluginFixtures {
		if fixture.compile {
			jobs = append(jobs, fixture)
		}
	}

	started := time.Now()
	if err := buildPlugin(ctx, goTool, moduleDir, fixturesDir, root, jobs[0]); err != nil {
		skipOrFail(t, err)
	}

	var (
		wg   sync.WaitGroup
		errs = make([]error, len(jobs))
	)
	for i := 1; i < len(jobs); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = buildPlugin(ctx, goTool, moduleDir, fixturesDir, root, jobs[i])
		}()
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			skipOrFail(t, err)
		}
	}
	t.Logf("compiled %d route plugins in %s", len(jobs), time.Since(started).Round(time.Millisecond))
}

// buildPlugin compiles one fixture into the plugin the framework will resolve
// for its API file.
func buildPlugin(ctx context.Context, goTool, moduleDir, fixturesDir, root string, fixture pluginFixture) error {
	pluginPath, err := compiler.PluginPath(filepath.Join(root, fixture.apiFile))
	if err != nil {
		return fmt.Errorf("resolving the plugin path of %s: %w", fixture.apiFile, err)
	}
	output := filepath.Join(root, pluginPath)
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return fmt.Errorf("creating the plugin folder of %s: %w", fixture.apiFile, err)
	}

	args := append([]string{"build", "-buildmode=plugin"}, pluginBuildFlags...)
	args = append(args, "-o", output, filepath.Join(fixturesDir, fixture.fixture))
	command := exec.CommandContext(ctx, goTool, args...)
	command.Dir = moduleDir
	command.Env = os.Environ()

	// -buildmode=plugin writes linker warnings to stderr on some platforms:
	// only the exit status decides.
	combined, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%w (%s): %v", errBuildBudget, pluginBuildBudget, ctxErr)
	}
	// Only the go tool mentions the build mode; a broken fixture reports a
	// compilation error instead, which must fail the test.
	if strings.Contains(string(combined), "buildmode=plugin") {
		return fmt.Errorf("%w: %v\n%s", errPluginUnsupported, err, combined)
	}

	return fmt.Errorf("building the plugin of %s failed: %v\n%s", fixture.fixture, err, combined)
}

func skipOrFail(t *testing.T, err error) {
	t.Helper()

	switch {
	case errors.Is(err, errPluginUnsupported):
		t.Skipf("this platform cannot compile Go plugins: %v", err)
	case errors.Is(err, errBuildBudget):
		t.Skipf("%v: warm the build cache with \"go build -buildmode=plugin\" and retry", err)
	default:
		t.Fatal(err)
	}
}

func goToolPath(t *testing.T) string {
	t.Helper()

	if path, err := exec.LookPath("go"); err == nil {
		return path
	}
	fallback := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(fallback); err != nil {
		t.Skip("the go tool is required to compile the route plugin fixtures")
	}

	return fallback
}

func copyFile(t *testing.T, source, destination string) {
	t.Helper()

	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("reading the fixture %s failed: %v", source, err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatalf("creating %s failed: %v", filepath.Dir(destination), err)
	}
	if err := os.WriteFile(destination, content, 0o644); err != nil {
		t.Fatalf("writing %s failed: %v", destination, err)
	}
}

// grantingProvider builds the in-memory provider backing the test: the reader
// may only read widgets, the editor may also create and update them.
func grantingProvider(t *testing.T) *memory.Provider {
	t.Helper()

	ctx := context.Background()
	provider := memory.NewProvider()

	grant := func(role string, subjectID string, permissions ...authorization.Action) {
		if err := provider.CreateRole(ctx, role); err != nil {
			t.Fatalf("creating the role %s failed: %v", role, err)
		}
		if err := provider.GrantPermissions(ctx, role, permissions...); err != nil {
			t.Fatalf("granting %v to %s failed: %v", permissions, role, err)
		}
		if err := provider.BindRoles(ctx, subjectID, authorization.ScopeGlobal, role); err != nil {
			t.Fatalf("binding %s to %s failed: %v", role, subjectID, err)
		}
	}

	grant("widget-reader", readerSubjectID, "widgets.read")
	grant("widget-editor", editorSubjectID, "widgets.read", "widgets.create", "widgets.update")

	return provider
}

// callbackProbe is the inherited middleware callback of the test: it attaches
// the subject named by the request header, exactly like a real authentication
// middleware, and counts its own executions so the test can prove it ran
// before the RBAC middleware.
type callbackProbe struct {
	mu    sync.Mutex
	count int
}

func (p *callbackProbe) attachSubject(c *gin.Context) {
	p.mu.Lock()
	p.count++
	p.mu.Unlock()

	subjectID := c.GetHeader(testSubjectHeader)
	if subjectID == "" || c.Request == nil {
		return
	}
	c.Request = c.Request.WithContext(
		authorization.WithSubject(c.Request.Context(), authorization.Subject{ID: subjectID}),
	)
}

func (p *callbackProbe) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.count
}

func assertRegisteredRoutes(t *testing.T, r *gin.Engine, want []string) {
	t.Helper()

	got := make([]string, 0, len(r.Routes()))
	for _, route := range r.Routes() {
		got = append(got, route.Method+" "+route.Path)
	}
	sort.Strings(got)
	expected := append([]string(nil), want...)
	sort.Strings(expected)

	if strings.Join(got, ", ") != strings.Join(expected, ", ") {
		t.Errorf("registered routes mismatch:\n got: %v\nwant: %v", got, expected)
	}
}

func assertDeclaredPermissions(t *testing.T, got []authorization.Action, want []authorization.Action) {
	t.Helper()

	formatted := make([]string, 0, len(got))
	for _, permission := range got {
		formatted = append(formatted, string(permission))
	}
	sort.Strings(formatted)
	expected := make([]string, 0, len(want))
	for _, permission := range want {
		expected = append(expected, string(permission))
	}
	sort.Strings(expected)

	if strings.Join(formatted, ", ") != strings.Join(expected, ", ") {
		t.Errorf("declared permissions mismatch:\n got: %v\nwant: %v", formatted, expected)
	}
}

func requireJSON(t *testing.T, recorder *httptest.ResponseRecorder, status int) map[string]any {
	t.Helper()

	if recorder.Code != status {
		t.Fatalf("expected status %d, got %d with body %s", status, recorder.Code, recorder.Body.String())
	}
	if recorder.Body.Len() == 0 {
		return nil
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the response body %q failed: %v", recorder.Body.String(), err)
	}

	return body
}

func requireFields(t *testing.T, body map[string]any, want map[string]any) {
	t.Helper()

	for field, expected := range want {
		got, ok := body[field]
		if !ok {
			t.Errorf("field %q missing from the response %v", field, body)

			continue
		}
		if got != expected {
			t.Errorf("field %q: got %#v, want %#v", field, got, expected)
		}
	}
}

// captureStandardLog redirects the standard logger, which is where the loader
// writes its warnings and its authorization surface, into a buffer readable by
// the test.
func captureStandardLog(t *testing.T) *syncBuffer {
	t.Helper()

	buffer := &syncBuffer{}
	flags, writer := log.Flags(), log.Writer()
	log.SetOutput(buffer)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(writer)
		log.SetFlags(flags)
	})

	return buffer
}

// syncBuffer is a log sink safe to write from the loader and read from the
// test.
type syncBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

var _ io.Writer = (*syncBuffer)(nil)

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buffer.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buffer.String()
}
