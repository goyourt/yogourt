# Migration de v1.1 vers v2

Ce guide cible la branche de préversion <code>release/v2.0.0</code>. Il sera à ajuster lors de la publication du tag stable.

> [!IMPORTANT]
> Le module porte encore le chemin <code>github.com/goyourt/yogourt</code>. N’ajoutez pas <code>/v2</code> aux imports tant que le module et le tag v2 n’ont pas été publiés sous cette forme. Pour les essais, épinglez une révision v2 précise.

## Résumé des changements incompatibles

| v1.1 | Préversion v2 |
| --- | --- |
| <code>config.yaml</code> à la racine | <code>configs/yogourt.yaml</code> |
| <code>providers.GetConfig()</code> | <code>providers.GetMainConfig()</code> |
| <code>providers.Config</code> | <code>providers.MainConfig</code> |
| Compilation implicite au runtime | Plugins <code>.so</code> précompilés sous <code>.yogourt</code> |
| <code>RespondSuccess(c, status, data)</code> | <code>RespondSuccess(c, data)</code> |
| Dossier middleware parfois documenté <code>middlewares/</code> | Chemin exact <code>middleware/middleware.go</code> |
| Paramètres dynamiques historiques | <code>id_</code> recommandé ; anciennes formes toujours compatibles |

Les handlers <code>func(*gin.Context)</code> restent compatibles. L’adoption de <code>*yogourt.Context</code> et de l’injection typée peut être progressive.

Le framework v1.1 utilisait déjà <code>database.db</code> et <code>paths.route_folder</code>. En revanche, les templates du CLI v0.5 génèrent encore les anciennes clés <code>database.dbname</code> et <code>paths.api_folder</code> : corrigez-les si le projet provient de ce CLI.

## 1. Épingler la préversion

Ne migrez pas une application de production vers une branche flottante. Épinglez le commit v2 que vous avez validé et conservez la même révision dans :

- le module principal ;
- les plugins de routes ;
- le plugin middleware ;
- le système de build et l’image de déploiement.

La toolchain Go et le graphe de dépendances doivent aussi être identiques entre le binaire et les plugins.

## 2. Déplacer la configuration

Déplacez le fichier principal :

~~~text
config.yaml
→ configs/yogourt.yaml
~~~

La structure attendue contient notamment :

~~~yaml
database:
  db: "example"

paths:
  route_folder: "api"
~~~

Si le fichier a été généré par le CLI v0.5, remplacez <code>dbname</code> par <code>db</code> et <code>api_folder</code> par <code>route_folder</code>. Il s’agit d’une correction du template CLI, pas d’un changement entre les contrats du framework v1.1 et v2. Un oubli se voit maintenant au démarrage : le chargement de la configuration nomme la clé et son remplaçant.

<pre><code>⚠️  configs/yogourt.yaml: "paths.api_folder" was renamed to "paths.route_folder" — the value is ignored
</code></pre>

Depuis la v2, <code>paths.route_folder</code> n’est plus décoratif : c’est le dossier de routes chargé par <code>routing.Initialize</code>, et la seule source de ce dossier. En revanche <code>paths.model_folder</code>, <code>paths.project_name</code> et <code>paths.main_file</code> ont quitté le contrat runtime — le framework ne les a jamais lus. Vous pouvez les supprimer ; le démarrage le rappelle.

<strong>Rupture de compilation</strong> : <code>routing.Initialize</code> ne prend
plus de dossier en argument. Déplacez celui de votre appel dans le fichier de
configuration :

~~~go
routing.Initialize("api", routing.WithAuthorizer(engine))  // v1
routing.Initialize(routing.WithAuthorizer(engine))         // v2
~~~

~~~yaml
paths:
  route_folder: "api"
~~~

Le dossier scanné est un réglage du déploiement : le déclarer à la fois dans le
code et dans la configuration permettait à un programme de contredire son
propre <code>yogourt.yaml</code>. Sans <code>route_folder</code>, le démarrage
échoue en nommant la clé.

<strong>Base de données</strong> : le DSN n’impose plus <code>sslmode=disable</code>.
C’est désormais la valeur par défaut de <code>database.ssl_mode</code>, donc une
configuration inchangée ouvre la même connexion qu’avant, et un déploiement qui
exige TLS n’a plus à adapter le fournisseur :

~~~yaml
database:
  ssl_mode: "verify-full"
  ssl_root_cert: "/etc/ssl/certs/db-ca.crt"
  search_path: "app,public"
  pool:
    max_open_conns: 25
    conn_max_lifetime: 30m
~~~

Les valeurs du DSN sont aussi échappées : un mot de passe contenant une espace
ou une apostrophe était jusque-là concaténé tel quel et tronquait la chaîne de
connexion.

<strong>Rupture de compilation</strong> : un échec de connexion n’arrête plus le
processus, il est retourné. Les signatures suivantes changent :

| Avant | Après |
| --- | --- |
| <code>providers.GetDB() *gorm.DB</code> | <code>providers.GetDB() (*gorm.DB, error)</code> |
| <code>providers.InitDB() *gorm.DB</code> | <code>providers.InitDB() (*gorm.DB, error)</code> |
| <code>database.SearchQuery(...) *gorm.DB</code> | <code>database.SearchQuery(...) (*gorm.DB, error)</code> |
| <code>database.JoinTables(...) *gorm.DB</code> | <code>database.JoinTables(...) (*gorm.DB, error)</code> |

~~~go
db := providers.GetDB()                    // v1 : log.Fatalf, le processus sort

db, err := providers.GetDB()               // v2
if err != nil {
	routing.RespondServiceUnavailable(c)
	return
}
~~~

<code>GetOneBy</code>, <code>GetAll</code>, <code>GetAllPaginated</code>, les
méthodes de <code>DataWriter</code>, <code>HardDelete</code> et les helpers
<code>Hydrate*</code> gardent leur signature : ils retournent maintenant
l’erreur de connexion là où ils retournaient déjà l’erreur GORM. Une base
indisponible fait donc échouer les requêtes qui en ont besoin, au lieu
d’emporter le processus depuis la première d’entre elles. La connexion n’étant
mémorisée qu’une fois ouverte, une base qui revient est reprise sans
redémarrage.

<pre><code>⚠️  configs/yogourt.yaml: "paths.model_folder" is not a runtime setting (only the v0.5 CLI reads it) — delete it
</code></pre>

Deux autres champs ont pris un sens : <code>database.type</code> est contrôlé (seul PostgreSQL est fourni, une autre valeur arrête le démarrage) et <code>server.base_path</code> règle le préfixe HTTP, jusque-là figé à <code>/api</code>.

<strong>Changement de comportement</strong> : <code>services.GetBaseUrl</code> ne
retourne plus <code>server.host</code> tel quel. <code>server.host</code> est une
adresse d’écoute — avec <code>host: "0.0.0.0"</code>, l’ancien helper retournait
<code>0.0.0.0</code>, sans schéma ni port. L’URL est désormais reconstruite avec
son schéma et son port (<code>http://localhost:8080</code> pour un hôte vide ou
non spécifié), et la nouvelle clé <code>server.base_url</code> porte l’URL
publique quand l’application est derrière un reverse proxy, une terminaison TLS
ou un port de conteneur remappé :

~~~yaml
server:
  host: "0.0.0.0"
  port: 8080
  base_url: "https://api.example.com"
~~~

Les applications qui contournaient le helper en concaténant elles-mêmes un
schéma et un port doivent retirer ce contournement, sous peine d’obtenir une
URL doublée.

<strong>Rupture de compilation possible</strong> : le code qui lisait <code>cfg.Paths.ModelFolder</code>, <code>cfg.Paths.ProjectName</code> ou <code>cfg.Paths.MainFile</code> ne compile plus, et <code>cfg.Server.CORS</code> est désormais un <code>*bool</code> (l’absence de clé et <code>false</code> devaient cesser de se confondre).

Ajoutez une configuration CORS avec au moins une origine :

~~~yaml
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

Si l’application utilise les fichiers, créez aussi <code>configs/files.yaml</code> avec des valeurs globales :

~~~yaml
file_folder: "./public/files/"
max_file_size: 5242880
files: {}
~~~

Consultez la [référence complète](configuration.md). En particulier :

- <code>server.host</code> est une adresse d’écoute brute, sans schéma ni port ;
- <code>server.cors: false</code> coupe tout le traitement CORS ; la clé absente laisse la section <code>cors</code> décider ;
- <code>cors.allow_all_origins</code> est transmis à Gin et gagne sur une liste d’origines simultanée ;
- une section <code>cors</code> sans origine n’installe plus le middleware : la v0.5 activait alors toutes les origines. Un frontend d’une autre origine cesse de fonctionner tant que <code>allowed_origins</code> n’est pas rempli ;
- <code>allowed_headers</code> vide ne prend plus seulement les défauts de Gin : <code>Authorization</code> y est ajouté, sans quoi le login JWT du framework ne pouvait pas fonctionner depuis une autre origine. Une liste écrite reste utilisée telle quelle ;
- <code>cors.max_age</code> n’est plus multiplié par <code>time.Hour</code> et accepte un nombre de secondes : <code>max_age: 12ns</code>, seul contournement de l’ancienne conversion, vaut désormais 12 nanosecondes — écrivez <code>12h</code> ;
- le fournisseur DB reste PostgreSQL et force <code>sslmode=disable</code>.

## 3. Mettre à jour les providers

Avant :

~~~go
cfg := providers.GetConfig()
var typed *providers.Config
~~~

Après :

~~~go
cfg := providers.GetMainConfig()
var typed *providers.MainConfig
~~~

La v2 ajoute le chargement des fichiers dotenv avant l’expansion du YAML. <code>env_files</code> accepte une chaîne ou une liste, mais pas la syntaxe <code>${VAR:-default}</code>.

## 4. Préparer les routes

Chaque fichier route doit rester un plugin Go autonome :

~~~go
package main

import "github.com/gin-gonic/gin"

func GET(c *gin.Context) {
	// Le handler Gin v1 reste valide.
}

func main() {}
~~~

La forme dynamique recommandée devient :

~~~text
api/users/id_/route.go
→ /api/users/:id
~~~

<code>[id]</code> et <code>_id</code> restent supportés pour faciliter une migration progressive.

Vous pouvez ensuite adopter l’injection v2 :

~~~go
package main

import "github.com/goyourt/yogourt"

type Params struct {
	ID uint64 `param:"id"`
}

func GET(c *yogourt.Context, params Params) {
	// ...
}

func main() {}
~~~

Le second argument ne reçoit que les paramètres de chemin. Les query params, headers et bodies restent à lire via Gin ou <code>routing.HandleRequest</code>.

## 5. Migrer les réponses

Avant :

~~~go
routing.RespondSuccess(c, http.StatusOK, data)
~~~

Après :

~~~go
routing.RespondSuccess(c, data)
~~~

Pour les autres statuts :

~~~go
routing.RespondCreated(c, data)
routing.RespondWithContent(c, http.StatusAccepted, "job", job)
routing.RespondNoContent(c)
routing.RespondNotFound(c)
routing.RespondAndAbort(c, http.StatusBadRequest, "invalid request")
~~~

Ces helpers attendent toujours <code>*gin.Context</code>. Depuis <code>*yogourt.Context</code>, utilisez <code>c.Gin()</code>.

## 6. Migrer le middleware

Le chemin attendu est strict :

~~~text
middleware/middleware.go
→ .yogourt/middleware/middleware.go.so
~~~

Structure minimale :

~~~go
package main

import "github.com/gin-gonic/gin"

var Callbacks = map[string]func(*gin.Context){}

func main() {}
~~~

Pour les routes dynamiques, utilisez des clés Gin :

~~~go
var Callbacks = map[string]func(*gin.Context){
	"/api/users/:id": authenticate,
}
~~~

N’utilisez pas <code>/api/users/id_</code> ou <code>/api/users/[id]</code> comme clé de callback.

Rappels :

- les callbacks parents s’empilent ;
- <code>nil</code> efface les callbacks hérités ;
- <code>^</code> remplace les parents.

## 7. Précompiler les plugins

Le runtime ne récupère et ne compile plus un plugin manquant ou périmé. Le chemin de sortie doit reproduire le chemin source :

~~~text
api/users/id_/route.go
→ .yogourt/api/users/id_/route.go.so
~~~

Exemple :

~~~sh
mkdir -p .yogourt/middleware .yogourt/api/users/id_
go build -buildmode=plugin -o .yogourt/middleware/middleware.go.so ./middleware/middleware.go
go build -buildmode=plugin -o .yogourt/api/users/id_/route.go.so ./api/users/id_/route.go
go run .
~~~

Tous les fichiers <code>.go</code> scannés, sauf <code>_test.go</code>, doivent avoir un <code>.so</code>. Reconstruisez l’ensemble après toute modification de :

- version de Go ;
- dépendance directe ou indirecte ;
- source de route ou middleware ;
- tags et options de compilation.

Le CLI stable v0.5 ne fournit pas les commandes v2 <code>build</code>, <code>dev</code> ou <code>start</code> et son <code>init</code> génère encore une structure v1. Utilisez la compilation manuelle tant qu’une nouvelle version n’est pas publiée.

## 8. Adapter les implémentations de fichiers

Les implémentations personnalisées de <code>interfaces.FileInterface</code> doivent désormais fournir :

~~~go
GetType() string
SetType(string)
GetFilePath(folder string) string
~~~

Le type intégré <code>interfaces.File</code> contient aussi un champ <code>Type</code>.

Ne migrez pas le stockage sans prendre en compte les limites actuelles : contenu chargé en mémoire, erreurs d’écriture masquées et extension concaténée sans point au UUID. Voir le [guide des services](services.md#fichiers).

## 9. Réviser les accès DB

La v2 ajoute notamment :

- <code>DataWriter.Upsert</code> ;
- <code>database.Or</code> ;
- la pagination dédupliquée avec <code>Distinct</code> ;
- la clé de filtre <code>orderBy</code> ;
- <code>UpsertRelations</code>.

Les lectures retournent désormais l’erreur GORM : <code>GetOneBy</code> (y compris <code>gorm.ErrRecordNotFound</code>), <code>GetAll</code>, <code>GetAllPaginated</code>, <code>HydrateRelation</code> et <code>HydrateManyToManyRelation</code>. L’ajout d’une valeur de retour ne casse pas les sites d’appel existants, mais vérifiez ces erreurs sur vos chemins critiques.

Pendant la migration :

- utilisez <code>[]*Model</code> pour les helpers génériques ;
- n’acceptez jamais directement un nom de colonne, relation ou ordre fourni par le client ;
- récupérez <code>.Error</code> via <code>SearchQuery</code> ou GORM pour les chemins critiques ;
- encapsulez les écritures composées dans une transaction explicite.

## 10. Réviser JWT et fichiers

La v2 crée toujours des JWT HS256 avec <code>uuid</code> et <code>exp</code>. La validation restreint désormais explicitement l’algorithme à <code>HS256</code>, valide le format du claim <code>uuid</code> et exige un secret de 32 octets minimum, contrôlé dès le démarrage (warning hors production, refus de démarrer en mode <code>production</code>). Elle ne vérifie toujours ni issuer ni audience, et n’exige pas explicitement la présence de <code>exp</code> sur un token qu’elle n’a pas émis.

Avant mise en production :

- configurez un secret aléatoire de 32 octets au moins, sous peine de refus de démarrage ;
- imposez les claims attendus (issuer, audience, expiration obligatoire) dans une couche applicative ;
- vérifiez les types MIME et extensions des uploads ;
- gérez les erreurs d’écriture de fichiers ;
- ajoutez un TTL et une purge au suivi Redis des échecs de mot de passe.

## 11. Valider la migration

Depuis l’application :

1. supprimez de manière contrôlée les anciens artefacts <code>.yogourt</code> ;
2. reconstruisez chaque plugin ;
3. démarrez le serveur depuis la racine du projet ;
4. testez chaque paire méthode/URL, y compris les routes dynamiques ;
5. testez les branches middleware <code>nil</code> et <code>^</code> ;
6. vérifiez CORS depuis les origines autorisées ;
7. testez PostgreSQL, Redis, JWT et uploads dans un environnement isolé.

Depuis le dépôt Yogourt :

~~~sh
go test ./...
go test -race ./...
go vet ./...
~~~

Une migration n’est complète que lorsque le binaire et tous les plugins ont été reconstruits ensemble et que les scénarios d’intégration applicatifs passent.

## Points à surveiller avant le tag stable

- choix final du chemin de module v2 ;
- publication du CLI et de son workflow build/dev/start ;
- validation stricte de la configuration ;
- stratégie de portabilité en dehors des plugins Go ;
- retours d’erreur DB et fichiers ;
- durcissement JWT restant : issuer, audience et expiration obligatoire ;
- détection des collisions de routes.
