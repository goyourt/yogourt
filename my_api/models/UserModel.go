
package models

import (
	"gorm.io/gorm"
)
	
type User struct {
	id int 	`gorm:"primaryKey;not null;unique" json:"id"`
	name string 	`gorm:"not null" json:"name"`
	age int `json:"age"`
}

