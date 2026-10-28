package database

import (
	"testing"

	"github.com/goyourt/yogourt/interfaces"
)

type guardTag struct {
	interfaces.Base
}

// The early-return paths must never reach the database provider: calling it
// without a configured PostgreSQL would terminate the process, so a test
// reaching the preload branch would not merely fail — it would kill the run.
// That is exactly why the broken guard went unnoticed: it always returned
// early, so nothing ever exercised the query.

type guardPtrModel struct {
	interfaces.Base
	Tags *[]*guardTag `gorm:"many2many:guard_ptr_tag;"`
}

func TestHydrateManyToManyRelationSkipsLoadedField(t *testing.T) {
	loaded := &[]*guardTag{{}}
	model := &guardPtrModel{Tags: loaded}

	if err := HydrateManyToManyRelation(model, "Tags", &model.Tags); err != nil {
		t.Fatalf("a loaded field must be left alone, got %v", err)
	}
	if model.Tags != loaded {
		t.Fatal("a loaded field must not be replaced")
	}
}

func TestHydrateManyToManyRelationSkipsNilAddress(t *testing.T) {
	model := &guardPtrModel{}

	if err := HydrateManyToManyRelation[*guardTag](model, "Tags", nil); err != nil {
		t.Fatalf("a nil address has nothing to fill, got %v", err)
	}
}

// An unsaved object would reach the preload branch of
// HydrateManyToManyRelation only through the database provider, which would kill the run: completing
// this test proves the provider is never called.
func TestHydrateManyToManyRelationAllocatesUnsavedObject(t *testing.T) {
	model := &guardPtrModel{}

	if err := HydrateManyToManyRelation(model, "Tags", &model.Tags); err != nil {
		t.Fatalf("an unsaved object has nothing to load, got %v", err)
	}
	if model.Tags == nil {
		t.Fatal("the field must be allocated so callers can dereference it")
	}
	*model.Tags = append(*model.Tags, &guardTag{})
	if len(*model.Tags) != 1 {
		t.Fatalf("expected the allocated field to accept appends, got %d items", len(*model.Tags))
	}
}
