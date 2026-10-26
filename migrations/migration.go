package migrations

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/goyourt/yogourt/dairy"
	"github.com/goyourt/yogourt/services/providers"
	"gorm.io/gorm"
)

type migrationRecord struct {
	ID         uint      `gorm:"primaryKey"`
	Timestamp  string    `gorm:"column:timestamp"`
	ExecutedAt time.Time `gorm:"column:executed_at"`
}

func (migrationRecord) TableName() string {
	return "migrations"
}

func InitMigrations() error {
	db, err := providers.GetDB()
	if err != nil {
		return err
	}

	return db.AutoMigrate(&migrationRecord{})
}

func Migrate(Up []func(), Down []func(), args []string) {
	down := false
	timestamp := ""
	for _, arg := range args {
		switch arg {
		case "--down":
			down = true
		case "--up":
			break
		default:
			timestamp = arg
		}
	}

	migrations := Up
	if down {
		migrations = Down
	}

	if timestamp != "" {
		found := false
		for _, migration := range migrations {
			if len(timestamp) == 12 && strings.Contains(dairy.FuncName(migration), timestamp) {
				executeMigration(migration, down)
				found = true
			}
		}
		if !found {
			fmt.Printf("migration %s not found\n", timestamp)
			os.Exit(1)
		}
		return
	}

	if down {
		fmt.Println("--down requires a migration timestamp")
		os.Exit(1)
	}

	for _, migration := range migrations {
		executeMigration(migration, down)
	}
}

func executeMigration(migration func(), down bool) {
	name := dairy.FuncName(migration)
	timestamp := strings.TrimPrefix(strings.TrimPrefix(name, "Up"), "Down")

	db, err := providers.GetDB()
	if err != nil {
		fmt.Printf("error while connecting database: %v\n", err)
		os.Exit(1)
	}

	var count int64
	if err = db.Model(&migrationRecord{}).Where("timestamp = ?", timestamp).Count(&count).Error; err != nil {
		fmt.Printf("error while reading migrations table: %v\n", err)
		os.Exit(1)
	}

	alreadyExecuted := count > 0
	if down != alreadyExecuted {
		fmt.Printf("migration %s skipped\n", timestamp)
		return
	}

	migration()

	var result *gorm.DB
	if down {
		result = db.Where("timestamp = ?", timestamp).Delete(&migrationRecord{})
	} else {
		result = db.Create(&migrationRecord{Timestamp: timestamp, ExecutedAt: time.Now()})
	}
	if result.Error != nil {
		fmt.Printf("error while updating migrations table: %v\n", result.Error)
		os.Exit(1)
	}
}
