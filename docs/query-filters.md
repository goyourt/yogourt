# Filtres de requête

Les helpers de lecture valident les noms de colonnes et de relations contre le
schéma GORM du modèle. Une clé simple cible une colonne du modèle ; une clé
`relation.colonne` cible une colonne de cette relation. Une clé, relation ou
colonne inconnue retourne une erreur avant l’exécution SQL.

Les valeurs ordinaires restent littérales : une chaîne produit une égalité et
une slice produit un `IN`. Les opérateurs sont des valeurs typées construites
par le serveur :

```go
filters := map[string]any{
	"tenant_id": database.And(tenantID),
	"name":      database.Or(database.Like(search)),
	"email":     database.Or(database.Like(search)),
	"orderBy": []database.Ordering{
		database.OrderBy("created_at", database.Descending),
	},
}
```

`database.Like(text)` sélectionne explicitement `LIKE %text%`.
`database.Or(value)` place le filtre dans un groupe d’alternatives. Tous les
filtres ordinaires sont reliés avec `AND`, puis l’unique groupe `OR` est ajouté
avec `AND`. `database.And(value)` rend cette conjonction explicite : le filtre
se comporte comme une valeur ordinaire et reste hors du groupe. `And` et `Or`
acceptent un littéral, une slice ou un `Like`, mais ne s’imbriquent pas.
L’exemple correspond donc à :

```text
tenant_id = ? AND (name LIKE ? OR email LIKE ?)
```

L’ordre d’itération d’une map ne change pas cette portée. Les expressions GORM
brutes ne sont pas acceptées comme valeurs de filtre.

Le tri accepte uniquement un `database.Ordering` ou une slice de ces valeurs
sous la clé `orderBy`. La colonne est validée comme un filtre et la direction
doit être `database.Ascending` ou `database.Descending`. Une chaîne SQL de tri
n’est pas acceptée.

`database.GetOneByIdentity(obj, column, value)` sert aux identités publiques :
la colonne est validée sur le modèle et la valeur `string` utilise toujours une
égalité liée. `DataWriter.Upsert` exige au moins une colonne de correspondance
du modèle avec une valeur scalaire et littérale ; les relations, slices,
opérateurs de recherche et expressions GORM y sont refusés.
