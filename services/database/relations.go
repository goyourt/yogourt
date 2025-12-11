package database

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/interfaces"
	"github.com/goyourt/yogourt/services/providers"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gorm.io/gorm"
)

func JoinTables(values map[string]any) *gorm.DB {
	query := providers.GetDB()
	for key, value := range values {
		if strings.Contains(key, ".") {
			model := toTitle(strings.Split(key, ".")[0])
			query = query.Joins(model)
		}

		query = query.Where(formatAlias(key)+"=?", value)
	}

	return query
}

func HydrateRelation(obj interfaces.BaseInterface, table string, relation interfaces.BaseInterface, relationId int) {
	if relationId == 0 || !reflect.ValueOf(relation).IsNil() {
		return
	}

	providers.GetDB().Preload(table).Find(obj, obj.GetID())
}

func UpsertRelations(c *gin.Context, obj interfaces.BaseInterface, relations []string) error {
	objRef := reflect.ValueOf(obj)
	dw := CreateDataWriter(c)

	for _, relation := range relations {
		relationGetter := objRef.MethodByName("Get" + relation)
		if !relationGetter.IsValid() {
			return fmt.Errorf("Missing getter for relation %s", relation)
		}

		relationInterface, ok := relationGetter.Call(nil)[0].Interface().(interfaces.BaseInterface)
		if !ok {
			return fmt.Errorf("Getter for relation %s doesn't return BaseInterface", relation)
		}
		if relationInterface == nil {
			return nil
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
				return fmt.Errorf("Unable to update relation %s: related object not found (uuid: %w)", relation, relationUuid)
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
