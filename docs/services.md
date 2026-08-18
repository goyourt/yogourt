# Services

Ce guide présente les API publiques observées sur la préversion v2. Les signatures peuvent encore évoluer avant le tag stable.

## Fournisseurs

Les fournisseurs sont chargés à la première utilisation et conservés dans des singletons :

~~~go
cfg := providers.GetMainConfig()
db := providers.GetDB()
cache := providers.GetCache()
files := providers.GetFileConfig()
~~~

Conséquences :

- la configuration n’est pas rechargée pendant l’exécution ;
- une erreur de fichier de configuration provoque un panic ;
- une erreur d’ouverture PostgreSQL termine le processus ;
- les dépendances globales rendent les tests isolés plus difficiles.

Voir le [guide de configuration](configuration.md) pour les chemins et champs exacts.

## Modèles

Un modèle persistant peut embarquer <code>interfaces.Base</code> :

~~~go
package models

import "github.com/goyourt/yogourt/interfaces"

type User struct {
	interfaces.Base
	Email string `gorm:"uniqueIndex;not null" json:"email"`
	Name  string `json:"name"`
}
~~~

<code>*User</code> implémente alors <code>interfaces.BaseInterface</code>. La structure de base fournit :

- un ID entier interne ;
- un UUID PostgreSQL généré par <code>gen_random_uuid()</code> ;
- les dates de création, modification et suppression ;
- les IDs d’audit <code>CreatedById</code>, <code>UpdatedById</code> et <code>DeletedById</code> ;
- la suppression douce GORM via <code>gorm.DeletedAt</code>.

Les champs ID, UUID et audit sont stockés via des pointeurs. Utilisez leurs getters et setters lorsque la valeur peut ne pas être initialisée.

## Lectures en base

### Un objet

~~~go
user := &models.User{}
err := database.GetOneBy(user, map[string]any{
	"uuid": userUUID,
})
if errors.Is(err, gorm.ErrRecordNotFound) {
	// Introuvable.
} else if err != nil {
	// Panne SQL/réseau : distincte d'un « non trouvé ».
}
~~~

### Une collection

Les méthodes de <code>interfaces.Base</code> ont des receivers pointeurs. Utilisez donc une slice de pointeurs :

~~~go
var users []*models.User

err := database.GetAll(&users, map[string]any{
	"name":   database.Like("alb"),
	"status": []string{"active", "invited"},
})
~~~

Pagination :

~~~go
var users []*models.User
err := database.GetAllPaginated(&users, filters, 1, 25)
~~~

Une page ou une taille inférieure à 1 désactive la pagination.

### Filtres et relations

- une clé simple cible une colonne du modèle principal ;
- une clé comme <code>profile.city</code> joint et précharge la relation <code>Profile</code> ;
- une slice produit une condition <code>IN</code> ;
- <code>database.Like("text")</code> produit une recherche <code>LIKE %text%</code> ;
- <code>database.Or(value)</code> demande une condition OR ;
- la clé spéciale <code>orderBy</code> est expérimentale.

N’utilisez jamais directement des noms de colonne, relation ou ordre provenant d’une requête HTTP. Ces valeurs participent à la construction SQL et doivent venir d’une liste applicative fermée.

Les combinaisons complexes avec <code>Or</code> sont également fragiles, car une map Go n’a pas d’ordre stable. Préférez <code>database.SearchQuery</code> puis l’API GORM pour les requêtes avancées, en contrôlant explicitement les entrées :

~~~go
var users []*models.User

err := database.
	SearchQuery(map[string]any{"status": "active"}, &users, 1, 25).
	Where("created_at >= ?", since).
	Find(&users).
	Error
~~~

<code>GetOneBy</code>, <code>GetAll</code> et <code>GetAllPaginated</code> retournent désormais l’erreur GORM (<code>gorm.ErrRecordNotFound</code> inclus pour <code>GetOneBy</code>) : une panne SQL n’est plus confondue avec un résultat vide. <code>SearchQuery</code> reste la porte d’entrée pour les requêtes GORM avancées.

## Écritures en base

Créez un writer à partir du contexte Gin pour renseigner les champs d’audit :

~~~go
writer := database.CreateDataWriter(c)

user := &models.User{
	Email: "alban@example.com",
	Name:  "Alban",
}

if err := writer.Create(user); err != nil {
	// Répondre à l’erreur.
}
~~~

Si le contexte est nil ou ne contient aucun utilisateur authentifié, l’écriture reste possible mais les champs d’audit utilisateur ne sont pas renseignés.

API disponible :

~~~go
err := writer.Create(user)
err = writer.Update(user)
err = writer.Upsert(user, map[string]any{"uuid": user.GetUuid()})
err = writer.Delete(user)
err = database.HardDelete(user)
~~~

- <code>Create</code> remet l’ID à zéro et crée la ligne ;
- <code>Update</code> cible la ligne par UUID, puis recharge l’objet ;
- <code>Upsert</code> cherche d’abord avec les filtres fournis, puis crée ou met à jour ;
- <code>Delete</code> effectue une suppression douce et renseigne l’audit ;
- <code>HardDelete</code> effectue une suppression définitive.

Comme <code>GetOneBy</code> ne remonte pas les erreurs SQL, <code>Upsert</code> peut tenter un <code>Create</code> après un échec de lecture. Ne l’utilisez pas pour un chemin critique sans contrôle supplémentaire.

Ces méthodes ne démarrent pas automatiquement une transaction commune. Dans un callback <code>Transaction</code>, utilisez directement le <code>*gorm.DB</code> reçu :

~~~go
err := providers.GetDB().Transaction(func(tx *gorm.DB) error {
	return tx.Create(user).Error
})
~~~

<code>DataWriter</code> continue d’appeler le provider global ; ses méthodes ne sont pas automatiquement rattachées à <code>tx</code>.

## Relations

Les helpers disponibles sont :

~~~go
err := database.HydrateRelation(user, "Profile", user.Profile, user.ProfileID)
err = database.UpsertRelations(c, user, []string{"Profile"})
~~~

<code>HydrateRelation</code> et <code>HydrateManyToManyRelation</code> retournent l’erreur GORM ; les sites d’appel qui l’ignorent continuent de compiler.

<code>UpsertRelations</code> recherche des méthodes <code>GetRelation</code> et <code>SetRelation</code> par réflexion. Il ne prend pas encore en charge l’upsert many-to-many.

<code>HydrateManyToManyRelation</code> est public, mais sa garde actuelle retourne immédiatement pour un pointeur de slice non nil ; ne vous appuyez pas sur ce helper avant sa correction.

## Authentification

### Middleware

<code>services.Authenticate</code> valide un token Bearer, charge l’utilisateur par UUID et le place dans le contexte.

~~~go
func authenticate(c *gin.Context) {
	services.Authenticate(c, &models.User{})
	if c.IsAborted() {
		return
	}
	c.Next()
}
~~~

Le header doit avoir exactement la forme :

~~~text
Authorization: Bearer <token>
~~~

Récupération de l’utilisateur :

~~~go
current := providers.GetCurrentUser(c)
user, ok := current.(*models.User)
if !ok {
	// Aucun utilisateur du type attendu.
}
~~~

### Tokens

~~~go
token, err := services.CreateToken(user.GetUuid())
~~~

Les tokens créés utilisent HS256 et contiennent :

- <code>uuid</code> ;
- <code>exp</code>, calculé à partir de <code>security.token_expires</code> en minutes.

Les helpers de plus bas niveau sont également publics :

~~~go
raw, err := services.GetRequestToken(c)
parsed, err := services.ValidToken(raw)
uuid, err := services.GetClaim(parsed, "uuid")
~~~

Limites de sécurité actuelles :

- la robustesse et la présence du secret ne sont pas validées ;
- <code>ValidToken</code> ne fixe pas explicitement la liste des algorithmes acceptés ;
- aucun issuer ou audience n’est vérifié ;
- l’expiration n’est pas explicitement exigée pour les tokens qui ne sont pas créés par Yogourt.

Pour un usage sensible, encapsulez ces helpers avec une politique JWT stricte ou attendez leur durcissement avant la v2 stable.

## Mots de passe

~~~go
if !services.IsPasswordValid(password) {
	// Politique définie dans configs/yogourt.yaml.
}

hash, err := services.GetHashedPassword(password)
~~~

<code>GetHashedPassword</code> utilise bcrypt. Le coût par défaut est 12 lorsque <code>security.hash_cost</code> vaut 0.

Suivi Redis des échecs :

~~~go
err := services.SavePasswordFailure(username)
count, err := services.GetPasswordFailureCount(username)
~~~

Le compteur regarde une fenêtre de 24 heures, mais les entrées anciennes ne sont actuellement ni supprimées ni associées à un TTL. Les appels utilisent aussi <code>context.Background()</code> et plusieurs échecs dans la même seconde peuvent partager le même membre Redis.

## Fichiers

La configuration est décrite dans <code>configs/files.yaml</code>. Lecture d’un upload multipart :

~~~go
uploaded, err := services.ReadUploadedFile(c, "avatar", "avatars")
if err != nil {
	routing.RespondAndAbort(c, 422, err.Error())
	return
}
~~~

Le helper :

- applique la taille maximale configurée ;
- refuse un fichier vide ;
- normalise le nom fourni par le client ;
- crée le dossier configuré ;
- place le contenu en mémoire dans un <code>interfaces.File</code>.

L’API actuelle n’offre pas de transaction fiable entre les métadonnées et le disque. Si elle doit être utilisée malgré cette limite, le chemin calculé par <code>SaveFile</code> doit être repersisté explicitement :

~~~go
writer := database.CreateDataWriter(c)
if err := writer.Create(uploaded); err != nil {
	routing.RespondAndAbort(c, 500, "unable to persist file metadata")
	return
}

services.SaveFile(uploaded)
if err := writer.Update(uploaded); err != nil {
	routing.RespondAndAbort(c, 500, "unable to update file metadata")
	return
}
~~~

Cette séquence n’est pas atomique et <code>SaveFile</code> ne permet pas de savoir si l’écriture a réussi. Pour un flux critique, fournissez une couche de stockage applicative qui retourne les erreurs avant de valider les métadonnées.

Autres fonctions :

~~~go
content, err := services.ReadFile(file)
services.GenerateFile(path, content)
services.CreateFolder(path)
serialized, err := services.SerializeFile(multipartFile)
~~~

Limites actuelles :

- les uploads sont chargés entièrement en mémoire ;
- aucun MIME type ni extension autorisée n’est validé ;
- <code>SaveFile</code>, <code>GenerateFile</code> et <code>CreateFolder</code> ne retournent pas leurs erreurs ;
- l’extension perd son point avant d’être concaténée à l’UUID, ce qui produit actuellement un nom comme <code>uuidpng</code> ;
- appeler <code>SaveFile</code> avant l’attribution d’un UUID peut provoquer des collisions ;
- le stockage est local et n’offre pas d’écriture atomique.

Ce service ne doit pas être considéré comme un stockage de fichiers durci tant que ces points ne sont pas corrigés.

## URL de base

~~~go
baseURL := services.GetBaseUrl()
~~~

Si <code>server.host</code> est vide, le résultat est <code>http://localhost:&lt;port&gt;</code>. Sinon, la valeur du champ est retournée telle quelle. Comme le runtime interprète désormais ce champ comme une adresse d’écoute brute, <code>GetBaseUrl</code> peut donc retourner <code>127.0.0.1</code> sans schéma ni port ; encapsulez ce helper ou utilisez une configuration d’URL publique séparée jusqu’à l’alignement de son contrat.

## Limites transverses

- les providers globaux ne prennent pas de <code>context.Context</code> ;
- les échecs de configuration utilisent panic ou <code>log.Fatal</code> ;
- les comportements de sécurité, de stockage et de concurrence doivent être renforcés avant un usage critique.
