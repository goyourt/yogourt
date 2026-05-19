package database

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/dairy"
	"github.com/goyourt/yogourt/interfaces"
	"github.com/goyourt/yogourt/services/providers"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const likePatern = "LIKE"
const orPatern = "OR"
const orderByPatern = "orderBy"

func JoinTables[T interfaces.BaseInterface](values map[string]any, objType *T) *gorm.DB {
	query := providers.GetDB().Model(*objType)
	for key, value := range values {
		if key == orderByPatern {
			query = addOrderBy(query, value)
			continue
		}
		if strings.Contains(key, ".") {
			model := toTitle(strings.Split(key, ".")[0])
			if tableName, isManyToMany := getMany2ManyTableName(*objType, model); isManyToMany {
				query = joinManyToMany(query, model, tableName)
			} else {
				query = query.Preload(model).InnerJoins(model)
			}
		}

		query = addConditionPatern(query, key, value)
	}

	return query.Preload(clause.Associations)
}

func HydrateRelation(obj interfaces.BaseInterface, table string, relation interfaces.BaseInterface, relationId int) {
	if relationId == 0 || !reflect.ValueOf(relation).IsNil() {
		return
	}

	providers.GetDB().Preload(table).Find(obj, obj.GetID())
}

func HydrateManyToManyRelation[T interfaces.BaseInterface](obj interfaces.BaseInterface, table string, relation *[]T) {
	if !reflect.ValueOf(relation).IsNil() {
		return
	}
	providers.GetDB().Preload(table).Find(obj, obj.GetID())
}

func UpsertRelations(c *gin.Context, obj interfaces.BaseInterface, relations []string) error {
	// TODO : upsert with many to many relations
	objRef := reflect.ValueOf(obj)
	dw := CreateDataWriter(c)

	for _, relation := range relations {
		relationGetter := objRef.MethodByName("Get" + relation)
		if !relationGetter.IsValid() {
			return fmt.Errorf("Missing getter for relation %s", relation)
		}

		results := relationGetter.Call(nil)
		if len(results) == 0 {
			return fmt.Errorf("Getter for relation %s returned no value", relation)
		}
		val := results[0]
		if !val.IsValid() || val.IsNil() {
			continue
		}

		relationInterface, ok := val.Interface().(interfaces.BaseInterface)
		if !ok {
			return fmt.Errorf("Getter for relation %s doesn't return BaseInterface", relation)
		}

		relationUuid := relationInterface.GetUuid()
		if relationUuid == "" {
			if err := dw.Create(relationInterface); err != nil {
				return fmt.Errorf("Unable to create relation %s: %w", relation, err)
			}
		} else {
			if err := dw.Update(relationInterface); err != nil {
				return fmt.Errorf("Unable to update relation %s: %w", relation, err)
			}

			if relationInterface.GetID() == 0 {
				return fmt.Errorf("Unable to update relation %s: related object not found (uuid: %s)", relation, relationUuid)
			}
		}

		relationSetter := objRef.MethodByName("Set" + relation)
		if !relationSetter.IsValid() {
			return fmt.Errorf("Missing setter for relation %s", relation)
		}

		errors := relationSetter.Call([]reflect.Value{reflect.ValueOf(relationInterface)})
		if len(errors) > 0 {
			if err, ok := errors[0].Interface().(error); ok && err != nil {
				return err
			}
		}
	}

	return nil
}

func addConditionPatern(query *gorm.DB, key string, value any) *gorm.DB {
	isOr := false
	if str, isStr := value.(string); isStr {
		if str, orFound := strings.CutPrefix(str, orPatern); orFound {
			str, prefixFound := strings.CutPrefix(str, "%")
			str, suffixFound := strings.CutSuffix(str, "%")
			if prefixFound && suffixFound {
				isOr = true
				value = str
			}
		}
	} else if dairy.IsArray(value) {
		if len(value.([]string)) > 0 {
			if value.([]string)[0] == orPatern {
				isOr = true
				value = value.([]string)[1:]
			}
		}
	}

	if isOr {
		return query.Or(searchPatern(key, value))
	}
	return query.Where(searchPatern(key, value))
}

func searchPatern(key string, value any) (string, any) {
	if str, isStr := value.(string); isStr {
		if str, likeFound := strings.CutPrefix(str, likePatern); likeFound {
			str, prefixFound := strings.CutPrefix(str, "%")
			str, suffixFound := strings.CutSuffix(str, "%")
			if prefixFound && suffixFound {
				return formatAlias(key) + " LIKE ?", "%" + str + "%"
			}
		}
	}

	if dairy.IsArray(value) {
		return formatAlias(key) + " IN ?", value
	}

	return formatAlias(key) + "=?", value
}

func addOrderBy(query *gorm.DB, values any) *gorm.DB {
	vType := reflect.TypeOf(values)
	if vType == nil {
		return query
	}

	if dairy.IsArray(values) {
		for _, order := range values.([]string) {
			query.Order(order)
		}
	} else {
		query.Order(values)
	}
	return query
}

func formatAlias(str string) string {
	if !strings.Contains(str, ".") {
		return "\"" + str + "\""
	}
	substr := strings.Split(str, ".")
	return "\"" + toTitle(substr[0]) + "\"." + substr[1]
}

func toTitle(str string) string {
	return cases.Title(language.Und).String(str)
}

func resetId(obj interfaces.BaseInterface) {
	value := reflect.ValueOf(obj)
	field := value.Elem().FieldByName("ID")
	field.Set(reflect.Zero(field.Type()))
}

func getMany2ManyTableName(obj interfaces.BaseInterface, fieldName string) (string, bool) {
	t := reflect.TypeOf(obj)
	if t == nil {
		return "", false
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return "", false
	}

	f, ok := t.FieldByName(fieldName)
	if !ok {
		return "", false
	}

	g := f.Tag.Get("gorm")
	if g == "" {
		return "", false
	}

	for _, part := range strings.Split(g, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "many2many:") {
			val := strings.TrimPrefix(part, "many2many:")
			val = strings.Trim(val, `"`)
			return val, true
		}
	}
	return "", false
}

func joinManyToMany(query *gorm.DB, model string, tableName string) *gorm.DB {
	tables := strings.Split(tableName, "_")
	from, to := tables[0], tables[1]

	return query.
		Joins(fmt.Sprintf("LEFT JOIN %s ON %s.id = %s.%s_id", tableName, asTableName(from), tableName, from)).
		Joins(fmt.Sprintf("LEFT JOIN %s \"%s\" ON \"%s\".id = %s.%s_id", asTableName(to), model, model, tableName, to))
}

func asTableName(table string) string {
	if strings.HasSuffix(table, "s") {
		return table
	}
	return table + "s"
}
