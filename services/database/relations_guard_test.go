package database

import (
	"testing"

	"github.com/goyourt/yogourt/interfaces"
)

type guardModel struct {
	interfaces.Base
	Tags []*guardTag
}

type guardTag struct {
	interfaces.Base
}

// The early-return paths must never reach the database provider: calling it
// without a configured PostgreSQL would terminate the process, so a test
// reaching the preload branch would not merely fail — it would kill the run.
// That is exactly why the broken guard went unnoticed: it always returned
// early, so nothing ever exercised the query.

func TestHydrateManyToManyRelationSkipsLoadedSlice(t *testing.T) {
	model := &guardModel{Tags: []*guardTag{}}

	// An allocated slice, even empty, means the relation was loaded.
	if err := HydrateManyToManyRelation(model, "Tags", &model.Tags); err != nil {
		t.Fatalf("an allocated slice must be left alone, got %v", err)
	}

	model.Tags = []*guardTag{{}}
	if err := HydrateManyToManyRelation(model, "Tags", &model.Tags); err != nil {
		t.Fatalf("a filled slice must be left alone, got %v", err)
	}
}

func TestHydrateManyToManyRelationSkipsNilPointer(t *testing.T) {
	model := &guardModel{}

	if err := HydrateManyToManyRelation[*guardTag](model, "Tags", nil); err != nil {
		t.Fatalf("a nil pointer has nothing to fill, got %v", err)
	}
}
