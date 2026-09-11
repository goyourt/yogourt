package database

import (
	"strings"
	"testing"

	"github.com/goyourt/yogourt/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"
)

func dryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatalf("open dummy db: %v", err)
	}
	return db
}

func TestCapabilityRejectsTypedNil(t *testing.T) {
	var nilRecord *auditRecord
	var asAny any = nilRecord

	if _, ok := capability[interfaces.Resource](asAny); ok {
		t.Error("a typed-nil pointer must not pass a capability assertion")
	}
	if _, ok := capability[interfaces.Resource](nil); ok {
		t.Error("nil must not pass a capability assertion")
	}
	if _, ok := capability[interfaces.Resource](&auditRecord{}); !ok {
		t.Error("a live pointer implementing the capability must pass")
	}
}

func TestActorIdSkipsUnpersistedActor(t *testing.T) {
	if _, ok := actorId(&auditRecord{}); ok {
		t.Error("an actor without an id must not produce an audit attribution")
	}

	id := 7
	actor := &auditRecord{}
	actor.ID = &id
	got, ok := actorId(actor)
	if !ok || got != 7 {
		t.Errorf("expected actor 7, got %d (ok=%v)", got, ok)
	}
}

func TestRequireCompletePrimaryKey(t *testing.T) {
	db := dryDB(t)

	if err := requireCompletePrimaryKey(db, &auditRecord{}); err == nil {
		t.Error("an empty primary key must be refused")
	}

	id := 3
	record := &auditRecord{}
	record.ID = &id
	if err := requireCompletePrimaryKey(db, record); err != nil {
		t.Errorf("a set primary key must pass, got %v", err)
	}

	if err := requireCompletePrimaryKey(db, nil); err == nil {
		t.Error("nil must be refused")
	}
}

func TestCopyPrimaryKeyFieldsCopiesOnlyThePK(t *testing.T) {
	db := dryDB(t)

	id := 9
	from := &auditRecord{Name: "loaded"}
	from.ID = &id
	to := &auditRecord{Name: "incoming"}

	if err := copyPrimaryKeyFields(db, from, to); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if to.ID == nil || *to.ID != 9 {
		t.Error("the primary key must be copied")
	}
	if to.Name != "incoming" {
		t.Error("non-key fields must be left alone")
	}
}

type joinedTag struct {
	interfaces.Base
	Label string
}

type joinedPost struct {
	interfaces.Base
	Tags         []*joinedTag `gorm:"many2many:joined_post_tags;"`
	AccessGroups []*joinedTag `gorm:"many2many:joined_post_access_groups;"`
}

func TestBuildQueryJoinsManyToManyFromMetadata(t *testing.T) {
	db := dryDB(t)

	query, err := buildQuery(db, &joinedPost{}, map[string]any{"tags.label": "go"})
	if err != nil {
		t.Fatalf("build query: %v", err)
	}

	var results []joinedPost
	sql := query.Session(&gorm.Session{DryRun: true}).Find(&results).Statement.SQL.String()

	for _, fragment := range []string{
		"LEFT JOIN `joined_post_tags` ON `joined_post_tags`.`joined_post_id` = `joined_posts`.`id`",
		"LEFT JOIN `joined_tags` `Tags` ON `Tags`.`id` = `joined_post_tags`.`joined_tag_id`",
		"`Tags`.`label`",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("expected %q in the generated SQL, got:\n%s", fragment, sql)
		}
	}
}

// dairy.ToTitle lowercases interior capitals: the relation lookup and the
// condition alias must still resolve a camelCase filter prefix to the exact
// GORM relation name.
func TestBuildQueryResolvesMultiWordRelationNames(t *testing.T) {
	db := dryDB(t)

	query, err := buildQuery(db, &joinedPost{}, map[string]any{"accessGroups.label": "go"})
	if err != nil {
		t.Fatalf("build query: %v", err)
	}

	var results []joinedPost
	sql := query.Session(&gorm.Session{DryRun: true}).Find(&results).Statement.SQL.String()

	for _, fragment := range []string{
		"LEFT JOIN `joined_tags` `AccessGroups`",
		"`AccessGroups`.`label`",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("expected %q in the generated SQL, got:\n%s", fragment, sql)
		}
	}
}

type valueGetterModel struct {
	interfaces.Base
	Owned joinedTag
}

func (m *valueGetterModel) GetOwned() joinedTag  { return m.Owned }
func (m *valueGetterModel) SetOwned(o joinedTag) { m.Owned = o }

type badSetterModel struct {
	interfaces.Base
	Owned *joinedTag
}

func (m *badSetterModel) GetOwned() *joinedTag { return m.Owned }
func (m *badSetterModel) SetOwned(o int)       {}

func TestUpsertRelationsPrevalidation(t *testing.T) {
	db := dryDB(t)

	err := upsertRelations(db, DataWriter{db: db}, &valueGetterModel{}, []string{"Owned"})
	if err == nil {
		t.Error("a value-returning getter must be refused, not panic")
	}

	err = upsertRelations(db, DataWriter{db: db}, &badSetterModel{Owned: &joinedTag{}}, []string{"Owned"})
	if err == nil {
		t.Error("a setter with a mismatched signature must be refused before any write")
	}
}
