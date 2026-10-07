package database

import (
	"strings"
	"testing"

	"gorm.io/gorm"
)

type queryFilterRecord struct {
	ID       int `gorm:"primaryKey"`
	TenantID int
	Name     string
	Email    string
}

func (queryFilterRecord) TableName() string { return "query_filter_records" }

func renderedQuery(t *testing.T, query *gorm.DB) (*gorm.Statement, string) {
	t.Helper()
	var records []queryFilterRecord
	statement := query.Session(&gorm.Session{DryRun: true}).Find(&records).Statement
	return statement, statement.SQL.String()
}

func TestBuildQueryTreatsOrdinaryStringsAsLiteralEquality(t *testing.T) {
	for _, value := range []string{"LIKE%literal%", "OR%literal%"} {
		query, err := buildQuery(dryDB(t), &queryFilterRecord{}, map[string]any{"name": value})
		if err != nil {
			t.Fatalf("build query for %q: %v", value, err)
		}
		statement, sql := renderedQuery(t, query)
		if !strings.Contains(sql, "`query_filter_records`.`name` = ?") {
			t.Fatalf("ordinary string must use equality, got %s", sql)
		}
		if strings.Contains(sql, " LIKE ") || strings.Contains(sql, " OR ") {
			t.Fatalf("ordinary string selected an operator: %s", sql)
		}
		if len(statement.Vars) != 1 || statement.Vars[0] != value {
			t.Fatalf("ordinary string must remain one bound literal, got %#v", statement.Vars)
		}
	}
}

func TestBuildQueryUsesTypedLikeAndGroupedAlternatives(t *testing.T) {
	query, err := buildQuery(dryDB(t), &queryFilterRecord{}, map[string]any{
		"tenant_id": 42,
		"name":      Or(Like("alice")),
		"email":     Or(Like("example.test")),
	})
	if err != nil {
		t.Fatalf("build query: %v", err)
	}
	_, sql := renderedQuery(t, query)
	if !strings.Contains(sql, "`query_filter_records`.`tenant_id` = ? AND (") {
		t.Fatalf("ordinary constraint must be outside the alternatives group: %s", sql)
	}
	if strings.Count(sql, " LIKE ?") != 2 || !strings.Contains(sql, " OR ") {
		t.Fatalf("typed alternatives must form one OR group: %s", sql)
	}
}

func TestBuildQueryKeepsExplicitAndOutsideAlternatives(t *testing.T) {
	query, err := buildQuery(dryDB(t), &queryFilterRecord{}, map[string]any{
		"tenant_id": And(42),
		"name":      Or(Like("alice")),
		"email":     Or(Like("example.test")),
	})
	if err != nil {
		t.Fatalf("build query: %v", err)
	}
	statement, sql := renderedQuery(t, query)
	if !strings.Contains(sql, "`query_filter_records`.`tenant_id` = ? AND (") {
		t.Fatalf("explicit And must stay outside the alternatives group: %s", sql)
	}
	if len(statement.Vars) != 3 || statement.Vars[0] != 42 {
		t.Fatalf("And must bind its value as one literal, got %#v", statement.Vars)
	}

	query, err = buildQuery(dryDB(t), &queryFilterRecord{}, map[string]any{"name": And(Like("alice"))})
	if err != nil {
		t.Fatalf("build query with And(Like): %v", err)
	}
	_, sql = renderedQuery(t, query)
	if !strings.Contains(sql, "`query_filter_records`.`name` LIKE ?") || strings.Contains(sql, " OR ") {
		t.Fatalf("And must keep the wrapped operator: %s", sql)
	}

	for name, value := range map[string]any{
		"and in or":  Or(And("alice")),
		"or in and":  And(Or("alice")),
		"and in and": And(And("alice")),
		"or in or":   Or(Or("alice")),
	} {
		if _, err := buildQuery(dryDB(t), &queryFilterRecord{}, map[string]any{"name": value}); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
}

func TestBuildQueryRejectsUnknownAndRawFilterExpressions(t *testing.T) {
	if _, err := buildQuery(dryDB(t), &queryFilterRecord{}, map[string]any{"missing": "value"}); err == nil {
		t.Fatal("unknown model column must be rejected")
	}
	if _, err := buildQuery(dryDB(t), &joinedPost{}, map[string]any{"missing.label": "value"}); err == nil {
		t.Fatal("unknown relation must be rejected")
	}
	if _, err := buildQuery(dryDB(t), &joinedPost{}, map[string]any{"tags.missing": "value"}); err == nil {
		t.Fatal("unknown relation column must be rejected")
	}
	if _, err := buildQuery(dryDB(t), &queryFilterRecord{}, map[string]any{"name": gorm.Expr("CURRENT_USER")}); err == nil {
		t.Fatal("raw GORM expression must not be accepted as a filter value")
	}
}

func TestBuildQueryUsesTypedSchemaValidatedOrdering(t *testing.T) {
	query, err := buildQuery(dryDB(t), &queryFilterRecord{}, map[string]any{
		orderByPattern: []Ordering{
			OrderBy("name", Ascending),
			OrderBy("email", Descending),
		},
	})
	if err != nil {
		t.Fatalf("build ordered query: %v", err)
	}
	_, sql := renderedQuery(t, query)
	if !strings.Contains(sql, "ORDER BY `query_filter_records`.`name`,`query_filter_records`.`email` DESC") {
		t.Fatalf("ordering must use quoted schema columns: %s", sql)
	}

	invalid := []any{
		"name DESC",
		OrderBy("missing", Ascending),
		OrderBy("name", Direction(99)),
	}
	for _, value := range invalid {
		if _, err := buildQuery(dryDB(t), &queryFilterRecord{}, map[string]any{orderByPattern: value}); err == nil {
			t.Fatalf("invalid ordering %#v must be rejected", value)
		}
	}
}

func TestIdentityQueryUsesExactBoundEquality(t *testing.T) {
	value := "LIKE%opaque-public-id%"
	query, err := buildIdentityQuery(dryDB(t), &queryFilterRecord{}, "name", value)
	if err != nil {
		t.Fatalf("build identity query: %v", err)
	}
	statement, sql := renderedQuery(t, query)
	if !strings.Contains(sql, "`query_filter_records`.`name` = ?") || strings.Contains(sql, " LIKE ") {
		t.Fatalf("identity must use equality: %s", sql)
	}
	if len(statement.Vars) != 1 || statement.Vars[0] != value {
		t.Fatalf("identity must remain one bound string, got %#v", statement.Vars)
	}
	if _, err := buildIdentityQuery(dryDB(t), &queryFilterRecord{}, "missing", value); err == nil {
		t.Fatal("unknown identity column must be rejected")
	}
}

func TestUpsertExactLookupRequiresLiteralIdentity(t *testing.T) {
	db := dryDB(t)
	if err := getOneByExact(db, &queryFilterRecord{}, nil); err == nil {
		t.Fatal("empty upsert identity must be rejected")
	}
	for name, value := range map[string]any{
		"like operator":  Like("alice"),
		"or operator":    Or("alice"),
		"and operator":   And("alice"),
		"slice":          []string{"alice", "bob"},
		"raw expression": gorm.Expr("CURRENT_USER"),
	} {
		t.Run(name, func(t *testing.T) {
			if err := getOneByExact(db, &queryFilterRecord{}, map[string]any{"name": value}); err == nil {
				t.Fatal("non-literal upsert identity must be rejected")
			}
		})
	}
}

func TestValidatedRelationNameUsesSchema(t *testing.T) {
	name, err := validatedRelationName(dryDB(t), &joinedPost{}, "accessGroups")
	if err != nil {
		t.Fatalf("resolve relation: %v", err)
	}
	if name != "AccessGroups" {
		t.Fatalf("expected schema relation name, got %q", name)
	}
	if _, err := validatedRelationName(dryDB(t), &joinedPost{}, "missing"); err == nil {
		t.Fatal("unknown hydration relation must be rejected")
	}
}
