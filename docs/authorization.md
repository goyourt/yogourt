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
- **convention** : la permission de chaque route est dérivée de son dossier et de sa méthode HTTP — rien à déclarer, et une route dont la permission n'est accordée à aucun rôle répond 403, jamais 200 ;
- les permissions ne sont **jamais** placées dans le JWT : une révocation est visible dès la requête suivante ;
- aucun bypass implicite pour un rôle `admin`.

Les exemples de cette page reprennent une application de test vérifiée de bout en bout : dérivation des permissions, surcharge, restrictions ABAC, masquage 404 et sous-ressource gouvernée par son parent. Les statuts HTTP annoncés dans les tableaux proviennent d'exécutions réelles.

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
	ctx := context.Background()

	// Provider de grants en mémoire (voir plus bas pour le store SQL, et
	// pour la gestion des rôles à l'exécution depuis une interface web).
	// Les noms de permissions suivent la convention : dossier + verbe.
	provider := memory.NewProvider()
	must(provider.CreateRole(ctx, "reader"))
	must(provider.GrantPermissions(ctx, "reader", "articles.read"))
	must(provider.CreateRole(ctx, "editor"))
	must(provider.GrantPermissions(ctx, "editor", "articles.read", "articles.create", "articles.update"))

	// Bindings sujet + scope → rôles. Sans multi-tenant, tout vit dans ScopeGlobal.
	must(provider.BindRoles(ctx, aliceID, authorization.ScopeGlobal, "editor"))
	must(provider.BindRoles(ctx, bobID, authorization.ScopeGlobal, "reader"))

	// Le moteur est construit une fois, avec toutes ses options, puis gelé.
	engine := authorization.NewEngine(
		authorization.WithProvider(provider),
		authorization.WithRestriction("articles.update", ownsResource),
		authorization.WithKnownPermissions("articles.read", "articles.create", "articles.update"),
	)

	routing.Initialize(routing.WithAuthorizer(engine))
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

### 3. Les permissions des routes (rien à déclarer)

Les permissions sont **dérivées par convention** du dossier et de la méthode HTTP — à la manière de Django, adapté au routage par fichiers. Aucune déclaration n'est nécessaire :

| Route | Méthode | Permission dérivée |
| --- | --- | --- |
| `/api/users` | `GET` | `users.read` |
| `/api/users` | `POST` | `users.create` |
| `/api/users/:id` | `PATCH` | `users.update` |
| `/api/users/:id` | `DELETE` | `users.delete` |
| `/api/users/:id/comments` | `GET` | `comments.read` |
| `/api/posts/:id/comments` | `GET` | `comments.read` |

La règle : **la ressource est le dernier segment statique** de la route sous le préfixe HTTP (les paramètres `:id` sont ignorés, et les segments du préfixe ne nomment jamais une ressource : sous `/v1`, `users` reste la ressource), et le verbe vient de la méthode — `GET` → `read`, `POST` → `create`, `PUT`/`PATCH` → `update`, `DELETE` → `delete`.

Un fichier de route n'a donc plus rien à déclarer :

```go
// api/users/route.go
package main

import (
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/routing"
)

// GET dérive users.read, POST dérive users.create.

func GET(c *gin.Context) {
	routing.RespondSuccess(c, []gin.H{{"id": 1, "name": "Alba"}})
}

func POST(c *gin.Context) {
	routing.RespondCreated(c, gin.H{"id": 2})
}

func main() {}
```

Deux routes servant la même ressource partagent la même permission — `/api/users/:id/comments` et `/api/posts/:id/comments` dérivent tous deux `comments.read`. C'est voulu : « lire des commentaires » est une seule capacité RBAC. Si la sensibilité diffère selon le contexte, surchargez ; si c'est un filtrage sur les données, utilisez une [restriction ABAC](#restrictions-abac-dans-les-handlers).

#### Surcharger la convention

`var Permissions map[string]string` reste disponible comme **surcharge**, et elle est **partielle par nature** : une méthode absente de la map garde sa permission dérivée.

```go
// api/health/route.go — route publique
package main

import (
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/routing"
)

var Permissions = map[string]string{
	"GET": authorization.Public, // exemption explicite
}

func GET(c *gin.Context) {
	routing.RespondSuccess(c, gin.H{"status": "ok"})
}

func main() {}
```

Une surcharge partielle ne liste que ce qui s'écarte de la convention :

```go
// api/articles/route.go
var Permissions = map[string]string{
	"POST": "articles.publish", // nom métier hors CRUD
	// GET et PATCH ne sont pas listés : ils dérivent
	// articles.read et articles.update.
}
```

Trois cas justifient une surcharge :

- **exempter** une méthode (`authorization.Public`) ;
- **nommer** autrement (verbe métier hors CRUD, ou distinguer deux contextes : `"GET": "profile_comments.read"`) ;
- couvrir les cas **non dérivables** : la route racine `/api` n'a aucun segment ressource, et une méthode HTTP hors du tableau ci-dessus n'a pas de verbe. Ces deux cas exigent une déclaration, sinon le démarrage échoue.

L'URL venant du dossier, plusieurs fichiers peuvent servir la même route : dans ce cas **un seul** déclare la map, qui vaut pour toutes les méthodes du dossier. Deux fichiers qui déclarent pour la même route refusent le démarrage.

#### La surface est loggée à chaque démarrage

Comme aucune déclaration n'est exigée, le framework affiche au boot la totalité de la surface d'autorisation, avec l'origine de chaque permission :

```text
authorization: GET /api -> @public (declared)
authorization: GET /api/profiles/:id -> profiles.read (derived)
authorization: GET /api/profiles/:id/comments -> comments.read (derived)
authorization: GET /api/users -> users.read (derived)
authorization: POST /api/users -> users.create (derived)
authorization: DELETE /api/users/:id -> users.delete (derived)
```

C'est le filet de sécurité qui remplace la validation d'une déclaration manquante : une nouvelle route apparaît dans ce log, et comme sa permission n'est accordée à aucun rôle, elle répond `403` — jamais `200`. Avec le [store SQL](#store-postgresql), elle apparaît aussi dans `authz_permissions` à la synchronisation.

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
# {"data":[{"id":1,"title":"Hello"}]}            → 200 (reader a articles.read)

curl -X POST -H "X-Debug-Subject: $BOB" http://localhost:8080/api/articles
# {"error":"Forbidden"}                          → 403 (reader n'a pas articles.create)

curl -X POST -H "X-Debug-Subject: $ALICE" http://localhost:8080/api/articles
# {"data":{"id":2}}                              → 201 (editor a articles.create)
```

## Validation au démarrage

Avec un authorizer configuré, le boot échoue si une route est mal déclarée, en listant **toutes** les violations en une seule fois :

```text
Error loading handlers: route permission validation failed (2 violation(s)):
/api/users: Permissions: declared in 2 files (users/test.go, users/users.go): route permissions must be declared in a single file
articles/route.go: DELETE: permission declared but no exported handler with this name
```

Sont refusés au démarrage :

- une méthode **non dérivable** et non déclarée (route racine `/api`, méthode HTTP hors convention) ;
- une déclaration dans **plusieurs** fichiers du même dossier ;
- une **entrée orpheline** : clé de la map sans handler exporté correspondant (attrape les fautes de frappe) ;
- une permission **vide** ;
- si `WithKnownPermissions` est utilisé, toute permission **inconnue** — vérifiée aussi bien sur les valeurs surchargées que sur les permissions dérivées.

Un dossier sans handler exporté (fichier utilitaire) n'a rien à déclarer.

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

// PATCH dérive articles.update — aucune map nécessaire.

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
	//    permission de la méthode courante (ici articles.update, dérivée).
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

Le masquage 404 et l'escalade par permission sont détaillés dans [Cas complet : profils privés](#cas-complet--profils-privés).

Les autres helpers du contexte :

| Méthode | Effet HTTP | Usage |
| --- | --- | --- |
| `c.HasPermission("article.read")` | aucun | question RBAC seule |
| `c.Can("article.update", article)` | aucun | décision complète, pour du contenu conditionnel |
| `c.Authorize(article)` | statut + abort si refus | permission effective de la route (dérivée ou surchargée) |
| `c.AuthorizeAction("autre.action", article)` | statut + abort si refus | action explicite |

Sur une méthode `Public`, `c.Authorize` sans action est indéfini (aucune permission à réutiliser) et répond 500 : utilisez `c.Can` ou `c.AuthorizeAction`.

## Cas complet : profils privés

Un profil « privé » n'est pas une permission — c'est une propriété de **l'objet**. Une permission `profiles.read_private` accordée par rôle dirait « peut lire tous les profils privés » ; la vraie règle est « peut lire *celui-ci* ». C'est donc de l'ABAC, et la règle complète tient en un seul endroit grâce à `PolicyInput.Grants` :

```go
// main.go
func canViewProfile(_ context.Context, in authorization.PolicyInput) (bool, error) {
	profile, ok := in.Resource.(interface {
		IsPublic() bool
		OwnerID() string
	})
	if !ok {
		// Ressource inattendue = erreur technique → 503, jamais une autorisation.
		return false, fmt.Errorf("resource %T is not visibility-aware", in.Resource)
	}
	if profile.IsPublic() || profile.OwnerID() == in.Subject.ID {
		return true, nil
	}

	// Escalade par PERMISSION (auditable, révocable), jamais par nom de rôle.
	return in.Grants.HasPermission("profiles.read_private"), nil
}

engine := authorization.NewEngine(
	authorization.WithProvider(provider),
	authorization.WithRestriction("profiles.read", canViewProfile),
	authorization.WithNotFoundOnDeny("profiles.read"), // voir ci-dessous
)
```

`in.Grants` contient les grants RBAC que le moteur vient de résoudre pour ce sujet (union `{scope, ScopeGlobal}`) — aucune requête supplémentaire. Le rôle `moderator` reçoit simplement la permission d'escalade :

```go
must(provider.GrantPermissions(ctx, "moderator", "profiles.read", "profiles.read_private"))
```

> [!WARNING]
> Utilisez `Grants.HasPermission`, jamais `Grants.HasRole("admin")` : un test sur un nom de rôle reconstitue le bypass admin implicite que le refus par défaut interdit — pouvoir qu'aucune ligne de permission n'enregistre, impossible à révoquer sans défaire tout le rôle, et qui dérive dès qu'un rôle est renommé.

Le handler charge la ressource puis tranche :

```go
// api/profiles/id_/route.go — GET dérive profiles.read
func GET(c *yogourt.Context, params Params) {
	profile := loadProfile(params.ID) // sa visibilité n'est connue qu'ici

	if !c.Authorize(profile) {
		return // 404 grâce au masquage, rien n'a fuité
	}

	routing.RespondSuccess(c.Gin(), gin.H{"profile": profile})
}
```

**Pourquoi `WithNotFoundOnDeny` est indispensable ici** : un 403 sur un profil privé dirait « ce profil existe et il est privé ». Un attaquant énumère les identifiants et cartographie tes utilisateurs. Le masquage rend le refus indistinguable d'un profil inexistant — et comme il est déclaré par action, le middleware RBAC et `c.Authorize` répondent identiquement.

### Sous-ressource gouvernée par le parent

`/api/profiles/:id/comments` dérive `comments.read`, mais la confidentialité vient du **parent**. Le handler réutilise exactement la même règle, donc le même masquage :

```go
// api/profiles/id_/comments/route.go
func GET(c *yogourt.Context, params Params) {
	// Le middleware a déjà validé comments.read ; l'accès est gouverné par
	// le profil parent.
	parent := loadProfile(params.ProfileID)
	if !c.AuthorizeAction("profiles.read", parent) {
		return
	}

	routing.RespondSuccess(c.Gin(), loadComments(params.ProfileID))
}
```

Résultat, vérifié de bout en bout :

| Requête | Sujet | Statut |
| --- | --- | ---: |
| `GET /api/profiles/1` (public) | anonyme | `401` |
| `GET /api/profiles/1` (public) | reader | `200` |
| `GET /api/profiles/2` (privé) | tiers | `404` |
| `GET /api/profiles/2` (privé) | propriétaire | `200` |
| `GET /api/profiles/2` (privé) | moderator | `200` |
| `GET /api/profiles/2/comments` | tiers | `404` |
| `GET /api/profiles/2/comments` | moderator | `200` |

### Les listes : c'est du SQL, pas de l'ABAC

`GET /api/profiles` renvoie une collection : l'ABAC statue objet par objet, il ne filtre pas une liste. Le filtrage doit vivre **dans la requête** (`WHERE visibility = 'public' OR owner_id = ?`). Ne bouclez jamais sur `c.Can` pour filtrer une page : coût en N requêtes, et un oubli sur un seul endpoint suffit à tout fuiter.

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
provider.BindRoles(ctx, userID, "tenant-42", "editor")

// Fixer le scope de la requête (par ex. dans un middleware) :
c.Request = c.Request.WithContext(authorization.WithScope(c.Request.Context(), "tenant-42"))
```

Sans appel à `WithScope`, tout se résout dans `ScopeGlobal` — une application mono-tenant ignore simplement les scopes.

> [!WARNING]
> Ne construisez jamais un scope à partir d'une entrée utilisateur brute. La valeur sentinelle de `ScopeGlobal` (`@global`) est hors d'atteinte des identifiants habituels, mais la validation des noms de tenants reste la responsabilité de l'application.

## Store PostgreSQL

`authorization/gormstore` fournit le provider SQL de production. Les tables (`authz_permissions`, `authz_roles`, `authz_role_permissions`, `authz_role_bindings`) sont créées par une migration SQL versionnée, embarquée dans le package et appliquée **explicitement** — jamais d'`AutoMigrate` :

```go
db, err := providers.GetDB() // ou toute connexion GORM
if err != nil {
	log.Fatal(err)
}
if err := gormstore.Migrate(ctx, db); err != nil {
	log.Fatal(err)
}
store := gormstore.New(db)

engine := authorization.NewEngine(authorization.WithProvider(store))
routing.Initialize(routing.WithAuthorizer(engine))
```

**Aucune permission ne s'insère à la main.** Au démarrage, le framework enregistre automatiquement dans le store toutes les permissions déclarées par les routes (synchronisation additive : rien n'est jamais supprimé), et `GrantPermissions` enregistre de lui-même une permission encore inconnue. Il ne reste à administrer que les rôles et les bindings — à l'exécution, y compris depuis une interface web (voir [Administrer les rôles](#administrer-les-rôles-depuis-une-interface-web)) :

```go
store.CreateRole(ctx, "editor") // idempotent
store.GrantPermissions(ctx, "editor", "articles.read", "articles.update")
store.BindRoles(ctx, aliceID, authorization.ScopeGlobal, "editor")
```

La résolution des grants tient en une seule requête indexée par `(sujet, scope)`. Toutes les opérations acceptent un `context.Context`, sont transactionnelles, idempotentes, et retournent leurs erreurs SQL. Les tests d'intégration du package se lancent avec `YOGOURT_TEST_DSN` (ils sont ignorés sinon).

## Administrer les rôles depuis une interface web

Rien n'est figé dans le code : rôles, permissions accordées et attributions se gèrent **à l'exécution**. Les deux providers implémentent `authorization.GrantAdmin`, qui ajoute aux mutations les lectures nécessaires à une interface d'administration :

| Méthode | Usage dans une UI |
| --- | --- |
| `Permissions(ctx)` | la **liste fermée** dans laquelle choisir — alimentée automatiquement au boot par les permissions dérivées des routes |
| `Roles(ctx)` | l'écran des rôles |
| `RolePermissions(ctx, role)` | ce qu'un rôle accorde |
| `Bindings(ctx, subjectID)` | les rôles d'un utilisateur, tous scopes |
| `RoleBindings(ctx, role)` | qui détient un rôle |
| `CreateRole` / `DeleteRole` | gestion des rôles |
| `GrantPermissions` / `RevokePermissions` | composition d'un rôle |
| `BindRoles` / `UnbindRoles` | attribution à un utilisateur, par scope |

Une route d'admin n'a rien à recevoir du `main.go` : elle reconstruit le store depuis le provider de base de données du framework.

```go
// api/admin/roles/route.go — GET dérive roles.read, POST dérive roles.create
func GET(c *gin.Context) {
	db, err := providers.GetDB()
	if err != nil {
		routing.RespondServiceUnavailable(c)
		return
	}

	store := gormstore.New(db)

	roles, err := store.Roles(c.Request.Context())
	if err != nil {
		routing.RespondServiceUnavailable(c)
		return
	}

	routing.RespondSuccess(c, roles)
}
```

Les routes d'administration sont elles-mêmes protégées par leurs permissions dérivées (`roles.read`, `roles.create`…). Quand le verbe HTTP ne décrit pas l'action métier, une surcharge partielle suffit :

```go
// api/admin/users/userId_/roles/route.go
// Attribuer un rôle n'est pas « créer un rôle » ; GET garde sa dérivation.
var Permissions = map[string]string{
	"POST":   "roles.assign",
	"DELETE": "roles.unassign",
}
```

### Le premier administrateur

Au tout premier démarrage, personne ne détient les permissions d'administration — donc personne ne peut en accorder. Il faut un amorçage explicite, idempotent, déclenché par une variable d'environnement :

```go
if subject := os.Getenv("ADMIN_SUBJECT"); subject != "" {
	store.CreateRole(ctx, "admin")
	store.GrantPermissions(ctx, "admin", "roles.read", "roles.create", "roles.assign", /* ... */)
	store.BindRoles(ctx, subject, authorization.ScopeGlobal, "admin")
}
```

Le rôle `admin` ne porte que des permissions **explicites** : le framework n'accorde aucun privilège à un rôle en fonction de son nom.

### Effet immédiat

Les grants sont résolus à chaque requête : une attribution ou une révocation prend effet **dès la requête suivante**, sans redémarrage. Cycle vérifié de bout en bout :

```sh
# L'admin crée un rôle, lui accorde une permission, l'attribue à un utilisateur
curl -X POST -H "$ADMIN" -d '{"role":"reader"}'                localhost:8080/api/admin/roles
curl -X POST -H "$ADMIN" -d '{"permissions":["users.read"]}'   localhost:8080/api/admin/roles/reader/permissions
curl -X POST -H "$ADMIN" -d '{"roles":["reader"]}'             localhost:8080/api/admin/users/$USER/roles

curl -H "$USER_HEADER" localhost:8080/api/users   # 403 avant, 200 après — sans redémarrage

# Révocation
curl -X DELETE -H "$ADMIN" -d '{"roles":["reader"]}' localhost:8080/api/admin/users/$USER/roles
curl -H "$USER_HEADER" localhost:8080/api/users   # 403 de nouveau
```

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

- renommer un dossier change la permission dérivée : l'ancienne reste en base (synchronisation additive) et les bindings qui la référencent deviennent inopérants — à traiter comme une migration de données (Django a le même travers) ;
- aucune interface d'administration n'est livrée : le framework fournit le contrat `GrantAdmin` et les routes sont à écrire côté application (voir ci-dessus) ;
- pas de commande CLI `yogourt routes` ni `permissions sync` ;
- pas de cache des grants entre requêtes : un `Resolve` (deux si scope ≠ global) par contrôle ;
- l'ADR consolidant les décisions de conception reste à rédiger ;
- le chargement des plugins impose toujours les contraintes du package `plugin` de Go (voir [Routage](routing.md)).
