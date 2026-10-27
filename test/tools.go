package test

import (
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// These helpers check the GORM structure of a model without a database:
// schema.Parse reads the tags exactly like AutoMigrate or a Preload would, and
// reports the same errors (unknown foreign key, inconsistent join table...).

var schemaCache = &sync.Map{}

// ParseSchema parses model, failing the test when GORM rejects its tags.
func ParseSchema(t testing.TB, model any) *schema.Schema {
	t.Helper()
	sch, err := schema.Parse(model, schemaCache, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("unable to parse gorm schema of %T: %v", model, err)
	}
	return sch
}

// RequireValidSchemas parses every model of a registry (name -> model), each
// in its own subtest, and requires a primary key on all of them.
func RequireValidSchemas(t *testing.T, models map[string]any) {
	t.Helper()
	for name, model := range models {
		t.Run(name, func(t *testing.T) {
			sch := ParseSchema(t, model)
			if len(sch.PrimaryFields) == 0 {
				t.Fatalf("expected %s to have a primary key", name)
			}
		})
	}
}

func RequireRelation(t testing.TB, sch *schema.Schema, name string, relationType schema.RelationshipType) *schema.Relationship {
	t.Helper()
	relation, ok := sch.Relationships.Relations[name]
	if !ok {
		t.Fatalf("expected relation %s on %s", name, sch.Name)
	}
	if relation.Type != relationType {
		t.Fatalf("expected %s.%s to be %s, got %s", sch.Name, name, relationType, relation.Type)
	}
	return relation
}

// RequireBelongsTo checks that the foreign key is carried by the model itself
// (sch.Table.fkColumn -> targetTable.id).
func RequireBelongsTo(t testing.TB, sch *schema.Schema, name string, fkColumn string, targetTable string) {
	t.Helper()
	relation := RequireRelation(t, sch, name, schema.BelongsTo)
	if len(relation.References) != 1 {
		t.Fatalf("expected a single reference for %s.%s, got %d", sch.Name, name, len(relation.References))
	}
	ref := relation.References[0]
	if ref.ForeignKey.Schema.Table != sch.Table || ref.ForeignKey.DBName != fkColumn {
		t.Fatalf("expected %s.%s foreign key %s.%s, got %s.%s",
			sch.Name, name, sch.Table, fkColumn, ref.ForeignKey.Schema.Table, ref.ForeignKey.DBName)
	}
	if ref.PrimaryKey.Schema.Table != targetTable || ref.PrimaryKey.DBName != "id" {
		t.Fatalf("expected %s.%s to reference %s.id, got %s.%s",
			sch.Name, name, targetTable, ref.PrimaryKey.Schema.Table, ref.PrimaryKey.DBName)
	}
}

// RequireMany2Many checks the join table and both of its columns: ownColumn
// points to sch.Table.id, targetColumn to targetTable.id.
func RequireMany2Many(t testing.TB, sch *schema.Schema, name string, joinTable string, ownColumn string, targetColumn string, targetTable string) {
	t.Helper()
	relation := RequireRelation(t, sch, name, schema.Many2Many)
	if relation.JoinTable == nil || relation.JoinTable.Table != joinTable {
		t.Fatalf("expected %s.%s to use join table %s", sch.Name, name, joinTable)
	}
	if len(relation.References) != 2 {
		t.Fatalf("expected two references for %s.%s, got %d", sch.Name, name, len(relation.References))
	}
	for _, ref := range relation.References {
		expectedColumn, expectedTable := targetColumn, targetTable
		if ref.OwnPrimaryKey {
			expectedColumn, expectedTable = ownColumn, sch.Table
		}
		if ref.ForeignKey.DBName != expectedColumn || ref.PrimaryKey.Schema.Table != expectedTable {
			t.Fatalf("expected %s.%s join column %s -> %s.id, got %s -> %s.%s",
				sch.Name, name, expectedColumn, expectedTable,
				ref.ForeignKey.DBName, ref.PrimaryKey.Schema.Table, ref.PrimaryKey.DBName)
		}
	}
}

// RequireHydratableRelation mirrors the resolution done by
// database.HydrateRelation and HydrateManyToManyRelation: the name a getter
// passes must match, case-insensitively, the name of a GORM relation (the
// field name, not the table name), otherwise hydration fails.
func RequireHydratableRelation(t testing.TB, sch *schema.Schema, name string) {
	t.Helper()
	for candidate := range sch.Relationships.Relations {
		if strings.EqualFold(candidate, name) {
			return
		}
	}
	t.Fatalf("getter hydrates relation %q but %s has no such relation", name, sch.Name)
}

// RequireNoPanic runs fn and fails the test instead of aborting the whole run
// when fn panics.
func RequireNoPanic(t testing.TB, label string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%s panicked: %v", label, r)
		}
	}()
	fn()
}

// Ptr returns a pointer to value, handy to fill the pointer fields of a model.
func Ptr[T any](value T) *T {
	return &value
}
