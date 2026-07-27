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
  model_folder: "models"
  project_name: "example"
  main_file: "main.go"
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

Le champ <code>cors.max_age</code> est volontairement omis. L’implémentation multiplie directement la valeur YAML décodée par <code>time.Hour</code> : une valeur numérique <code>1</code> produit une heure, tandis qu’une durée textuelle déjà exprimée comme <code>1h</code> serait multipliée une seconde fois.

## Référence des champs

### Application et environnement

| Champ | Type | Comportement actuel |
| --- | --- | --- |
| <code>app_name</code> | chaîne | Métadonnée applicative |
| <code>version</code> | chaîne | Métadonnée de version |
| <code>mode</code> | chaîne | Métadonnée de mode |
| <code>env_files</code> | chaîne ou liste | Fichiers dotenv chargés avant l’expansion du YAML |

### Serveur

| Champ | Type | Comportement actuel |
| --- | --- | --- |
| <code>server.port</code> | entier | Port d’écoute |
| <code>server.host</code> | chaîne | Adresse d’écoute ; <code>0.0.0.0</code> si la valeur est vide |
| <code>server.cors</code> | booléen | Analysé mais non utilisé ; le middleware CORS est toujours installé |

<code>server.host</code> est un nom d’hôte ou une adresse IP sans schéma ni port, par exemple <code>127.0.0.1</code>, <code>localhost</code> ou <code>::1</code>. Le port vient de <code>server.port</code>.

### Base de données

| Champ | Type | Comportement actuel |
| --- | --- | --- |
| <code>database.type</code> | chaîne | Analysé mais ignoré |
| <code>database.user</code> | chaîne | Utilisateur PostgreSQL |
| <code>database.password</code> | chaîne | Mot de passe PostgreSQL |
| <code>database.host</code> | chaîne | Hôte PostgreSQL |
| <code>database.port</code> | entier | Port PostgreSQL |
| <code>database.db</code> | chaîne | Nom de base ; remplace l’ancienne clé <code>dbname</code> |

Le fournisseur actuel utilise uniquement PostgreSQL et ajoute <code>sslmode=disable</code> au DSN. Il faut donc l’adapter avant un déploiement qui exige TLS.

### Cache

| Champ | Type | Comportement actuel |
| --- | --- | --- |
| <code>cache.host</code> | chaîne | Hôte Redis |
| <code>cache.port</code> | chaîne | Port Redis, volontairement typé comme chaîne |
| <code>cache.password</code> | chaîne | Mot de passe Redis |
| <code>cache.db</code> | entier | Index de base Redis |

Le client est créé à la première utilisation de <code>providers.GetCache()</code>.

### Chemins

| Champ | Type | Comportement actuel |
| --- | --- | --- |
| <code>paths.model_folder</code> | chaîne | Analysé pour les outils et conventions de projet |
| <code>paths.project_name</code> | chaîne | Analysé pour les outils et conventions de projet |
| <code>paths.main_file</code> | chaîne | Analysé pour les outils et conventions de projet |
| <code>paths.route_folder</code> | chaîne | Analysé, mais <code>routing.Initialize</code> utilise son propre argument |

Le préfixe d’URL reste <code>/api</code>, même si un autre dossier est fourni à <code>routing.Initialize</code>.

### Sécurité

| Champ | Type | Unité ou effet |
| --- | --- | --- |
| <code>security.secret_key</code> | chaîne | Secret de signature JWT |
| <code>security.hash_cost</code> | entier | Coût bcrypt ; 12 si la valeur est 0 |
| <code>security.token_expires</code> | entier | Durée de vie du token, en minutes |
| <code>security.password_minimum_length</code> | entier | Longueur minimale |
| <code>security.password_special_char_required</code> | booléen | Exige un caractère Unicode de ponctuation ou symbole |
| <code>security.password_number_required</code> | booléen | Exige un chiffre Unicode |
| <code>security.password_upper_case_required</code> | booléen | Exige une majuscule Unicode |
| <code>security.password_lower_case_required</code> | booléen | Exige une minuscule Unicode |

Utilisez un secret JWT long, aléatoire et non vide. La préversion ne valide pas sa robustesse.

### CORS

| Champ | Type | Comportement actuel |
| --- | --- | --- |
| <code>cors.allowed_origins</code> | liste | Origines transmises à Gin CORS |
| <code>cors.allowed_methods</code> | liste | Méthodes autorisées |
| <code>cors.allowed_headers</code> | liste | En-têtes autorisés |
| <code>cors.allow_credentials</code> | booléen | Autorise les credentials |
| <code>cors.allow_all_origins</code> | booléen | Active <code>AllowAllOrigins</code> dans Gin CORS |
| <code>cors.max_age</code> | nombre ou durée | Valeur décodée multipliée par <code>time.Hour</code> ; à omettre avant correction |

Si <code>allowed_origins</code> est vide, le runtime active automatiquement toutes les origines. Si <code>allow_all_origins</code> vaut <code>true</code>, ce mode gagne et une éventuelle liste est ignorée afin d’éviter une configuration Gin conflictuelle. Pour les applications avec cookies ou credentials, utilisez une liste d’origines explicite plutôt que le mode global.

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

- les clés YAML inconnues sont silencieusement ignorées ;
- aucune validation globale n’empêche des valeurs vides ou incohérentes ;
- <code>server.cors</code> ne désactive pas CORS ;
- <code>cors.max_age</code> est mal converti ;
- <code>database.type</code> est ignoré et <code>sslmode=disable</code> est forcé ;
- les erreurs de connexion PostgreSQL terminent le processus via <code>log.Fatal</code>.
