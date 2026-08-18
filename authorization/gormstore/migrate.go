package gormstore

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"gorm.io/gorm"
)

// migrationFiles embeds the versioned SQL migrations shipped with the
// package. Files are applied in lexical order of their name, so new
// migrations must use a greater numeric prefix (0002_..., 0003_...).
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate applies the embedded SQL migrations of the RBAC store. It is
// idempotent (every statement uses IF NOT EXISTS) and strictly opt-in: the
// integrator calls it explicitly, typically at deploy time — the store never
// runs it automatically and there is no AutoMigrate involved.
func Migrate(ctx context.Context, db *gorm.DB) error {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("gormstore: read embedded migrations: %w", err)
	}

	// fs.ReadDir returns entries sorted by name, which is the version order.
	for _, entry := range entries {
		name := entry.Name()
		content, err := fs.ReadFile(migrationFiles, "migrations/"+name)
		if err != nil {
			return fmt.Errorf("gormstore: read migration %s: %w", name, err)
		}
		err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return tx.Exec(string(content)).Error
		})
		if err != nil {
			return fmt.Errorf("gormstore: apply migration %s: %w", name, err)
		}
	}

	return nil
}
