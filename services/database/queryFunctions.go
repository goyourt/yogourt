package database

func Like(s string) string {
	return likePatern + "%" + s + "%"
}
