package services

import "golang.org/x/crypto/bcrypt"

func GetHashedPassword(pwd string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(pwd), 14)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}
