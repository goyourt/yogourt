package services

import (
	"fmt"
	"io/ioutil"
	"log"
	"mime/multipart"
	"os"

	"github.com/goyourt/yogourt/interfaces"
)

const FileFolder = "./public/files/"

func SaveFile(f interfaces.FileInterface) {
	path := FileFolder + f.GetUuid()
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
	folderError := os.Mkdir(folderPath, os.ModePerm)
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
