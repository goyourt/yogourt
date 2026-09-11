package migrations

import (
	"bytes"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/goyourt/yogourt/interfaces"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

//go:embed v1_to_v2.sql
var v1ToV2SQL string

// LegacyBase is the v1 interfaces.Base definition at c9bd648^, retained here
// only to generate the schema that existing v1 applications created.
type LegacyBase struct {
	ID          *int      `gorm:"primaryKey;autoIncrement;not null;unique"`
	Uuid        *string   `gorm:"type:uuid;default:gen_random_uuid();not null;unique"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	CreatedById *int
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
	UpdatedById *int
	DeletedAt   gorm.DeletedAt
	DeletedById *int
}

type legacyBaseRecord struct {
	LegacyBase
	Name string
}

type v2BaseRecord struct {
	interfaces.Base
	Name string
}

func openMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("YOGOURT_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("YOGOURT_MIGRATION_TEST_DSN not set; skipping v1-to-v2 migration integration test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open migration test database: %v", err)
	}
	return db
}

func TestV1ToV2(t *testing.T) {
	db := openMigrationDB(t)
	suffix := fmt.Sprintf("migration_v%d", time.Now().UnixNano())
	legacyTable := suffix + "_legacy"
	freshTable := suffix + "_fresh"
	t.Cleanup(func() {
		if err := db.Exec("DROP TABLE IF EXISTS " + quoteIdentifier(legacyTable)).Error; err != nil {
			t.Errorf("drop legacy test table: %v", err)
		}
		if err := db.Exec("DROP TABLE IF EXISTS " + quoteIdentifier(freshTable)).Error; err != nil {
			t.Errorf("drop fresh test table: %v", err)
		}
	})

	if err := db.Table(legacyTable).AutoMigrate(&legacyBaseRecord{}); err != nil {
		t.Fatalf("create v1 Base table: %v", err)
	}

	const uuid = "8ce5b518-43b1-4fcb-9b96-730fccf1ce7b"
	createdAt := time.Date(2026, 9, 11, 9, 30, 0, 0, time.UTC)
	if err := db.Exec(
		"INSERT INTO "+quoteIdentifier(legacyTable)+
			" (id, uuid, created_at, created_by_id, updated_at, updated_by_id, deleted_at, deleted_by_id, name) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		41, uuid, createdAt, 7, createdAt.Add(time.Hour), 8, createdAt.Add(2*time.Hour), 9, "preserved",
	).Error; err != nil {
		t.Fatalf("insert v1 data: %v", err)
	}

	assertLegacyPrimaryKey(t, db, legacyTable, "uni_"+legacyTable+"_id")
	if err := db.Exec(v1ToV2SQL).Error; err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	assertLegacyPrimaryKey(t, db, legacyTable, legacyTable+"_pkey")
	assertPreservedRow(t, db, legacyTable, uuid, createdAt)
	assertIndex(t, db, legacyTable, "idx_"+legacyTable+"_deleted_at")

	firstDump := normalizeDump(schemaDump(t, legacyTable), legacyTable)
	if err := db.Exec(v1ToV2SQL).Error; err != nil {
		t.Fatalf("reapply migration: %v", err)
	}
	if got := normalizeDump(schemaDump(t, legacyTable), legacyTable); got != firstDump {
		t.Fatalf("migration is not idempotent: schema dump changed on the second application:\n%s", dumpDiff(got, firstDump))
	}
	assertPreservedRow(t, db, legacyTable, uuid, createdAt)

	if err := db.Table(freshTable).AutoMigrate(&v2BaseRecord{}); err != nil {
		t.Fatalf("create fresh v2 Base table: %v", err)
	}
	legacySchema := firstDump
	freshSchema := normalizeDump(schemaDump(t, freshTable), freshTable)
	if legacySchema != freshSchema {
		t.Fatalf("migrated v1 schema differs from fresh v2 schema:\n%s", dumpDiff(legacySchema, freshSchema))
	}
}

func assertLegacyPrimaryKey(t *testing.T, db *gorm.DB, table, want string) {
	t.Helper()
	var got string
	if err := db.Raw(`SELECT conname FROM pg_constraint WHERE conrelid = ?::regclass AND contype = 'p'`, "public."+table).Scan(&got).Error; err != nil {
		t.Fatalf("read primary-key constraint: %v", err)
	}
	if got != want {
		t.Fatalf("primary-key constraint = %q, want %q", got, want)
	}
}

func assertIndex(t *testing.T, db *gorm.DB, table, want string) {
	t.Helper()
	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM pg_indexes WHERE schemaname = 'public' AND tablename = ? AND indexname = ?`, table, want).Scan(&count).Error; err != nil {
		t.Fatalf("read deleted_at index: %v", err)
	}
	if count != 1 {
		t.Fatalf("deleted_at index %q missing from %q", want, table)
	}
}

func assertPreservedRow(t *testing.T, db *gorm.DB, table, uuid string, createdAt time.Time) {
	t.Helper()
	var row struct {
		ID          int
		Uuid        string
		CreatedAt   time.Time
		CreatedByID int
		UpdatedAt   time.Time
		UpdatedByID int
		DeletedAt   sql.NullTime
		DeletedByID int
		Name        string
	}
	if err := db.Raw("SELECT id, uuid::text, created_at, created_by_id, updated_at, updated_by_id, deleted_at, deleted_by_id, name FROM "+quoteIdentifier(table)+" WHERE uuid = ?", uuid).Scan(&row).Error; err != nil {
		t.Fatalf("read migrated v1 row: %v", err)
	}
	if row.ID != 41 || row.Uuid != uuid || !row.CreatedAt.Equal(createdAt) || row.CreatedByID != 7 || !row.UpdatedAt.Equal(createdAt.Add(time.Hour)) || row.UpdatedByID != 8 || !row.DeletedAt.Valid || !row.DeletedAt.Time.Equal(createdAt.Add(2*time.Hour)) || row.DeletedByID != 9 || row.Name != "preserved" {
		t.Fatalf("migration changed row: %+v", row)
	}
}

func schemaDump(t *testing.T, table string) string {
	t.Helper()
	command := exec.Command("pg_dump", "--schema-only", "--no-owner", "--no-privileges", "--table=public."+table, os.Getenv("YOGOURT_MIGRATION_TEST_DSN"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("pg_dump schema for %q: %v\n%s", table, err, output)
	}
	return string(output)
}

func normalizeDump(dump, table string) string {
	dump = strings.ReplaceAll(dump, table, "base_records")
	lines := make([]string, 0)
	for _, line := range strings.Split(dump, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, "\\restrict") || strings.HasPrefix(trimmed, "\\unrestrict") {
			continue
		}
		lines = append(lines, line)
	}
	return normalizeCreateTableMembers(strings.Join(lines, "\n"))
}

func normalizeCreateTableMembers(dump string) string {
	const createTable = "CREATE TABLE public.base_records ("
	start := strings.Index(dump, createTable)
	if start < 0 {
		return dump
	}
	bodyStart := start + len(createTable)
	bodyEnd := strings.Index(dump[bodyStart:], "\n);")
	if bodyEnd < 0 {
		return dump
	}
	bodyEnd += bodyStart
	members := strings.Split(dump[bodyStart:bodyEnd], "\n")
	sort.Strings(members)
	return dump[:bodyStart] + strings.Join(members, "\n") + dump[bodyEnd:]
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func dumpDiff(got, want string) string {
	return "migrated:\n" + indent(got) + "\nfresh:\n" + indent(want)
}

func indent(value string) string {
	return string(bytes.ReplaceAll([]byte(value), []byte("\n"), []byte("\n  ")))
}
