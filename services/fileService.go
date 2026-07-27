package services

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/interfaces"
	"github.com/goyourt/yogourt/services/providers"
)

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
	file, fileHeader, err := c.Request.FormFile(field)
	fileInterface := &interfaces.File{}
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
