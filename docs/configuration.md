# Configuration

Ce guide décrit le contrat observé sur la préversion v2. Les chemins sont résolus depuis le répertoire de lancement de l’application.

## Fichier principal

Le runtime lit exclusivement <code>./configs/yogourt.yaml</code>.

~~~yaml
app_name: "example"
version: "2.0.0"
mode: "development"

env_files:
  - ".env"
  - ".env.local"

server:
  port: 8080
  host: "127.0.0.1"
  cors: true
  base_path: "/api"

database:
  type: "postgres"
  user: "${DB_USER}"
  password: "${DB_PASSWORD}"
  host: "${DB_HOST}"
  port: 5432
  db: "${DB_NAME}"

cache:
  host: "${REDIS_HOST}"
  port: "6379"
  password: "${REDIS_PASSWORD}"
  db: 0

paths:
  route_folder: "api"

security:
  secret_key: "${JWT_SECRET}"
  hash_cost: 12
  token_expires: 60
  password_minimum_length: 12
  password_special_char_required: true
  password_number_required: true
  password_upper_case_required: true
  password_lower_case_required: true

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
  allow_credentials: true
  allow_all_origins: false
~~~

Le champ <code>cors.max_age</code> accepte un nombre de secondes (<code>3600</code>) ou une durée (<code>12h</code>, <code>300ms</code>). Omis, il vaut <code>0</code> et l’en-tête <code>Access-Control-Max-Age</code> n’est pas envoyé.

## Référence des champs

### Application et environnement

| Champ | Type | Comportement actuel |
| --- | --- | --- |
| <code>app_name</code> | chaîne | Métadonnée applicative |
| <code>version</code> | chaîne | Version journalisée au démarrage, avec <code>app_name</code> |
| <code>mode</code> | chaîne | Mode applicatif. <code>production</code> rend fatal au démarrage un <code>security.secret_key</code> vide ou trop court et met Gin en mode release ; <code>test</code> met Gin en mode test ; toute autre valeur le laisse en debug. Une variable d’environnement <code>GIN_MODE</code> explicite reste prioritaire |
| <code>env_files</code> | chaîne ou liste | Fichiers dotenv chargés avant l’expansion du YAML |

### Serveur

| Champ | Type | Comportement actuel |
| --- | --- | --- |
| <code>server.port</code> | entier | Port d’écoute |
| <code>server.host</code> | chaîne | Adresse d’écoute ; <code>0.0.0.0</code> si la valeur est vide |
| <code>server.cors</code> | booléen | Interrupteur CORS. <code>false</code> retire le middleware et la réponse au préflight ; clé absente ou <code>true</code> laisse la section <code>cors</code> décider |
| <code>server.base_path</code> | chaîne | Préfixe HTTP de toutes les routes ; <code>/api</code> si la valeur est vide |
| <code>server.base_url</code> | chaîne | URL publique retournée par <code>services.GetBaseUrl</code> ; reconstruite depuis <code>server.host</code> et <code>server.port</code> si la valeur est vide |

<code>server.host</code> est un nom d’hôte ou une adresse IP sans schéma ni port, par exemple <code>127.0.0.1</code>, <code>localhost</code> ou <code>::1</code>. Le port vient de <code>server.port</code>.

<code>server.host</code> est une adresse d’écoute, pas une adresse joignable :
<code>0.0.0.0</code> demande au socket d’accepter toutes les interfaces et ne dit
rien de l’URL par laquelle un client atteint l’application.
<code>server.base_url</code> porte donc cette URL publique, la seule que le
processus ne peut pas deviner — reverse proxy, terminaison TLS, port de
conteneur remappé :

~~~yaml
server:
  port: 8080
  host: "0.0.0.0"
  base_url: "https://api.example.com"
~~~

Sans <code>server.base_url</code>, <code>services.GetBaseUrl</code> reconstruit
l’URL depuis l’adresse d’écoute : schéma <code>http</code>, port inclus, et un
hôte vide ou non spécifié (<code>0.0.0.0</code>, <code>::</code>) remplacé par
<code>localhost</code>. Une valeur écrite sans schéma est servie en
<code>http</code>, et le slash final est retiré.

<code>server.cors: false</code> coupe tout le traitement CORS : aucun en-tête, et le
<code>OPTIONS /*path</code> qui répondait <code>204</code> à chaque préflight n’est plus
enregistré — les requêtes <code>OPTIONS</code> atteignent alors les routes, ou un 404.
Une section <code>cors</code> renseignée en même temps est signalée au démarrage, parce
qu’elle ne sert plus à rien. Une clé absente conserve le comportement
historique : c’est la section <code>cors</code> qui décide.

<code>server.base_path</code> déplace toutes les URLs publiées. Il accepte plusieurs
segments (<code>/api/v2</code>), tolère la forme sans slash (<code>v1</code>) et la barre
oblique finale, et <code>/</code> sert l’arborescence à la racine
(<code>api/users/route.go</code> répond alors sur <code>/users</code>). Un paramètre Gin
ou une espace dans le préfixe arrête le démarrage. <code>routing.WithPrefix</code>
l’emporte sur ce champ, voir le [guide de routage](routing.md).

### Base de données

| Champ | Type | Comportement actuel |
| --- | --- | --- |
| <code>database.type</code> | chaîne | Contrôlé au démarrage : <code>postgres</code>, <code>postgresql</code> ou vide ; toute autre valeur arrête le processus |
| <code>database.user</code> | chaîne | Utilisateur PostgreSQL |
| <code>database.password</code> | chaîne | Mot de passe PostgreSQL |
| <code>database.host</code> | chaîne | Hôte PostgreSQL |
| <code>database.port</code> | entier | Port PostgreSQL |
| <code>database.db</code> | chaîne | Nom de base ; remplace l’ancienne clé <code>dbname</code> |
| <code>database.ssl_mode</code> | chaîne | Mode TLS libpq : <code>disable</code>, <code>allow</code>, <code>prefer</code>, <code>require</code>, <code>verify-ca</code>, <code>verify-full</code> ; <code>disable</code> si la valeur est vide. Toute autre valeur arrête le processus |
| <code>database.ssl_root_cert</code> | chaîne | Chemin du certificat d’autorité vérifié par <code>verify-ca</code> et <code>verify-full</code> |
| <code>database.ssl_cert</code> | chaîne | Chemin du certificat client |
| <code>database.ssl_key</code> | chaîne | Chemin de la clé privée du certificat client |
| <code>database.search_path</code> | chaîne | Chemin de recherche des schémas de chaque session ; défaut du serveur si la valeur est vide |
| <code>database.pool.max_open_conns</code> | entier | Connexions ouvertes simultanées ; illimité si <code>0</code> |
| <code>database.pool.max_idle_conns</code> | entier | Connexions inactives conservées ; défaut de <code>database/sql</code> (2) si <code>0</code> |
| <code>database.pool.conn_max_lifetime</code> | durée | Âge maximal d’une connexion ; sans limite si <code>0</code> |
| <code>database.pool.conn_max_idle_time</code> | durée | Durée d’inactivité maximale d’une connexion ; sans limite si <code>0</code> |

Le fournisseur ne construit qu’une connexion PostgreSQL. Le DSN n’impose plus
<code>sslmode=disable</code> : c’est la valeur par défaut de
<code>database.ssl_mode</code>, celle d’une configuration qui n’a jamais écrit la
clé, et un déploiement exigeant TLS la remplace.

~~~yaml
database:
  type: "postgres"
  host: "${DB_HOST}"
  port: 5432
  user: "${DB_USER}"
  password: "${DB_PASSWORD}"
  db: "${DB_NAME}"
  ssl_mode: "verify-full"
  ssl_root_cert: "/etc/ssl/certs/db-ca.crt"
  search_path: "app,public"
  pool:
    max_open_conns: 25
    max_idle_conns: 5
    conn_max_lifetime: 30m
    conn_max_idle_time: 5m
~~~

Les mots-clés facultatifs ne sont écrits dans le DSN que lorsqu’ils portent une
valeur, ce qui laisse à libpq ses propres défauts — notamment ses emplacements
habituels de certificats. Les valeurs sont échappées : un mot de passe
contenant une espace ou une apostrophe ne tronque plus la chaîne de connexion.

Les deux durées du pool s’écrivent en chaîne (<code>30m</code>, <code>90s</code>)
ou en nombre de secondes (<code>90</code>). Un champ à zéro conserve le défaut de
<code>database/sql</code>.

Un échec de connexion est retourné, jamais fatal :
<code>providers.GetDB()</code> et <code>providers.InitDB()</code> rendent
<code>(*gorm.DB, error)</code>. La connexion n’est mémorisée qu’une fois
ouverte, donc une base indisponible au premier appel ne condamne pas le
processus : l’appel suivant retente.

### Cache

| Champ | Type | Comportement actuel |
| --- | --- | --- |
| <code>cache.host</code> | chaîne | Hôte Redis |
| <code>cache.port</code> | chaîne | Port Redis, volontairement typé comme chaîne |
| <code>cache.password</code> | chaîne | Mot de passe Redis |
| <code>cache.db</code> | entier | Index de base Redis |

Le client est créé à la première utilisation de <code>providers.GetCache()</code>, qui
n'est mémorisé qu'après un <code>PING</code> réussi. Une instance injoignable est donc
signalée par une erreur au lieu d'être découverte à la première opération de cache,
et l'appel suivant retente la connexion. La connexion et le <code>PING</code> sont bornés
à 2 secondes.

### Chemins

| Champ | Type | Comportement actuel |
| --- | --- | --- |
| <code>paths.route_folder</code> | chaîne | Dossier du plan de routes chargé par <code>routing.Initialize</code> |

<code>paths.route_folder</code> est la seule source du dossier de routes :
<code>routing.Initialize</code> ne prend pas de dossier en argument, parce que le
dossier scanné par un déploiement est un réglage et que le déclarer à deux
endroits permettait à un programme de contredire son propre fichier de
configuration. Sans <code>route_folder</code>, le démarrage échoue en nommant la
clé.

Le dossier scanné ne change jamais l’URL publiée : le préfixe HTTP est réglé
séparément par <code>server.base_path</code> ou <code>routing.WithPrefix</code>.

<code>model_folder</code>, <code>project_name</code> et <code>main_file</code> ne sont plus
des champs de la configuration : ils appartiennent au CLI v0.5 et le runtime ne
les a jamais lus. Les écrire produit un avertissement (ci-dessous) ; les
supprimer ne change rien au comportement du serveur.

### Clés sans effet

Toute clé absente des tableaux ci-dessus est ignorée par le parseur YAML. Depuis
la v2, le chargement de la configuration les signale au lieu de les avaler en
silence :

<pre><code>⚠️  configs/yogourt.yaml: "paths.api_folder" was renamed to "paths.route_folder" — the value is ignored
⚠️  configs/yogourt.yaml: "paths.model_folder" is not a runtime setting (only the v0.5 CLI reads it) — delete it
⚠️  configs/yogourt.yaml: unknown key "tpyo" — the value is ignored
</code></pre>

Trois familles d’avertissements :

- une clé <strong>inconnue</strong> (faute de frappe, section inventée) ;
- une clé <strong>renommée</strong> depuis les gabarits du CLI v0.5 —
  <code>paths.api_folder</code> → <code>paths.route_folder</code>,
  <code>database.dbname</code> → <code>database.db</code> — dont le remplaçant est
  nommé dans le message ;
- une clé <strong>retirée</strong> du contrat runtime :
  <code>paths.model_folder</code>, <code>paths.project_name</code>,
  <code>paths.main_file</code>.

Tous les champs que la configuration déclare encore sont lus par le framework :
il n’y a plus de clé « analysée mais ignorée ». <code>configs/files.yaml</code> passe
par le même contrôle, mais seules les clés inconnues y sont signalées : les
listes de clés renommées et retirées ne concernent que <code>yogourt.yaml</code>.

Ces avertissements ne bloquent jamais le démarrage : une clé inconnue a toujours
été tolérée ici, en faire une erreur casserait des configurations qui démarrent
aujourd’hui.

### Sécurité

| Champ | Type | Unité ou effet |
| --- | --- | --- |
| <code>security.secret_key</code> | chaîne | Secret de signature JWT ; 32 octets minimum exigés |
| <code>security.hash_cost</code> | entier | Coût bcrypt ; 12 si la valeur est 0 |
| <code>security.token_expires</code> | entier | Durée de vie du token, en minutes |
| <code>security.password_minimum_length</code> | entier | Longueur minimale, en octets ; <code>0</code> n’exige aucune longueur |
| <code>security.password_special_char_required</code> | booléen | Exige un caractère Unicode de ponctuation ou symbole |
| <code>security.password_number_required</code> | booléen | Exige un chiffre Unicode |
| <code>security.password_upper_case_required</code> | booléen | Exige une majuscule Unicode |
| <code>security.password_lower_case_required</code> | booléen | Exige une minuscule Unicode |

Ces cinq champs ne sont lus que par <code>services.IsPasswordValid</code>, que le framework n’appelle jamais : la route d’inscription ou de changement de mot de passe doit l’appeler elle-même. Les drapeaux sont indépendants et cumulatifs ; à <code>false</code> ou absents, le contrôle correspondant n’a pas lieu. Un mot de passe vide est toujours refusé, mais sans aucune de ces clés c’est le seul cas rejeté.

Utilisez un secret JWT long, aléatoire et non vide : <code>security.secret_key</code> est validé. Un secret vide ou de moins de 32 octets est journalisé en warning au démarrage hors production et refuse le démarrage lorsque <code>mode</code> vaut <code>production</code> ; les services de token échouent de leur côté tant qu’il n’est pas corrigé.

### CORS

| Champ | Type | Comportement actuel |
| --- | --- | --- |
| <code>cors.allowed_origins</code> | liste | Liste blanche d’origines, schéma obligatoire ; alimente <code>Access-Control-Allow-Origin</code> |
| <code>cors.allowed_methods</code> | liste | Méthodes autorisées ; les valeurs par défaut de Gin si la liste est vide |
| <code>cors.allowed_headers</code> | liste | <code>Access-Control-Allow-Headers</code> ; si la liste est vide, les valeurs par défaut de Gin plus <code>Authorization</code>. Une liste écrite est reprise telle quelle |
| <code>cors.allow_credentials</code> | booléen | <code>true</code> émet <code>Access-Control-Allow-Credentials: true</code> : le navigateur peut joindre cookies et en-têtes d’authentification à une requête d’origine croisée. Sinon l’en-tête n’est pas émis et le navigateur retire ces éléments |
| <code>cors.allow_all_origins</code> | booléen | <code>true</code> répond <code>Access-Control-Allow-Origin: *</code> à toute origine et ignore <code>allowed_origins</code> ; suffit à lui seul à installer le middleware |
| <code>cors.max_age</code> | secondes ou durée | <code>Access-Control-Max-Age</code> : durée de mise en cache du préflight ; un nombre nu est lu en secondes, <code>0</code> n’émet pas l’en-tête |

Toute cette section est subordonnée à <code>server.cors</code> : avec <code>server.cors: false</code>, elle est ignorée et le démarrage le signale. Si <code>allowed_origins</code> est vide et <code>allow_all_origins</code> faux, le middleware CORS n’est pas installé : aucune en-tête CORS n’est émise et le navigateur refuse les appels d’origine croisée. Si <code>allow_all_origins</code> vaut <code>true</code>, ce mode gagne et une éventuelle liste est ignorée afin d’éviter une configuration Gin conflictuelle. Pour les applications avec cookies ou credentials, utilisez une liste d’origines explicite : un navigateur rejette une réponse credentialed portant <code>Access-Control-Allow-Origin: *</code>, et le démarrage l’indique par un avertissement.

## Variables d’environnement

<code>env_files</code> accepte une chaîne :

~~~yaml
env_files: ".env"
~~~

ou une liste :

~~~yaml
env_files:
  - ".env"
  - ".env.local"
~~~

Le chargement suit ces règles :

1. les fichiers dotenv sont chargés dans l’ordre déclaré ;
2. une variable déjà présente dans l’environnement du processus n’est pas écrasée ;
3. le YAML brut est ensuite traité par <code>os.ExpandEnv</code> ;
4. une variable absente est remplacée par une chaîne vide.

Seule la forme <code>${NAME}</code> est prise en charge. La syntaxe de valeur par défaut <code>${NAME:-default}</code> n’est pas supportée.

Les chemins des fichiers dotenv sont interprétés depuis le répertoire de lancement. Un fichier déclaré mais absent empêche le chargement de la configuration.

## Configuration des fichiers

Les services de fichiers lisent exclusivement <code>./configs/files.yaml</code>.

~~~yaml
file_folder: "./public/files/"
max_file_size: 5242880

files:
  avatars:
    file_folder: "./public/avatars/"
    max_file_size: 2097152
  documents:
    max_file_size: 10485760
~~~

Les tailles sont exprimées en octets. Une catégorie hérite des valeurs globales qu’elle n’écrase pas.

Définissez toujours <code>file_folder</code> et <code>max_file_size</code> au niveau global. Les services d’accès déréférencent ces valeurs et une configuration sans valeur de repli peut provoquer un panic.

Le chargement dotenv est global et ne s’exécute qu’une fois. Dans le flux normal, <code>configs/yogourt.yaml</code> est chargé en premier : déclarez donc les fichiers d’environnement dans le fichier principal. Si <code>configs/files.yaml</code> est exceptionnellement la première configuration lue, il peut déclarer son propre <code>env_files</code> ; sinon le fournisseur recherche ceux du fichier principal.

## Accès depuis Go

~~~go
cfg := providers.GetMainConfig()
fmt.Println(cfg.AppName, cfg.Server.Port)

fileCfg := providers.GetConfigByFileType("avatars")
fmt.Println(*fileCfg.FileFolder, *fileCfg.MaxFileSize)
~~~

Les fournisseurs sont des singletons chargés une seule fois. Une erreur de lecture ou de parsing provoque un panic ; une modification du YAML pendant l’exécution n’est pas rechargée.

## Limites actuelles

- les clés YAML inconnues sont signalées au démarrage, mais jamais fatales : la valeur reste ignorée ;
- aucune validation globale n’empêche des valeurs vides ou incohérentes, à l’exception de <code>security.secret_key</code>, <code>database.type</code>, <code>database.ssl_mode</code>, <code>server.base_path</code> et <code>cors.max_age</code>, contrôlés au démarrage ;
- rien n’est rechargé à chaud, et les chemins des deux fichiers sont codés en dur.
