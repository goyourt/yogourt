package database

import (
	"strings"

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

func formatAlias(str string) string {
	substr := strings.Split(str, ".")
	return "\"" + toTitle(substr[0]) + "\"." + substr[1]
}

func toTitle(str string) string {
	return cases.Title(language.Und).String(str)
}
