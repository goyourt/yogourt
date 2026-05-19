package database

import "github.com/goyourt/yogourt/dairy"

func Like(s string) string {
	return likePatern + "%" + s + "%"
}

func Or(s any) any {
	if dairy.IsArray(s) {
		if len(s.([]string)) != 0 {
			s = append([]string{orPatern}, s.([]string)...)
		}
		return s
	}
	return orPatern + "%" + s.(string) + "%"
}
