package routing

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/services/providers"
)

func TestHTTPServerUsesBoundedDefaults(t *testing.T) {
	server, err := newHTTPServer(http.NotFoundHandler(), providers.ServerConfig{})
	if err != nil {
		t.Fatal(err)
	}

	if server.ReadHeaderTimeout != defaultReadHeaderTimeout || server.ReadTimeout != defaultReadTimeout ||
		server.WriteTimeout != defaultWriteTimeout || server.IdleTimeout != defaultIdleTimeout {
		t.Fatalf("unexpected default timeouts: %#v", server)
	}
	if server.MaxHeaderBytes != defaultMaxHeaderBytes {
		t.Fatalf("MaxHeaderBytes = %d, want %d", server.MaxHeaderBytes, defaultMaxHeaderBytes)
	}
}

func TestHTTPServerAppliesConfiguredBudgets(t *testing.T) {
	cfg := providers.ServerConfig{
		ReadHeaderTimeout: providers.Duration(2 * time.Second),
		ReadTimeout:       providers.Duration(3 * time.Second),
		WriteTimeout:      providers.Duration(4 * time.Second),
		IdleTimeout:       providers.Duration(5 * time.Second),
		MaxHeaderBytes:    2048,
		MaxBodyBytes:      4096,
	}
	server, err := newHTTPServer(nil, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if server.ReadHeaderTimeout != 2*time.Second || server.ReadTimeout != 3*time.Second ||
		server.WriteTimeout != 4*time.Second || server.IdleTimeout != 5*time.Second || server.MaxHeaderBytes != 2048 {
		t.Fatalf("configured budgets were not applied: %#v", server)
	}
}

func TestHTTPServerRejectsUnboundedBudgets(t *testing.T) {
	cases := []providers.ServerConfig{
		{ReadTimeout: providers.Duration(maxHTTPTimeout + time.Second)},
		{MaxHeaderBytes: maxHTTPHeaderBytes + 1},
		{MaxBodyBytes: providers.MaxRequestBodyBytes + 1},
	}
	for _, cfg := range cases {
		if _, err := newHTTPServer(nil, cfg); err == nil {
			t.Fatalf("newHTTPServer(%#v) accepted an unbounded budget", cfg)
		}
	}
}

func TestUntrustedForwardedAddressDoesNotReplacePeer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	if err := configureTrustedProxies(engine, nil); err != nil {
		t.Fatal(err)
	}
	engine.GET("/", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "203.0.113.10:4321"
	request.Header.Set("X-Forwarded-For", "198.51.100.20")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	if response.Body.String() != "203.0.113.10" {
		t.Fatalf("ClientIP() = %q, want the direct peer", response.Body.String())
	}
}

func TestRequestBodyLimitRejectsDeclaredAndStreamedBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(limitRequestBody(providers.ServerConfig{MaxBodyBytes: 8}))
	called := false
	engine.POST("/", func(c *gin.Context) {
		called = true
		_, err := io.ReadAll(c.Request.Body)
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.Status(http.StatusRequestEntityTooLarge)
		}
	})

	declared := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("123456789")))
	declaredResponse := httptest.NewRecorder()
	engine.ServeHTTP(declaredResponse, declared)
	if declaredResponse.Code != http.StatusRequestEntityTooLarge || called {
		t.Fatalf("declared oversized body = %d, handler called %v", declaredResponse.Code, called)
	}

	streamed := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("123456789")))
	streamed.ContentLength = -1
	streamedResponse := httptest.NewRecorder()
	engine.ServeHTTP(streamedResponse, streamed)
	if streamedResponse.Code != http.StatusRequestEntityTooLarge || !called {
		t.Fatalf("streamed oversized body = %d, handler called %v", streamedResponse.Code, called)
	}
}

func TestRequestBodyLimitRemovesMultipartTemporaryFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("upload", "large.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(bytes.Repeat([]byte("x"), 4096)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	var temporaryFile string
	engine := gin.New()
	engine.Use(limitRequestBody(providers.ServerConfig{MaxBodyBytes: int64(body.Len() + 1)}))
	engine.POST("/", func(c *gin.Context) {
		if err := c.Request.ParseMultipartForm(1); err != nil {
			c.Error(err)
			return
		}
		file, _, err := c.Request.FormFile("upload")
		if err != nil {
			c.Error(err)
			return
		}
		defer file.Close()
		if diskFile, ok := file.(*os.File); ok {
			temporaryFile = diskFile.Name()
		}
	})

	request := httptest.NewRequest(http.MethodPost, "/", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if temporaryFile == "" {
		t.Fatal("multipart parser did not create the expected temporary file")
	}
	if _, err := os.Stat(temporaryFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("multipart temporary file still exists after the request: %v", err)
	}
}
