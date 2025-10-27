package services

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/goyourt/yogourt/interfaces"
	"gorm.io/gorm"
)

func SaveFile(f interfaces.FileInterface) error {
	GenerateFile(f.GetUuid(), f.GetContent())

	db := GetDB()
	ctx := context.Background()

	err := gorm.G[interfaces.FileInterface](db, gorm.WithResult()).Create(ctx, &f)

	return err
}

func GenerateFile(fileName string, fileContent string) {
	file, fileError := os.Create(fileName)
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
