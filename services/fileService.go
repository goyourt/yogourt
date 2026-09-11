package services

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/interfaces"
	"github.com/goyourt/yogourt/services/providers"
)

const requestBodyLimitKey = "yogourt.request_body_limit"

func SaveFile(f interfaces.FileInterface) {
	cfg := providers.GetConfigByFileType(f.GetType())
	path := f.GetFilePath(*cfg.FileFolder)
	f.SetPath(path)
	GenerateFile(path, f.GetContent())
}

func ReadFile(f interfaces.FileInterface) (string, error) {
	if f.GetContent() != "" {
		return f.GetContent(), nil
	}

	content, err := os.ReadFile(f.GetPath())
	if err != nil {
		return "", err
	}

	f.SetContent(string(content))
	return f.GetContent(), nil
}

func GenerateFile(filePath string, fileContent string) {
	file, fileError := os.Create(filePath)
	if fileError != nil {
		fmt.Printf("error while creating file: %v \n", fileError)
		log.Printf("ERROR: %s\n", fileError)
		return
	}
	defer file.Close()

	file.WriteString(fileContent)
}

func CreateFolder(folderPath string) {
	folderError := os.MkdirAll(folderPath, os.ModePerm)
	if folderError != nil {
		fmt.Printf("error while creating folder: %v \n", folderError)
		log.Printf("ERROR: %s\n", folderError)
		return
	}
}

func SerializeFile(file multipart.File) (string, error) {
	defer file.Close()
	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func ReadUploadedFile(c *gin.Context, field string, fileType string) (interfaces.FileInterface, error) {
	cfg := providers.GetConfigByFileType(fileType)
	bodyLimit, err := providers.GetMainConfig().Server.RequestBodyLimit()
	if err != nil {
		return &interfaces.File{}, err
	}
	return readUploadedFile(c, field, fileType, cfg, bodyLimit)
}

func readUploadedFile(c *gin.Context, field string, fileType string, cfg providers.FileOptions, bodyLimit int64) (interfaces.FileInterface, error) {
	fileInterface := &interfaces.File{}
	if c == nil || c.Request == nil {
		return fileInterface, fmt.Errorf("cannot read %s without an HTTP request", field)
	}
	if bodyLimit <= 0 {
		return fileInterface, fmt.Errorf("request body limit must be positive")
	}
	if cfg.MaxFileSize == nil || *cfg.MaxFileSize <= 0 {
		return fileInterface, fmt.Errorf("files.%s.max_file_size must be positive", fileType)
	}
	if cfg.FileFolder == nil || strings.TrimSpace(*cfg.FileFolder) == "" {
		return fileInterface, fmt.Errorf("files.%s.file_folder must not be empty", fileType)
	}

	if c.Request.MultipartForm != nil && !requestHasBodyLimit(c, bodyLimit) {
		return fileInterface, fmt.Errorf("multipart body was parsed before ReadUploadedFile could apply its size limit")
	}
	if c.Request.ContentLength > bodyLimit {
		return fileInterface, &http.MaxBytesError{Limit: bodyLimit}
	}
	applyRequestBodyLimit(c, bodyLimit)

	file, fileHeader, err := c.Request.FormFile(field)
	if err != nil {
		return fileInterface, err
	}
	defer file.Close()

	if fileHeader.Size > int64(*cfg.MaxFileSize) {
		return fileInterface, fmt.Errorf("%s exceeds %dB limit", field, *cfg.MaxFileSize)
	}

	filename := sanitizeFilename(fileHeader.Filename)
	if filename == "" {
		return fileInterface, fmt.Errorf("%s has an invalid filename", field)
	}

	content, err := io.ReadAll(io.LimitReader(file, int64(*cfg.MaxFileSize)+1))
	if err != nil {
		return fileInterface, fmt.Errorf("unable to read %s", field)
	}
	if len(content) == 0 {
		return fileInterface, fmt.Errorf("%s cannot be empty", field)
	}
	if len(content) > *cfg.MaxFileSize {
		return fileInterface, fmt.Errorf("%s exceeds %dB limit", field, *cfg.MaxFileSize)
	}

	CreateFolder(*cfg.FileFolder)
	fileInterface.SetName(filename)
	fileInterface.SetContent(string(content))
	fileInterface.SetExtension(fileExtension(fileHeader.Filename))
	fileInterface.SetPath(*cfg.FileFolder + filename)
	fileInterface.SetType(fileType)

	return fileInterface, nil
}

func requestHasBodyLimit(c *gin.Context, maximum int64) bool {
	applied, exists := c.Get(requestBodyLimitKey)
	limit, ok := applied.(int64)
	return exists && ok && limit > 0 && limit <= maximum
}

func applyRequestBodyLimit(c *gin.Context, limit int64) {
	if requestHasBodyLimit(c, limit) || c.Request.Body == nil {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	c.Set(requestBodyLimitKey, limit)
}

func fileExtension(filename string) string {
	extension := strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
	return strings.TrimPrefix(extension, ".")
}

func sanitizeFilename(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	trimmed = strings.ReplaceAll(trimmed, "\\", "/")
	name := strings.TrimSpace(path.Base(trimmed))
	if name == "" || name == "." || name == "/" {
		return ""
	}

	return name
}
