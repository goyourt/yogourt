# Routage

Yogourt transforme les dossiers Go d’un répertoire source en routes Gin. La préversion v2 charge des plugins déjà compilés ; elle ne compile plus les sources au démarrage.

## Convention de fichiers

<code>routing.Initialize("api")</code> parcourt récursivement le dossier <code>api</code> depuis le répertoire de lancement.

~~~text
api/
├── route.go
├── health/
│   └── route.go
└── users/
    ├── route.go
    └── id_/
        └── route.go
~~~

| Source | URL |
| --- | --- |
| <code>api/route.go</code> | <code>/api</code> |
| <code>api/health/route.go</code> | <code>/api/health</code> |
| <code>api/users/route.go</code> | <code>/api/users</code> |
| <code>api/users/id_/route.go</code> | <code>/api/users/:id</code> |

Le nom du fichier n’entre pas dans l’URL. Tous les fichiers <code>.go</code> sont chargés, sauf ceux terminés par <code>_test.go</code>.

Le dossier scanné et le préfixe HTTP sont deux réglages distincts. <code>route_folder: "routes"</code> scanne le dossier <code>routes</code> et publie sous <code>/api</code>, le préfixe par défaut.

## Préfixe HTTP

Le préfixe vient, dans cet ordre : de l’option <code>routing.WithPrefix</code>, du champ <code>server.base_path</code> de la configuration, puis de la valeur par défaut <code>/api</code>.

~~~go
routing.Initialize(routing.WithPrefix("/v1"))   // /v1/users
routing.Initialize(routing.WithPrefix("/"))     // /users
routing.Initialize()                            // server.base_path, sinon /api
~~~

| Écriture | Préfixe retenu |
| --- | --- |
| <code>"/v1"</code>, <code>"v1"</code>, <code>"/v1/"</code> | <code>/v1</code> |
| <code>"/api/v2"</code> | <code>/api/v2</code> |
| <code>"/"</code> | racine : <code>api/users/route.go</code> répond sur <code>/users</code> |

Un <code>base_path</code> vide ou absent n’est pas la racine, c’est le défaut <code>/api</code> : servir à la racine se demande explicitement avec <code>/</code>.

Un préfixe contenant un paramètre Gin (<code>:id</code>, <code>*path</code>) ou une espace arrête le démarrage, en nommant sa source — l’option ou le champ de configuration.

Le préfixe désigne un point de montage, pas une ressource : il est exclu des permissions dérivées par convention. Sous <code>/v1</code>, <code>api/users/route.go</code> dérive toujours <code>users.read</code>, jamais <code>v1.read</code>.

## Dossier des routes

Le dossier scanné vient du seul <code>paths.route_folder</code> du fichier de
configuration :

~~~yaml
paths:
  route_folder: "api"
~~~

<code>Initialize</code> ne prend pas de dossier en argument : le dossier d’un
déploiement est un réglage, et le déclarer à deux endroits permettait à un
programme de contredire son propre <code>yogourt.yaml</code>. Sans
<code>route_folder</code>, le démarrage échoue en nommant la clé. Voir le
[guide de configuration](configuration.md).

## Segments dynamiques

La forme recommandée ajoute un underscore à la fin du dossier :

~~~text
api/users/userId_/posts/postSlug_
~~~

Elle produit :

~~~text
/api/users/:userId/posts/:postSlug
~~~

Trois syntaxes sont reconnues :

| Dossier | Segment Gin | Statut |
| --- | --- | --- |
| <code>id_</code> | <code>:id</code> | Recommandé |
| <code>[id]</code> | <code>:id</code> | Compatibilité |
| <code>_id</code> | <code>:id</code> | Historique |

<code>id_</code> est préférable, car le dossier reste compatible avec les outils Go.

## Fichiers route

Chaque fichier est compilé comme un plugin autonome. Il doit utiliser <code>package main</code> et déclarer une fonction <code>main</code> vide :

~~~go
package main

import "github.com/gin-gonic/gin"

func GET(c *gin.Context) {
	c.JSON(200, gin.H{"message": "ok"})
}

func main() {}
~~~

Les seuls symboles HTTP reconnus sont :

- <code>GET</code> ;
- <code>POST</code> ;
- <code>PUT</code> ;
- <code>PATCH</code> ;
- <code>DELETE</code>.

Un fichier peut exposer plusieurs de ces fonctions. Elles ne doivent rien retourner.

## Signatures de handler

Quatre signatures sont acceptées :

~~~go
func GET(c *gin.Context)
func GET(c *yogourt.Context)
func GET(c *yogourt.Context, params Params)
func GET(c *yogourt.Context, params *Params)
~~~

Une autre signature fait échouer le chargement du plugin.

<code>yogourt.Context</code> embarque <code>*gin.Context</code>. Les méthodes Gin sont donc directement disponibles, et <code>c.Gin()</code> restitue explicitement le contexte brut attendu par les helpers Yogourt existants.

~~~go
func GET(c *yogourt.Context) {
	routing.RespondSuccess(c.Gin(), gin.H{"status": "ok"})
}
~~~

Les handlers <code>*gin.Context</code> de la v1 restent compatibles.

## Injection des paramètres

Le second argument d’un handler Yogourt doit être une structure ou un pointeur vers une structure.

~~~go
package main

import (
	"github.com/goyourt/yogourt"
	"github.com/goyourt/yogourt/routing"
)

type Params struct {
	UserID   uint64 `param:"userId"`
	PostSlug string
	Draft    *bool `param:"draft"`
	Ignored  string `param:"-"`
}

func GET(c *yogourt.Context, params Params) {
	routing.RespondSuccess(c.Gin(), params)
}

func main() {}
~~~

Les règles de correspondance sont :

- <code>param:"name"</code> sélectionne explicitement le paramètre ;
- <code>param:"-"</code> ignore le champ ;
- sans tag, le nom du champ est comparé sans tenir compte de la casse ni de la ponctuation : <code>UserID</code> correspond à <code>userId</code> ;
- seuls les champs exportés sont renseignés.

Types pris en charge :

- <code>string</code> ;
- <code>bool</code> ;
- entiers signés et non signés ;
- <code>float32</code> et <code>float64</code> ;
- pointeurs vers ces types.

L’injection concerne uniquement les paramètres de chemin Gin. Elle ne lit ni la query string, ni les headers, ni le body. Une conversion impossible interrompt le handler avec un HTTP 400 au corps générique — <code>{"error":"Invalid request parameters"}</code> — le détail de la conversion restant côté serveur, dans les logs.

## Body JSON

<code>routing.HandleRequest</code> lie un body JSON à une structure :

~~~go
type CreateRequest struct {
	Name string `json:"name" binding:"required"`
}

func POST(c *yogourt.Context) {
	var req CreateRequest
	if !routing.HandleRequest(c.Gin(), &req) {
		return
	}

	routing.RespondCreated(c.Gin(), req)
}
~~~

Passez un pointeur vers une structure. Une erreur de binding renvoie HTTP 422 :

~~~json
{"error":"Invalid request: argument mismatch"}
~~~

Le helper tente aussi d’hydrater certaines relations qui implémentent <code>interfaces.BaseInterface</code> et possèdent un UUID. Une panne de base pendant cette hydratation interrompt désormais la requête avec un <code>503</code> générique. Un UUID inconnu, en revanche, laisse volontairement l’objet non hydraté et laisse tourner le handler : répondre autrement donnerait à un appelant anonyme un oracle d’existence sur la table référencée.

## Réponses

Les helpers attendent <code>*gin.Context</code> :

~~~go
routing.RespondSuccess(c, data)                    // 200 {"data": ...}
routing.RespondCreated(c, data)                    // 201 {"data": ...}
routing.RespondWithContent(c, 202, "job", job)     // 202 {"job": ...}
routing.RespondNoContent(c)                        // 204
routing.RespondNotFound(c)                         // 404
routing.RespondAndAbort(c, 400, "invalid request") // 400 {"error": ...}
~~~

Depuis <code>*yogourt.Context</code>, passez <code>c.Gin()</code>.

<code>RespondSuccess</code>, <code>RespondCreated</code>, <code>RespondWithContent</code> et <code>RespondNoContent</code> appellent ensuite <code>c.Next()</code>. <code>RespondAndAbort</code> et <code>RespondNotFound</code> interrompent au contraire la chaîne.

## Middlewares

Le runtime charge obligatoirement :

~~~text
middleware/middleware.go
→ .yogourt/middleware/middleware.go.so
~~~

Le plugin doit exporter une variable <code>Callbacks</code> du type exact <code>map[string]func(*gin.Context)</code>.

~~~go
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func authenticate(c *gin.Context) {
	if c.GetHeader("Authorization") == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Next()
}

func adminOnly(c *gin.Context) {
	// Vérifier les droits...
	c.Next()
}

var Callbacks = map[string]func(*gin.Context){
	"/api":               authenticate,
	"/api/auth":          nil,
	"^/api/admin":        adminOnly,
	"/api/users/:userId": authenticate,
}

func main() {}
~~~

Règles :

- la clé <code>/</code> s’applique à toutes les routes ;
- les callbacks parents s’empilent avec ceux des sous-routes ;
- une valeur <code>nil</code> efface les callbacks hérités pour la route et ses enfants ;
- le préfixe <code>^</code> remplace les callbacks parents par le callback indiqué ;
- les callbacks utilisent toujours <code>*gin.Context</code>.

Pour une route dynamique, utilisez le format Gin dans la map : <code>/api/users/:userId</code>. Les formes de dossier <code>userId_</code>, <code>[userId]</code> et <code>_userId</code> ne sont pas normalisées dans les clés de <code>Callbacks</code>.

## Compilation des plugins

Le chemin de sortie reproduit le chemin source sous <code>.yogourt</code> et ajoute <code>.so</code> :

~~~text
api/users/userId_/handler.go
→ .yogourt/api/users/userId_/handler.go.so
~~~

Compilation manuelle depuis la racine de l’application :

~~~sh
mkdir -p .yogourt/middleware .yogourt/api/users/userId_
go build -buildmode=plugin -o .yogourt/middleware/middleware.go.so ./middleware/middleware.go
go build -buildmode=plugin -o .yogourt/api/users/userId_/handler.go.so ./api/users/userId_/handler.go
~~~

Chaque fichier Go scanné doit avoir son plugin, même s’il n’expose aucun verbe HTTP. Le runtime :

- vérifie seulement que le fichier <code>.so</code> existe ;
- ne compare pas les dates ou les hashes ;
- ne reconstruit pas les plugins obsolètes ;
- ne nettoie pas les anciens artefacts ;
- ne redémarre pas le serveur après une modification.

Reconstruisez tous les plugins après un changement de Go, de dépendance, de tag ou d’option de build. Les commandes CLI <code>build</code>, <code>dev</code> et <code>start</code> ne sont pas encore publiées.

## Démarrage

~~~go
package main

import "github.com/goyourt/yogourt/routing"

func main() {
	routing.Initialize("api")
}
~~~

L’argument désigne le dossier à scanner depuis le répertoire courant. Le processus charge la configuration, installe CORS, charge le plugin middleware, charge les routes, puis écoute sur <code>&lt;server.host&gt;:&lt;server.port&gt;</code>. Si <code>server.host</code> est vide, l’adresse par défaut est <code>0.0.0.0</code> ; les adresses IPv6 sont correctement encadrées.

## Limites actuelles

- une collision entre deux fichiers qui exposent la même méthode sur la même URL n’est pas détectée en amont et peut provoquer un panic Gin ;
- les plugins sont chargés en concurrence, donc l’ordre d’enregistrement n’est pas déterministe ;
- le préfixe URL reste <code>/api</code> ;
- le runtime dépend des contraintes d’ABI et de plateforme du paquet Go <code>plugin</code> ;
- le support du race detector avec les plugins Go est limité ;
- un plugin compilé avec une toolchain ou des dépendances différentes peut échouer dans <code>plugin.Open</code>.
