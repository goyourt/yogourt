package database

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/dairy"
	"github.com/goyourt/yogourt/interfaces"
	"github.com/goyourt/yogourt/services/providers"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

const likePatern = "LIKE"
const orPatern = "OR"
const orderByPatern = "orderBy"

// buildQuery derives joins, conditions and ordering from values.
func buildQuery(db *gorm.DB, model any, values map[string]any) (*gorm.DB, error) {
	query := db.Model(model)
	if err := query.Statement.Parse(query.Statement.Model); err != nil {
		return nil, err
	}
	joined := []string{dairy.ToTitle(query.Statement.Table)}
	aliases := map[string]string{}
	for key, value := range values {
		if key == orderByPatern {
			query = addOrderBy(query, value)
			continue
		}
		if strings.Contains(key, ".") {
			prefix := strings.Split(key, ".")[0]
			relation := relationName(query.Statement.Schema, prefix)
			aliases[strings.ToLower(prefix)] = relation
			if !slices.Contains(joined, relation) {
				query = joinRelation(query, relation)
				joined = append(joined, relation)
			}
		}
		query = addConditionPatern(query, key, value, aliases)
	}
	return query.Preload(clause.Associations), nil
}

// relationName resolves a filter prefix to the exact GORM relation name:
// dairy.ToTitle lowercases interior capitals ("accessGroups" -> "Accessgroups"),
// so the schema keys are matched case-insensitively.
func relationName(sch *schema.Schema, prefix string) string {
	name := dairy.ToTitle(prefix)
	if sch == nil {
		return name
	}
	if _, ok := sch.Relationships.Relations[name]; ok {
		return name
	}
	for candidate := range sch.Relationships.Relations {
		if strings.EqualFold(candidate, name) {
			return candidate
		}
	}
	return name
}

// joinRelation joins one relation, aliased by its name so dotted filters can
// reference it. Many-to-many joins are built from the GORM relationship
// metadata (join table, references), everything else inner-joins directly.
func joinRelation(query *gorm.DB, name string) *gorm.DB {
	relation, ok := query.Statement.Schema.Relationships.Relations[name]
	if !ok || relation.Type != schema.Many2Many {
		return query.Preload(name).InnerJoins(name)
	}

	joinTable := relation.JoinTable.Table
	var onJoin, onTarget []string
	for _, ref := range relation.References {
		if ref.OwnPrimaryKey {
			onJoin = append(onJoin, fmt.Sprintf("%s.%s = %s.%s",
				joinTable, ref.ForeignKey.DBName, query.Statement.Table, ref.PrimaryKey.DBName))
		} else {
			onTarget = append(onTarget, fmt.Sprintf("%q.%s = %s.%s",
				name, ref.PrimaryKey.DBName, joinTable, ref.ForeignKey.DBName))
		}
	}
	return query.
		Joins(fmt.Sprintf("LEFT JOIN %s ON %s", joinTable, strings.Join(onJoin, " AND "))).
		Joins(fmt.Sprintf("LEFT JOIN %s %q ON %s", relation.FieldSchema.Table, name, strings.Join(onTarget, " AND ")))
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
	return db.Preload(table).First(obj).Error
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
	return db.Preload(table).First(obj).Error
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
	err := getOneBy(tx, probe, map[string]any{resource.PublicIdColumn(): publicId})
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

func addConditionPatern(query *gorm.DB, key string, value any, aliases map[string]string) *gorm.DB {
	isOr := false
	mainTable := query.Statement.Table
	if str, isStr := value.(string); isStr {
		if str, orFound := strings.CutPrefix(str, orPatern); orFound {
			str, prefixFound := strings.CutPrefix(str, "%")
			str, suffixFound := strings.CutSuffix(str, "%")
			if prefixFound && suffixFound {
				isOr = true
				value = str
			}
		}
	} else if arr, isArr := value.([]string); isArr && len(arr) > 0 && arr[0] == orPatern {
		isOr = true
		value = arr[1:]
	}

	if isOr {
		return query.Or(searchPatern(key, value, mainTable, aliases))
	}
	return query.Where(searchPatern(key, value, mainTable, aliases))
}

func searchPatern(key string, value any, mainTable string, aliases map[string]string) (string, any) {
	if str, isStr := value.(string); isStr {
		if str, likeFound := strings.CutPrefix(str, likePatern); likeFound {
			str, prefixFound := strings.CutPrefix(str, "%")
			str, suffixFound := strings.CutSuffix(str, "%")
			if prefixFound && suffixFound {
				return formatAlias(key, mainTable, aliases) + " LIKE ?", "%" + str + "%"
			}
		}
	}

	if dairy.IsArray(value) {
		return formatAlias(key, mainTable, aliases) + " IN ?", value
	}

	return formatAlias(key, mainTable, aliases) + "=?", value
}

func addOrderBy(query *gorm.DB, values any) *gorm.DB {
	switch v := values.(type) {
	case []string:
		for _, order := range v {
			query.Order(order)
		}
	case string:
		query.Order(v)
	}
	return query
}

func formatAlias(str string, maintable string, aliases map[string]string) string {
	if !strings.Contains(str, ".") {
		return "\"" + str + "\""
	}
	substr := strings.Split(str, ".")
	alias := substr[0]
	if alias != maintable {
		if name, ok := aliases[strings.ToLower(alias)]; ok {
			alias = name
		} else {
			alias = dairy.ToTitle(alias)
		}
	}
	return "\"" + alias + "\"." + substr[1]
}
