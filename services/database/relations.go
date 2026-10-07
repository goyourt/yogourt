package database

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/interfaces"
	"github.com/goyourt/yogourt/services/providers"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

const orderByPattern = "orderBy"

// buildQuery derives schema-validated joins, conditions and ordering from
// values. Ordinary filters are conjoined. Filters wrapped with Or are grouped
// as one set of alternatives, then that group is conjoined with the ordinary
// filters. Map iteration order therefore cannot broaden an ordinary constraint.
func buildQuery(db *gorm.DB, model any, values map[string]any) (*gorm.DB, error) {
	query := db.Model(model)
	if err := query.Statement.Parse(query.Statement.Model); err != nil {
		return nil, err
	}

	joined := []string{}
	var ordinary, alternatives []clause.Expression
	var ordering any
	for key, value := range values {
		if key == orderByPattern {
			ordering = value
			continue
		}

		var column clause.Column
		var err error
		query, column, err = resolveQueryColumn(query, key, &joined)
		if err != nil {
			return nil, err
		}
		expression, alternative, err := filterExpression(column, value)
		if err != nil {
			return nil, fmt.Errorf("filter %q: %w", key, err)
		}
		if alternative {
			alternatives = append(alternatives, expression)
		} else {
			ordinary = append(ordinary, expression)
		}
	}

	for _, expression := range ordinary {
		query = query.Where(expression)
	}
	if len(alternatives) > 0 {
		query = query.Where(clause.Or(alternatives...))
	}
	if ordering != nil {
		var err error
		query, err = addOrdering(query, ordering, &joined)
		if err != nil {
			return nil, err
		}
	}
	return query.Preload(clause.Associations), nil
}

func resolveQueryColumn(query *gorm.DB, key string, joined *[]string) (*gorm.DB, clause.Column, error) {
	parts := strings.Split(key, ".")
	switch len(parts) {
	case 1:
		field := query.Statement.Schema.LookUpField(parts[0])
		if field == nil || field.DBName == "" {
			return nil, clause.Column{}, fmt.Errorf("unknown column %q on %s", key, query.Statement.Schema.Name)
		}
		return query, clause.Column{Table: query.Statement.Table, Name: field.DBName}, nil
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return nil, clause.Column{}, fmt.Errorf("invalid related column %q", key)
		}
		relation, err := resolveRelation(query.Statement.Schema, parts[0])
		if err != nil {
			return nil, clause.Column{}, err
		}
		field := relation.FieldSchema.LookUpField(parts[1])
		if field == nil || field.DBName == "" {
			return nil, clause.Column{}, fmt.Errorf("unknown column %q on relation %s", parts[1], relation.Name)
		}
		if !slices.Contains(*joined, relation.Name) {
			query = joinRelation(query, relation)
			*joined = append(*joined, relation.Name)
		}
		return query, clause.Column{Table: relation.Name, Name: field.DBName}, nil
	default:
		return nil, clause.Column{}, fmt.Errorf("invalid related column %q", key)
	}
}

func resolveRelation(sch *schema.Schema, name string) (*schema.Relationship, error) {
	if sch == nil {
		return nil, fmt.Errorf("cannot resolve relation %q without a schema", name)
	}
	for candidate, relation := range sch.Relationships.Relations {
		if strings.EqualFold(candidate, name) {
			return relation, nil
		}
	}
	return nil, fmt.Errorf("unknown relation %q on %s", name, sch.Name)
}

// joinRelation joins one schema-resolved relation. Many-to-many joins are
// built from GORM metadata and bind tables and columns as structured clauses.
func joinRelation(query *gorm.DB, relation *schema.Relationship) *gorm.DB {
	if relation.Type != schema.Many2Many {
		return query.Preload(relation.Name).InnerJoins(relation.Name)
	}

	joinTable := relation.JoinTable.Table
	joinSQL := "LEFT JOIN ? ON "
	joinArgs := []any{clause.Table{Name: joinTable}}
	targetSQL := "LEFT JOIN ? ON "
	targetArgs := []any{clause.Table{Name: relation.FieldSchema.Table, Alias: relation.Name}}
	var joinPredicates, targetPredicates []string
	for _, ref := range relation.References {
		if ref.OwnPrimaryKey {
			joinPredicates = append(joinPredicates, "? = ?")
			joinArgs = append(joinArgs,
				clause.Column{Table: joinTable, Name: ref.ForeignKey.DBName},
				clause.Column{Table: query.Statement.Table, Name: ref.PrimaryKey.DBName},
			)
		} else {
			targetPredicates = append(targetPredicates, "? = ?")
			targetArgs = append(targetArgs,
				clause.Column{Table: relation.Name, Name: ref.PrimaryKey.DBName},
				clause.Column{Table: joinTable, Name: ref.ForeignKey.DBName},
			)
		}
	}
	joinSQL += strings.Join(joinPredicates, " AND ")
	targetSQL += strings.Join(targetPredicates, " AND ")
	return query.Joins(joinSQL, joinArgs...).Joins(targetSQL, targetArgs...)
}

// HydrateRelation preloads the relation when it has not been loaded yet.
func HydrateRelation(obj any, table string, relation any) error {
	rv := reflect.ValueOf(relation)
	if rv.Kind() == reflect.Ptr && !rv.IsNil() {
		return nil
	}
	db, err := providers.GetDB()
	if err != nil {
		return err
	}
	if err := requireCompletePrimaryKey(db, obj); err != nil {
		return err
	}
	name, err := validatedRelationName(db, obj, table)
	if err != nil {
		return err
	}
	return db.Preload(name).First(obj).Error
}

// HydrateManyToManyRelation preloads a many-to-many relation that has not
// been loaded yet. relation points to the slice field to fill: a nil slice
// means "not loaded", an allocated slice, even empty, is left alone.
func HydrateManyToManyRelation[T any](obj any, table string, relation *[]T) error {
	if relation == nil || *relation != nil {
		return nil
	}
	db, err := providers.GetDB()
	if err != nil {
		return err
	}
	if err := requireCompletePrimaryKey(db, obj); err != nil {
		return err
	}
	name, err := validatedRelationName(db, obj, table)
	if err != nil {
		return err
	}
	return db.Preload(name).First(obj).Error
}

func validatedRelationName(db *gorm.DB, obj any, name string) (string, error) {
	sch, err := parseSchema(db, obj)
	if err != nil {
		return "", err
	}
	relation, err := resolveRelation(sch, name)
	if err != nil {
		return "", err
	}
	return relation.Name, nil
}

// UpsertRelations upserts nested one-to-one relations of obj through their
// Get<Relation>/Set<Relation> methods, all in one transaction. Each relation
// must implement interfaces.Resource. A present but unknown public id is an
// error: an HTTP payload must never create an entity with a client-chosen
// identifier.
func UpsertRelations(c *gin.Context, obj any, relations []string) error {
	dw := CreateDataWriter(c)
	db, err := dw.connection()
	if err != nil {
		return err
	}
	return upsertRelations(db, dw, obj, relations)
}

func upsertRelations(db *gorm.DB, dw DataWriter, obj any, relations []string) error {
	if _, err := structPointer(obj); err != nil {
		return err
	}
	objRef := reflect.ValueOf(obj)

	type pending struct {
		name   string
		value  interfaces.Resource
		setter reflect.Value
	}
	var work []pending
	for _, relation := range relations {
		getter := objRef.MethodByName("Get" + relation)
		if !getter.IsValid() {
			return fmt.Errorf("missing getter for relation %s", relation)
		}
		setter := objRef.MethodByName("Set" + relation)
		if !setter.IsValid() {
			return fmt.Errorf("missing setter for relation %s", relation)
		}
		results := getter.Call(nil)
		if len(results) == 0 {
			return fmt.Errorf("getter for relation %s returned no value", relation)
		}
		val := results[0]
		if !val.IsValid() {
			continue
		}
		switch val.Kind() {
		case reflect.Ptr, reflect.Interface:
			if val.IsNil() {
				continue
			}
		default:
			return fmt.Errorf("getter for relation %s must return a pointer, got %s", relation, val.Kind())
		}
		resource, ok := capability[interfaces.Resource](val.Interface())
		if !ok {
			return fmt.Errorf("relation %s does not implement interfaces.Resource", relation)
		}
		if _, err := structPointer(resource); err != nil {
			return fmt.Errorf("relation %s: %w", relation, err)
		}
		if setter.Type().NumIn() != 1 || !reflect.TypeOf(resource).AssignableTo(setter.Type().In(0)) {
			return fmt.Errorf("setter for relation %s does not accept %T", relation, resource)
		}
		work = append(work, pending{relation, resource, setter})
	}
	if len(work) == 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		txWriter := dw.withDB(tx)
		for _, w := range work {
			if err := upsertRelation(tx, txWriter, w.value); err != nil {
				return fmt.Errorf("relation %s: %w", w.name, err)
			}
			outputs := w.setter.Call([]reflect.Value{reflect.ValueOf(w.value)})
			for _, out := range outputs {
				if err, ok := out.Interface().(error); ok && err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func upsertRelation(tx *gorm.DB, writer DataWriter, resource interfaces.Resource) error {
	publicId := resource.GetPublicId()
	if publicId == "" {
		return writer.Create(resource)
	}
	probe := reflect.New(reflect.ValueOf(resource).Elem().Type()).Interface()
	err := getOneByIdentity(tx, probe, resource.PublicIdColumn(), publicId)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("no record for public id %s", publicId)
	}
	if err != nil {
		return err
	}
	if err := copyPrimaryKeyFields(tx, probe, resource); err != nil {
		return err
	}
	return writer.Update(resource)
}

func filterExpression(column clause.Column, value any) (clause.Expression, bool, error) {
	value, alternative, err := unwrapCombinator(value)
	if err != nil {
		return nil, false, err
	}

	if operator, ok := value.(LikeOperator); ok {
		return clause.Like{Column: column, Value: "%" + operator.value + "%"}, alternative, nil
	}
	if _, ok := value.(clause.Expression); ok {
		return nil, false, errors.New("raw GORM expressions are not filter values")
	}
	if isSliceOrArray(value) {
		return clause.IN{Column: column, Values: sliceValues(value)}, alternative, nil
	}
	return clause.Eq{Column: column, Value: value}, alternative, nil
}

func unwrapCombinator(value any) (any, bool, error) {
	var inner any
	alternative := false
	switch operator := value.(type) {
	case OrOperator:
		inner, alternative = operator.value, true
	case AndOperator:
		inner = operator.value
	default:
		return value, false, nil
	}
	if isCombinator(inner) {
		return nil, false, errors.New("nested And and Or operators are invalid")
	}
	return inner, alternative, nil
}

func isCombinator(value any) bool {
	switch value.(type) {
	case OrOperator, AndOperator:
		return true
	}
	return false
}

func isSliceOrArray(value any) bool {
	if value == nil {
		return false
	}
	kind := reflect.TypeOf(value).Kind()
	return kind == reflect.Slice || kind == reflect.Array
}

func sliceValues(value any) []any {
	rv := reflect.ValueOf(value)
	values := make([]any, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		values[i] = rv.Index(i).Interface()
	}
	return values
}

func addOrdering(query *gorm.DB, value any, joined *[]string) (*gorm.DB, error) {
	var terms []Ordering
	switch typed := value.(type) {
	case Ordering:
		terms = []Ordering{typed}
	case []Ordering:
		terms = typed
	default:
		return nil, fmt.Errorf("%s must contain Ordering values constructed by OrderBy", orderByPattern)
	}
	for _, term := range terms {
		var column clause.Column
		var err error
		query, column, err = resolveQueryColumn(query, term.column, joined)
		if err != nil {
			return nil, fmt.Errorf("ordering: %w", err)
		}
		switch term.direction {
		case Ascending:
			query = query.Order(clause.OrderByColumn{Column: column})
		case Descending:
			query = query.Order(clause.OrderByColumn{Column: column, Desc: true})
		default:
			return nil, fmt.Errorf("invalid ordering direction %d", term.direction)
		}
	}
	return query, nil
}
