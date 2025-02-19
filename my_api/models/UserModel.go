package models

// Modèle pour User
type User struct {
	ID int `json:"ID"`	// PRIMARY_KEY
	Name string `json:"Name"`	// NOT NULL
	Age int `json:"Age"`
}

