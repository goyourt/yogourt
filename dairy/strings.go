package dairy

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func ToTitle(str string) string {
	return cases.Title(language.Und).String(str)
}
