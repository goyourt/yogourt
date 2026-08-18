# Autorisation

Yogourt v2 intègre un système d'autorisation hybride, activable à la demande :

```text
permission RBAC (rôles → permissions)
→ restrictions ABAC éventuelles sur la ressource
→ décision autoriser ou refuser
```

Principes :

- **opt-in** : sans authorizer configuré, le framework se comporte exactement comme avant ;
- **refus par défaut** : pas de sujet → 401, pas de permission → 403, erreur technique → jamais une autorisation ;
- **fail-fast** : avec un authorizer, toute route non déclarée empêche le serveur de démarrer, avec un rapport exhaustif ;
- les permissions ne sont **jamais** placées dans le JWT : une révocation est visible dès la requête suivante ;
- aucun bypass implicite pour un rôle `admin`.

Tous les exemples de cette page forment une application complète qui a été compilée et testée telle quelle.

## Démarrage rapide

### 1. Arborescence

```text
.
├── api/
│   ├── health/route.go
│   ├── articles/route.go
│   └── articles/id_/route.go
├── configs/yogourt.yaml
├── middleware/middleware.go
├── .yogourt/
└── main.go
```

### 2. Construire et activer le moteur

Le démarrage rapide utilise le provider mémoire ; pour la production, voir [Store PostgreSQL](#store-postgresql).

```go
// main.go
package main

import (
	"context"
	"fmt"

	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/authorization/memory"
	"github.com/goyourt/yogourt/routing"
)

const (
	aliceID = "11111111-1111-1111-1111-111111111111" // editor, propriétaire
	bobID   = "22222222-2222-2222-2222-222222222222" // reader
)

func main() {
	// Provider de grants en mémoire (un store SQL arrivera plus tard).
	provider := memory.NewProvider()
	must(provider.CreateRole("reader"))
	must(provider.GrantPermissions("reader", "article.read"))
	must(provider.CreateRole("editor"))
	must(provider.GrantPermissions("editor", "article.read", "article.create", "article.update"))

	// Bindings sujet + scope → rôles. Sans multi-tenant, tout vit dans ScopeGlobal.
	must(provider.BindRoles(aliceID, authorization.ScopeGlobal, "editor"))
	must(provider.BindRoles(bobID, authorization.ScopeGlobal, "reader"))

	// Le moteur est construit une fois, avec toutes ses options, puis gelé.
	engine := authorization.NewEngine(
		authorization.WithProvider(provider),
		authorization.WithRestriction("article.update", ownsResource),
		authorization.WithKnownPermissions("article.read", "article.create", "article.update"),
	)

	routing.Initialize("api", routing.WithAuthorizer(engine))
}

// Restriction ABAC : seul le propriétaire modifie sa ressource.
func ownsResource(_ context.Context, in authorization.PolicyInput) (bool, error) {
	owned, ok := in.Resource.(interface{ OwnerID() string })
	if !ok {
		return false, fmt.Errorf("resource %T does not expose OwnerID()", in.Resource)
	}
	return owned.OwnerID() == in.Subject.ID, nil
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
```

Sans `routing.WithAuthorizer(engine)`, rien ne change : aucune déclaration exigée, un symbole `Permissions` présent est simplement ignoré avec un warning.

### 3. Déclarer les permissions des routes

Les permissions se déclarent **par route, pas par fichier** : `var Permissions map[string]string` (méthode HTTP → permission, ou `authorization.Public` pour une exemption explicite). L'URL venant du dossier, plusieurs fichiers peuvent servir la même route — dans ce cas **un seul** d'entre eux déclare la map, qui couvre toutes les méthodes du dossier :

```go
// api/users/users.go — déclare les permissions de TOUTE la route /api/users
var Permissions = map[string]string{
	"GET":  "user.read",  // exporté par users.go
	"POST": "user.create", // exporté par test.go, même dossier
}
```

Deux fichiers déclarant `Permissions` pour la même route refusent le démarrage.

```go
// api/health/route.go — route publique
package main

import (
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/routing"
)

var Permissions = map[string]string{
	"GET": authorization.Public,
}

func GET(c *gin.Context) {
	routing.RespondSuccess(c, gin.H{"status": "ok"})
}

func main() {}
```

```go
// api/articles/route.go — une permission par méthode
package main

import (
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/routing"
)

var Permissions = map[string]string{
	"GET":  "article.read",
	"POST": "article.create",
}

func GET(c *gin.Context) {
	routing.RespondSuccess(c, []gin.H{{"id": 1, "title": "Hello"}})
}

func POST(c *gin.Context) {
	routing.RespondCreated(c, gin.H{"id": 2})
}

func main() {}
```

Le middleware RBAC est inséré par le framework en dernière position avant le handler, après les middlewares hérités ; il n'est pas inséré pour les méthodes `Public`.

### 4. Attacher un sujet

En production, `services.Authenticate` attache automatiquement le sujet après validation du JWT (voir [Intégration avec l'authentification](#intégration-avec-lauthentification)). Pour tester sans base de données, un middleware de développement peut lire un header :

```go
// middleware/middleware.go — RÉSERVÉ AU DÉVELOPPEMENT
package main

import (
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/authorization"
)

var Callbacks = map[string]func(*gin.Context){
	"/": attachDebugSubject,
}

func attachDebugSubject(c *gin.Context) {
	if id := c.GetHeader("X-Debug-Subject"); id != "" {
		subject := authorization.Subject{ID: id}
		c.Request = c.Request.WithContext(authorization.WithSubject(c.Request.Context(), subject))
	}
}

func main() {}
```

### 5. Compiler et lancer

```sh
mkdir -p .yogourt/middleware .yogourt/api/health .yogourt/api/articles/id_
go build -buildmode=plugin -o .yogourt/middleware/middleware.go.so ./middleware/middleware.go
go build -buildmode=plugin -o .yogourt/api/health/route.go.so ./api/health/route.go
go build -buildmode=plugin -o .yogourt/api/articles/route.go.so ./api/articles/route.go
go build -buildmode=plugin -o .yogourt/api/articles/id_/route.go.so ./api/articles/id_/route.go
go run .
```

### 6. Tester

```sh
ALICE="11111111-1111-1111-1111-111111111111"
BOB="22222222-2222-2222-2222-222222222222"

curl http://localhost:8080/api/health
# {"data":{"status":"ok"}}                       → 200 (Public)

curl http://localhost:8080/api/articles
# {"error":"Unauthorized"}                       → 401 (aucun sujet)

curl -H "X-Debug-Subject: $BOB" http://localhost:8080/api/articles
# {"data":[{"id":1,"title":"Hello"}]}            → 200 (reader a article.read)

curl -X POST -H "X-Debug-Subject: $BOB" http://localhost:8080/api/articles
# {"error":"Forbidden"}                          → 403 (reader n'a pas article.create)

curl -X POST -H "X-Debug-Subject: $ALICE" http://localhost:8080/api/articles
# {"data":{"id":2}}                              → 201 (editor a article.create)
```

## Validation au démarrage

Avec un authorizer configuré, le boot échoue si une route est mal déclarée, en listant **toutes** les violations en une seule fois :

```text
Error loading handlers: route permission validation failed (1 violation(s)):
articles/route.go: Permissions: missing required symbol: var Permissions map[string]string
```

Sont refusés au démarrage : symbole `Permissions` manquant pour un dossier qui exporte des handlers, déclaration dans **plusieurs** fichiers du même dossier, méthode exportée sans entrée dans la map, entrée sans handler exporté (faute de frappe), permission vide, et — si `WithKnownPermissions` est utilisé — toute permission inconnue. Un dossier sans aucun handler exporté n'a pas besoin de symbole.

## Restrictions ABAC dans les handlers

Le middleware ne fait que le contrôle RBAC : la ressource n'est pas encore chargée. Les restrictions s'évaluent **dans le handler**, après chargement et avant toute mutation, via `c.Authorize` :

```go
// api/articles/id_/route.go
package main

import (
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt"
	"github.com/goyourt/yogourt/routing"
)

var Permissions = map[string]string{
	"PATCH": "article.update",
}

type Params struct {
	ID int `param:"id"`
}

type Article struct {
	ID    int
	Owner string
}

// OwnerID est consommé par la restriction ownership du main.go.
func (a Article) OwnerID() string { return a.Owner }

func PATCH(c *yogourt.Context, params Params) {
	// 1. Charger la ressource (ici factice ; en vrai, depuis la base).
	article := Article{ID: params.ID, Owner: "11111111-1111-1111-1111-111111111111"}

	// 2. Contrôle ABAC AVANT toute mutation. Authorize réutilise la
	//    permission déclarée pour la méthode courante ("article.update").
	if !c.Authorize(article) {
		return // statut déjà écrit (403/404/503), requête interrompue
	}

	// 3. Mutation seulement ici.
	routing.RespondSuccess(c.Gin(), gin.H{"updated": params.ID})
}

func main() {}
```

```sh
curl -X PATCH -H "X-Debug-Subject: $ALICE" http://localhost:8080/api/articles/7
# {"data":{"updated":7}}    → 200 (editor ET propriétaire)

curl -X PATCH -H "X-Debug-Subject: $BOB" http://localhost:8080/api/articles/7
# {"error":"Forbidden"}     → 403 dès le middleware (reader n'a pas article.update)

# Un second editor, non propriétaire, passe le RBAC mais échoue sur l'ABAC :
curl -X PATCH -H "X-Debug-Subject: 44444444-4444-4444-4444-444444444444" http://localhost:8080/api/articles/7
# {"error":"Forbidden"}     → 403 dans le handler (restriction ownership)
```

Les autres helpers du contexte :

| Méthode | Effet HTTP | Usage |
| --- | --- | --- |
| `c.HasPermission("article.read")` | aucun | question RBAC seule |
| `c.Can("article.update", article)` | aucun | décision complète, pour du contenu conditionnel |
| `c.Authorize(article)` | statut + abort si refus | permission déclarée de la route |
| `c.AuthorizeAction("autre.action", article)` | statut + abort si refus | action explicite |

Sur une méthode `Public`, `c.Authorize` sans action est indéfini (aucune permission déclarée) et répond 500 : utilisez `c.Can` ou `c.AuthorizeAction`.

## Statuts HTTP

| Situation | Statut |
| --- | ---: |
| Aucun sujet attaché | `401` |
| Permission RBAC absente ou restriction ABAC fausse | `403` (ou `404` si masquage) |
| Moteur absent ou mal configuré | `500` |
| Provider ou restriction en erreur technique | `503` |

Les raisons internes (`missing_permission`, `policy_error`…) ne transitent jamais dans les corps de réponse : messages génériques uniquement.

**Masquage 404** : pour cacher l'existence d'une ressource aux appelants non autorisés, déclarez l'action à la construction du moteur — le masquage s'applique alors de façon identique au middleware RBAC et à `c.Authorize` :

```go
authorization.NewEngine(
	authorization.WithProvider(provider),
	authorization.WithNotFoundOnDeny("document.read"),
)
```

## Scopes et multi-tenant

Un scope est un identifiant opaque (tenant, organisation). La résolution des grants fait toujours l'union `{scope demandé, ScopeGlobal}` : un binding sur `authorization.ScopeGlobal` est le chemin légitime d'un admin global.

```go
// Binding limité à un tenant :
provider.BindRoles(userID, "tenant-42", "editor")

// Fixer le scope de la requête (par ex. dans un middleware) :
c.Request = c.Request.WithContext(authorization.WithScope(c.Request.Context(), "tenant-42"))
```

Sans appel à `WithScope`, tout se résout dans `ScopeGlobal` — une application mono-tenant ignore simplement les scopes.

> [!WARNING]
> Ne construisez jamais un scope à partir d'une entrée utilisateur brute. La valeur sentinelle de `ScopeGlobal` (`@global`) est hors d'atteinte des identifiants habituels, mais la validation des noms de tenants reste la responsabilité de l'application.

## Store PostgreSQL

`authorization/gormstore` fournit le provider SQL de production. Les tables (`authz_permissions`, `authz_roles`, `authz_role_permissions`, `authz_role_bindings`) sont créées par une migration SQL versionnée, embarquée dans le package et appliquée **explicitement** — jamais d'`AutoMigrate` :

```go
db := providers.GetDB() // ou toute connexion GORM
if err := gormstore.Migrate(ctx, db); err != nil {
	log.Fatal(err)
}
store := gormstore.New(db)

engine := authorization.NewEngine(authorization.WithProvider(store))
routing.Initialize("api", routing.WithAuthorizer(engine))
```

**Aucune permission ne s'insère à la main.** Au démarrage, le framework enregistre automatiquement dans le store toutes les permissions déclarées par les routes (synchronisation additive : rien n'est jamais supprimé), et `GrantPermissions` enregistre de lui-même une permission encore inconnue. Il ne reste à administrer que les rôles et les bindings :

```go
store.CreateRole(ctx, "editor") // idempotent
store.GrantPermissions(ctx, "editor", "article.read", "article.update")
store.BindRoles(ctx, aliceID, authorization.ScopeGlobal, "editor")
```

La résolution des grants tient en une seule requête indexée par `(sujet, scope)`. Toutes les opérations acceptent un `context.Context`, sont transactionnelles, idempotentes, et retournent leurs erreurs SQL. Les tests d'intégration du package se lancent avec `YOGOURT_TEST_DSN` (ils sont ignorés sinon).

## Intégration avec l'authentification

Après un `services.Authenticate` réussi, le sujet est attaché automatiquement : identité = claim `uuid` du JWT, avec l'ID SQL interne dans `Attributes["internal_id"]`. Un modèle utilisateur peut contrôler son propre sujet en implémentant :

```go
func (u *User) AuthorizationSubject() authorization.Subject {
	return authorization.Subject{ID: *u.Uuid, Attributes: map[string]any{"role_hint": u.Role}}
}
```

Une application avec sa propre authentification appelle `services.AttachSubject(c, subject)` après validation.

Le Lot 0 du chantier a durci cette chaîne : algorithme JWT restreint à HS256, claim `uuid` validé, secret de 32 octets minimum vérifié au démarrage, et une panne de base pendant l'authentification répond 503 — jamais 401.

## Limites actuelles

- pas de commande CLI `yogourt routes` ni `permissions sync` ;
- pas de cache des grants entre requêtes : un `Resolve` (deux si scope ≠ global) par contrôle ;
- l'ADR consolidant les décisions de conception reste à rédiger ;
- le chargement des plugins impose toujours les contraintes du package `plugin` de Go (voir [Routage](routing.md)).
