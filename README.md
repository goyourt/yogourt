# Yogourt

Yogourt est un framework Go pour construire des API avec Gin, un routage basé sur le système de fichiers, GORM, PostgreSQL, Redis et des services d’authentification et de fichiers.

> [!WARNING]
> La v2 est en préversion sur la branche <code>release/v2.0.0</code>. Le module porte encore le chemin <code>github.com/goyourt/yogourt</code>, aucun tag v2 n’est publié et le CLI stable v0.5 n’est pas compatible avec cette organisation. Pour tester cette version dans une autre application, épinglez une révision v2 précise ; n’utilisez pas <code>@latest</code>.

## Documentation

| Guide | Contenu |
| --- | --- |
| [Configuration](docs/configuration.md) | Fichiers YAML, variables d’environnement et limites runtime |
| [Routage](docs/routing.md) | Routes fichiers, handlers, paramètres, middlewares et plugins |
| [Services](docs/services.md) | Modèles, base de données, authentification, mots de passe et fichiers |
| [Autorisation](docs/authorization.md) | RBAC, ABAC, permissions par route, scopes et statuts HTTP |
| [Migration vers la v2](docs/migration-v2.md) | Changements incompatibles et checklist de migration |

## Prérequis

- Go 1.24 ou une version compatible avec le module ;
- macOS, Linux ou FreeBSD pour charger les plugins Go ;
- PostgreSQL pour les services de base de données ;
- Redis pour le cache et le suivi des échecs de mot de passe.

Le binaire principal et tous les plugins doivent être compilés avec exactement la même version de Go, les mêmes dépendances, les mêmes tags et les mêmes options de build. Consultez les [contraintes du paquet standard <code>plugin</code>](https://pkg.go.dev/plugin).

## Démarrage minimal

Cette procédure suppose que l’application dépend déjà d’une révision v2 épinglée.

### 1. Créer l’arborescence

~~~text
.
├── api/
│   └── health/
│       └── route.go
├── configs/
│   └── yogourt.yaml
├── middleware/
│   └── middleware.go
├── .yogourt/
└── main.go
~~~

Le dossier <code>.yogourt</code> contient des artefacts compilés et ne doit normalement pas être versionné. Ajoutez <code>.yogourt/</code> au <code>.gitignore</code> de l’application.

### 2. Configurer le runtime

Créez <code>configs/yogourt.yaml</code> :

~~~yaml
app_name: "example"
version: "2.0.0"
mode: "development"

server:
  port: 8080

cors:
  allowed_origins:
    - "http://localhost:3000"
  allowed_methods:
    - GET
    - POST
    - PUT
    - PATCH
    - DELETE
    - OPTIONS
  allowed_headers:
    - Origin
    - Content-Type
    - Authorization
~~~

Sans origine explicite, le runtime active désormais <code>AllowAllOrigins</code> sans panic. Pour la production, conservez une liste d’origines fermée. Voir la [configuration CORS](docs/configuration.md#cors).

### 3. Ajouter le middleware

Créez <code>middleware/middleware.go</code> :

~~~go
package main

import "github.com/gin-gonic/gin"

var Callbacks = map[string]func(*gin.Context){}

func main() {}
~~~

Ce fichier et son symbole <code>Callbacks</code> sont obligatoires, même si aucun middleware applicatif n’est encore défini.

### 4. Ajouter une route

Créez <code>api/health/route.go</code> :

~~~go
package main

import (
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/routing"
)

func GET(c *gin.Context) {
	routing.RespondSuccess(c, gin.H{"status": "ok"})
}

func main() {}
~~~

Cette source expose <code>GET /api/health</code>.

### 5. Démarrer l’application

Créez <code>main.go</code> :

~~~go
package main

import "github.com/goyourt/yogourt/routing"

func main() {
	routing.Initialize("api")
}
~~~

Le runtime v2 ne compile pas les plugins. Construisez-les avant de lancer le serveur :

~~~sh
mkdir -p .yogourt/middleware .yogourt/api/health
go build -buildmode=plugin -o .yogourt/middleware/middleware.go.so ./middleware/middleware.go
go build -buildmode=plugin -o .yogourt/api/health/route.go.so ./api/health/route.go
go run .
~~~

Puis vérifiez :

~~~sh
curl http://localhost:8080/api/health
~~~

Réponse :

~~~json
{"data":{"status":"ok"}}
~~~

## Routage en bref

L’URL dépend du dossier contenant le fichier, pas de son nom :

| Source | Route |
| --- | --- |
| <code>api/route.go</code> | <code>/api</code> |
| <code>api/users/route.go</code> | <code>/api/users</code> |
| <code>api/users/id_/route.go</code> | <code>/api/users/:id</code> |
| <code>api/users/id_/posts/postSlug_/route.go</code> | <code>/api/users/:id/posts/:postSlug</code> |

Les symboles exportés reconnus sont <code>GET</code>, <code>POST</code>, <code>PUT</code>, <code>PATCH</code> et <code>DELETE</code>. Les handlers Gin existants restent compatibles ; <code>*yogourt.Context</code> permet en plus l’injection typée des paramètres de chemin.

~~~go
package main

import (
	"github.com/goyourt/yogourt"
	"github.com/goyourt/yogourt/routing"
)

type Params struct {
	UserID int `param:"id"`
}

func GET(c *yogourt.Context, params Params) {
	routing.RespondSuccess(c.Gin(), params)
}

func main() {}
~~~

La syntaxe de dossier <code>id_</code> est recommandée. Les formes historiques <code>[id]</code> et <code>_id</code> restent reconnues. Le [guide de routage](docs/routing.md) décrit les signatures, conversions et règles de middleware.

## CLI

Le CLI publié ne doit pas être utilisé pour initialiser une application v2 : il génère encore <code>config.yaml</code>, la clé <code>database.dbname</code> et une structure ancienne incompatible avec ce runtime. Les commandes de compilation <code>build</code>, <code>dev</code> et <code>start</code> sont en cours de préparation mais ne font pas partie d’une version publiée.

En attendant leur publication, utilisez les commandes <code>go build -buildmode=plugin</code> indiquées ci-dessus.

## Vérification du framework

Depuis ce dépôt :

~~~sh
go test ./...
go test -race ./...
go vet ./...
~~~

Un test d’intégration compile de vrais plugins (<code>go build -buildmode=plugin</code>) et fait tourner le chargeur du framework dessus : ouverture des <code>.so</code>, extraction des symboles, enregistrement des routes, permissions dérivées et déclarées. Il est inclus dans <code>go test ./...</code> et coûte quelques secondes par plugin, beaucoup plus au premier appel puisque toutes les dépendances doivent être recompilées pour ce mode de construction :

~~~sh
go test ./routing -run TestPluginRoutesEndToEnd -v   # ce test seul
go test -short ./...                                 # le saute
~~~

Sur une plateforme sans support des plugins Go, le test se saute de lui-même avec un message explicite.

## Limites connues de la préversion

- les plugins Go ne sont pas portables vers Windows et sont sensibles à toute différence de toolchain ou de dépendances ;
- le runtime vérifie uniquement l’existence des fichiers <code>.so</code>, pas leur fraîcheur ;
- le préfixe HTTP est toujours <code>/api</code>, quel que soit l’argument de <code>routing.Initialize</code> ;
- une collision méthode/route n’est pas détectée avant l’enregistrement et peut provoquer un panic Gin ;
- <code>server.cors</code> est encore ignoré et <code>cors.max_age</code> est mal converti ;
- le fournisseur de base de données est PostgreSQL uniquement et force <code>sslmode=disable</code>.

Ces contraintes sont détaillées dans les guides afin de ne pas les confondre avec des garanties de la future version stable.
