package migrations

import (
	"fmt"
	"os"
	"sort"
	"time"

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

func Migrate(Up map[string]func(*gorm.DB), Down map[string]func(*gorm.DB), args []string) {
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
		migration, found := migrations[timestamp]
		if !found {
			fmt.Printf("migration %s not found\n", timestamp)
			os.Exit(1)
		}
		executeMigration(migration, timestamp, down)
		return
	}

	if down {
		fmt.Println("--down requires a migration timestamp")
		os.Exit(1)
	}

	timestamps := make([]string, 0, len(migrations))
	for timestamp := range migrations {
		timestamps = append(timestamps, timestamp)
	}
	sort.Strings(timestamps)

	for _, timestamp := range timestamps {
		executeMigration(migrations[timestamp], timestamp, down)
	}
}

func executeMigration(migration func(*gorm.DB), timestamp string, down bool) {
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

	migration(db)

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
