package services

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/services/providers"
)

func multipartUploadRequest(t *testing.T, files map[string]string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for field, content := range files {
		part, err := writer.CreateFormFile(field, field+".txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "/", body)
	context.Request.Header.Set("Content-Type", writer.FormDataContentType())
	return context, response
}

func TestReadUploadedFileAppliesCapBeforeMultipartParsing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := multipartUploadRequest(t, map[string]string{"upload": strings.Repeat("x", 128)})
	context.Request.ContentLength = -1
	folder := t.TempDir() + "/"
	maxFileSize := 1024

	_, err := readUploadedFile(context, "upload", "documents", providers.FileOptions{
		FileFolder: &folder, MaxFileSize: &maxFileSize,
	}, 64)
	var tooLarge *http.MaxBytesError
	if !errors.As(err, &tooLarge) {
		t.Fatalf("readUploadedFile() error = %v, want MaxBytesError", err)
	}
	if context.Request.MultipartForm != nil {
		defer context.Request.MultipartForm.RemoveAll()
	}
}

func TestReadUploadedFileSupportsSeveralFilesWithinOneLimitedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := multipartUploadRequest(t, map[string]string{"first": "one", "second": "two"})
	folder := t.TempDir() + "/"
	maxFileSize := 16
	cfg := providers.FileOptions{FileFolder: &folder, MaxFileSize: &maxFileSize}
	defer func() {
		if context.Request.MultipartForm != nil {
			_ = context.Request.MultipartForm.RemoveAll()
		}
	}()

	first, err := readUploadedFile(context, "first", "documents", cfg, 4096)
	if err != nil {
		t.Fatal(err)
	}
	second, err := readUploadedFile(context, "second", "documents", cfg, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if first.GetContent() != "one" || second.GetContent() != "two" {
		t.Fatalf("uploaded contents = %q and %q", first.GetContent(), second.GetContent())
	}
}

func TestReadUploadedFileRejectsUnsafePreparsedOrIncompleteConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := multipartUploadRequest(t, map[string]string{"upload": "content"})
	if err := context.Request.ParseMultipartForm(1024); err != nil {
		t.Fatal(err)
	}
	defer context.Request.MultipartForm.RemoveAll()
	folder := t.TempDir() + "/"
	maxFileSize := 16
	if _, err := readUploadedFile(context, "upload", "documents", providers.FileOptions{
		FileFolder: &folder, MaxFileSize: &maxFileSize,
	}, 4096); err == nil || !strings.Contains(err.Error(), "parsed before") {
		t.Fatalf("unsafe pre-parsed multipart error = %v", err)
	}

	fresh, _ := multipartUploadRequest(t, map[string]string{"upload": "content"})
	if _, err := readUploadedFile(fresh, "upload", "documents", providers.FileOptions{}, 4096); err == nil {
		t.Fatal("incomplete file configuration was accepted")
	}
	if fresh.Request.MultipartForm != nil {
		t.Fatal("invalid configuration was detected only after multipart parsing")
	}
}
