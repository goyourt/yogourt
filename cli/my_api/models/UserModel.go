package models

import (
	"fmt"
)

type User struct {
	ID   string `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	Name string `gorm:"not null;unique" json:"Name"`
	Age  int    `gorm:"not null" json:"Age"`
}

// Champ Name
func (u *User) GetName() string {
	return u.Name
}

func (u *User) SetName(Name string) error {
	if len(Name) == 0 {
		return fmt.Errorf("Le nom ne peut pas être vide")
	}
	u.Name = Name
	return nil
}

// Champ Age
func (u *User) GetAge() int {
	return u.Age
}

func (u *User) SetAge(Age int) error {
	if Age < 0 {
		return fmt.Errorf("La valeur ne peut pas être négative")
	}
	u.Age = Age
	return nil
}
