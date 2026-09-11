package database

import (
	"errors"
	"testing"

	"github.com/goyourt/yogourt/interfaces"
	"gorm.io/gorm"
)

// customerRecord has an application-assigned primary key: the framework must
// never reset, erase or overwrite it.
type customerRecord struct {
	PublicId *string `gorm:"primaryKey"`
	Name     string
}

func (customerRecord) TableName() string { return "customer_records" }

func (c *customerRecord) GetPublicId() string {
	if c.PublicId == nil {
		return ""
	}
	return *c.PublicId
}

func (c *customerRecord) PublicIdColumn() string { return "public_id" }

func setupCustomerTable(t *testing.T) *gorm.DB {
	t.Helper()
	db := openDB(t)
	if err := db.AutoMigrate(&customerRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Exec(`TRUNCATE customer_records`).Error; err != nil {
		t.Fatalf("truncate customer_records: %v", err)
	}
	return db
}

func stringPtr(s string) *string { return &s }

func TestGetOneByLeavesCallerIntactOnMiss(t *testing.T) {
	db := setupCustomerTable(t)

	obj := &customerRecord{PublicId: stringPtr("cus_unknown"), Name: "incoming"}
	err := getOneBy(db, obj, map[string]any{"public_id": "cus_unknown"})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}
	if obj.PublicId == nil || *obj.PublicId != "cus_unknown" || obj.Name != "incoming" {
		t.Error("a failed lookup must leave the caller's object intact")
	}
}

func TestCreatePreservesApplicativePrimaryKey(t *testing.T) {
	db := setupCustomerTable(t)

	writer := DataWriter{db: db}
	obj := &customerRecord{PublicId: stringPtr("cus_a8x3k"), Name: "alice"}
	if err := writer.Create(obj); err != nil {
		t.Fatalf("create: %v", err)
	}
	if obj.PublicId == nil || *obj.PublicId != "cus_a8x3k" {
		t.Error("an application-assigned primary key must survive Create")
	}
}

func TestUpsertWithApplicativePrimaryKey(t *testing.T) {
	db := setupCustomerTable(t)
	writer := DataWriter{db: db}

	obj := &customerRecord{PublicId: stringPtr("cus_a8x3k"), Name: "alice"}
	if err := writer.Upsert(obj, map[string]any{"public_id": "cus_a8x3k"}); err != nil {
		t.Fatalf("upsert create: %v", err)
	}
	if *obj.PublicId != "cus_a8x3k" {
		t.Error("upsert-create must keep the application-assigned key")
	}

	update := &customerRecord{PublicId: stringPtr("cus_a8x3k"), Name: "alice2"}
	if err := writer.Upsert(update, map[string]any{"public_id": "cus_a8x3k"}); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	if update.Name != "alice2" {
		t.Error("upsert-update must keep the caller's new values")
	}

	var count int64
	db.Raw(`SELECT COUNT(*) FROM customer_records`).Scan(&count)
	if count != 1 {
		t.Errorf("expected one row after upserting twice, got %d", count)
	}
}

func TestCreateResetsGeneratedIdentity(t *testing.T) {
	db := setupAuditTable(t)

	chosenId := 424242
	chosenUuid := "11111111-1111-1111-1111-111111111111"
	record := &auditRecord{Name: "client"}
	record.ID = &chosenId
	record.Uuid = &chosenUuid

	writer := DataWriter{db: db}
	if err := writer.Create(record); err != nil {
		t.Fatalf("create: %v", err)
	}
	if record.ID != nil && *record.ID == chosenId {
		t.Error("a generated primary key must be reset on Create")
	}
	var count int64
	db.Raw(`SELECT COUNT(*) FROM audit_records WHERE uuid = ?`, chosenUuid).Scan(&count)
	if count != 0 {
		t.Error("a client-chosen uuid must never be inserted as a generated identity")
	}
}

func TestUpdateMissingRowFails(t *testing.T) {
	db := setupAuditTable(t)

	id := 424242
	record := &auditRecord{Name: "ghost"}
	record.ID = &id
	writer := DataWriter{db: db}
	if err := writer.Update(record); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("updating a missing row must fail, got %v", err)
	}
}

func TestUpdateDoesNotWriteAssociations(t *testing.T) {
	db := setupRelationTables(t)

	owner := &ownerRecord{Name: "alice"}
	if err := db.Create(owner).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	attacker := &securityRecord{Level: "admin"}
	attacker.Uuid = stringPtr("22222222-2222-2222-2222-222222222222")
	owner.Security = attacker

	writer := DataWriter{db: db}
	if err := writer.Update(owner); err != nil {
		t.Fatalf("update: %v", err)
	}

	var count int64
	db.Raw(`SELECT COUNT(*) FROM security_records`).Scan(&count)
	if count != 0 {
		t.Errorf("Update must never write associations implicitly, found %d security rows", count)
	}
}

func TestAuditedDeleteMissingRowFails(t *testing.T) {
	db := setupAuditTable(t)

	actor := createAuditRecord(t, db, "actor")
	id := 424242
	record := &auditRecord{}
	record.ID = &id

	writer := DataWriter{db: db, CurrentUser: actor}
	if err := writer.Delete(record); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("an audited delete of a missing row must fail, got %v", err)
	}
}

func TestDeleteMissingRowFails(t *testing.T) {
	db := setupAuditTable(t)

	id := 424242
	record := &auditRecord{}
	record.ID = &id
	writer := DataWriter{db: db}
	if err := writer.Delete(record); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("deleting a missing row must fail, got %v", err)
	}
}

type securityRecord struct {
	interfaces.Base
	Level string
}

func (securityRecord) TableName() string { return "security_records" }

type ownerRecord struct {
	interfaces.Base
	Name       string
	SecurityId *int
	Security   *securityRecord
}

func (ownerRecord) TableName() string { return "owner_records" }

func (o *ownerRecord) GetSecurity() *securityRecord { return o.Security }

func (o *ownerRecord) SetSecurity(s *securityRecord) { o.Security = s }

func setupRelationTables(t *testing.T) *gorm.DB {
	t.Helper()
	db := openDB(t)
	if err := db.AutoMigrate(&securityRecord{}, &ownerRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, table := range []string{"owner_records", "security_records"} {
		if err := db.Exec(`TRUNCATE ` + table + ` RESTART IDENTITY CASCADE`).Error; err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	return db
}

func TestUpsertRelationsRejectsUnknownPublicId(t *testing.T) {
	db := setupRelationTables(t)

	owner := &ownerRecord{Security: &securityRecord{Level: "high"}}
	owner.Security.Uuid = stringPtr("11111111-1111-1111-1111-111111111111")

	err := upsertRelations(db, DataWriter{db: db}, owner, []string{"Security"})
	if err == nil {
		t.Fatal("a client-provided unknown public id must be an error, never an implicit create")
	}

	var count int64
	db.Raw(`SELECT COUNT(*) FROM security_records`).Scan(&count)
	if count != 0 {
		t.Errorf("no row must be created for an unknown public id, found %d", count)
	}
}

func TestUpsertRelationsCreatesWhenPublicIdIsEmpty(t *testing.T) {
	db := setupRelationTables(t)

	owner := &ownerRecord{Security: &securityRecord{Level: "high"}}
	if err := upsertRelations(db, DataWriter{db: db}, owner, []string{"Security"}); err != nil {
		t.Fatalf("upsert relations: %v", err)
	}

	if owner.Security.ID == nil || *owner.Security.ID == 0 {
		t.Error("the created relation must carry its primary key back")
	}
}

func TestUpsertRelationsUpdatesKnownPublicId(t *testing.T) {
	db := setupRelationTables(t)

	existing := &securityRecord{Level: "low"}
	if err := db.Create(existing).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.First(existing).Error; err != nil {
		t.Fatalf("reload seed: %v", err)
	}

	owner := &ownerRecord{Security: &securityRecord{Level: "high"}}
	owner.Security.Uuid = existing.Uuid
	if err := upsertRelations(db, DataWriter{db: db}, owner, []string{"Security"}); err != nil {
		t.Fatalf("upsert relations: %v", err)
	}

	var level string
	db.Raw(`SELECT level FROM security_records WHERE id = ?`, *existing.ID).Scan(&level)
	if level != "high" {
		t.Errorf("the known relation must be updated, level is %q", level)
	}
	var count int64
	db.Raw(`SELECT COUNT(*) FROM security_records`).Scan(&count)
	if count != 1 {
		t.Errorf("expected a single row, got %d", count)
	}
}
