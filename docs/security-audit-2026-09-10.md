# Audit de sécurité Yogourt — 10 septembre 2026

> État historique au moment de l’audit. Les correctifs et leur validation sont
> consignés dans le [suivi du 12 septembre](security-remediation-2026-09-12.md).

Finalisé le 11 septembre 2026 après reprise ; les empreintes des sources confirment que l'état audité n'a pas changé depuis la revue et le scan du 10 septembre.

Le code présente **13 constats prioritaires : 5 de gravité élevée, 7 moyenne et 1 faible**. Les priorités sont de corriger le langage de filtres SQL, mettre à jour la chaîne Go/dépendances, borner les uploads avant leur parsing et fiabiliser le compteur d'échecs de connexion. L'exposition de plusieurs défauts dépend de l'intégration applicative ; ces conditions sont précisées ci-dessous.

Le moteur d'autorisation présente une base solide : refus par défaut, scopes explicites, erreurs traitées comme des refus, RBAC avant ABAC, restrictions combinées et cache par requête. Cela ne suffit pas à qualifier l'ensemble du framework de prêt pour une production sensible avant correction des points élevés applicables.

## Périmètre et méthode

- État audité : branche `feature/v2`, commit `1464dd4a5e586b31f59f143c1036106da4ef0244`, **avec les modifications locales et fichiers nouveaux préexistants**. Il ne s'agit pas du seul commit publié ni de la version stable v1.
- Revue du routage, plugins et middleware, authentification/JWT, mots de passe, RBAC/ABAC, providers mémoire et GORM, CRUD et relations, fichiers, configuration, migrations, tests et documentation. Inventaire de 116 fichiers avant création du rapport ; empreintes dans l'annexe JSON.
- Analyse statique manuelle avec contre-revue indépendante des autorisations et des filtres SQL ; exécution des tests existants, détection de courses, `go vet`, intégrité des modules et `govulncheck`.
- Recherche locale de signatures de clés privées et de jetons AWS, GitHub, Slack, Google et Stripe, dans l'état courant et les lignes ajoutées de l'historique Git accessible. Aucun résultat pour ces signatures. Ce contrôle limité ne prouve pas l'absence de tout secret ; les valeurs de configuration de test ne sont pas des secrets de production vérifiés.
- Aucun endpoint déployé, compte réel, infrastructure cloud ou service de production audité. Aucun scénario d'exploitation ni test de charge exécuté. Les constats de code reposent sur leurs chemins d'exécution et les contrats des dépendances, sans reproduction offensive.
- Aucun correctif applicatif, commit, push ou migration de production effectué. Les fichiers produits sont ce rapport et ses annexes.

Les gravités sont qualitatives : elles prennent en compte l'impact et les préconditions, sans score CVSS artificiellement précis pour une application dont le déploiement n'est pas connu. « Confirmé » désigne le comportement du code ; « conditionnel » désigne son exposition dans une application.

## Résultats des vérifications

| Vérification | Résultat |
| --- | --- |
| `go test ./...` | Réussi sur Go 1.24.0, macOS arm64 |
| `go test -race ./...` | Réussi ; avertissements du linker macOS `LC_DYSYMTAB`, sans échec ni course signalée |
| `go vet ./...` | Réussi |
| `go mod verify` | Tous les modules vérifiés |
| Intégration de vrais plugins | Réussie : 5 plugins compilés, chargement et contrôles de permissions validés par `TestPluginRoutesEndToEnd` |
| PostgreSQL | Réussi avec `-count=1 -race` pour `authorization/gormstore` et `services/database`, sur PostgreSQL 15.16 temporaire via socket Unix privé ; instance arrêtée après les tests |
| `govulncheck v1.8.0 -json ./...` | 33 avis au niveau symbole, 28 au niveau paquet uniquement, 25 au niveau module uniquement |
| Recherche de secrets par signatures | Aucun résultat sur les signatures recherchées, état courant et historique local |

Les tests existants qui passent ne constituent pas des tests de non-régression pour les nouveaux constats. Les critères proposés ci-dessous restent à implémenter avec les correctifs.

Les premières suites générales ignoraient les tests PostgreSQL faute de `YOGOURT_TEST_DSN`. La validation dédiée finale les a exécutés sur une instance créée uniquement pour l'audit. Les restrictions du bac à sable ont nécessité une autorisation de mémoire partagée et de socket local ; leurs échecs initiaux étaient environnementaux, puis la relance complète a réussi. Les tests réinitialisent leurs tables : cette configuration temporaire ne doit pas être remplacée par une base applicative existante.

## Constats prioritaires

| ID | Gravité | Constat | Condition principale |
| --- | --- | --- | --- |
| YSEC-01 | Élevée | Des valeurs de filtres deviennent des opérateurs | Données utilisateur transmises aux helpers de recherche |
| YSEC-02 | Élevée | Identifiants et tris SQL concaténés | Noms de filtres, relations ou tris non maîtrisés |
| YSEC-03 | Élevée | Chaîne Go et dépendances vulnérables | Binaire compilé avec les versions auditées ; préconditions propres aux avis |
| YSEC-04 | Élevée | Taille d'upload contrôlée après parsing | Route multipart sans plafond de corps préalable |
| YSEC-05 | Élevée | Échecs de mot de passe sous-comptés | Limitation applicative basée sur ce compteur |
| YSEC-06 | Moyenne | Cache de grants partagé entre moteurs | Plusieurs moteurs/providers dans le même contexte |
| YSEC-07 | Moyenne | Politique de claims JWT incomplète | Jetons d'autres usages/émetteurs utilisant la même clé |
| YSEC-08 | Moyenne | Serveur HTTP sans délais explicites | Exposition sans protection amont suffisante |
| YSEC-09 | Moyenne | Tous les proxies implicitement approuvés | En-têtes IP clients atteignant Gin |
| YSEC-10 | Moyenne | Transport DB/cache insuffisamment protégé | Connexions sur un réseau non fiable |
| YSEC-11 | Moyenne | Paramètres SQL journalisés par défaut | Erreurs/lenteurs et accès aux logs |
| YSEC-12 | Moyenne | Historique des échecs Redis sans expiration | Usage prolongé ou nombreuses identités |
| YSEC-13 | Faible | Refus anonymes absents des hooks de décision | Observabilité fondée sur `WithDecisionHook` |

### YSEC-01 — Les valeurs ne restent pas des données

**Preuves :** [relations.go](../services/database/relations.go), lignes 230–268 et 31–45 ; [queries.go](../services/database/queries.go), lignes 75–89 ; [authService.go](../services/authService.go), ligne 43.

Le constructeur reconnaît des opérateurs de recherche directement dans les chaînes et certaines slices. Une valeur ordinaire peut ainsi changer une égalité en recherche élargie ou ajouter un `OR` au `WHERE` commun. Une liste fermée de noms de colonnes ne protège pas contre ce comportement.

Si une application mélange dans cette map des filtres utilisateur et une contrainte serveur de tenant/propriétaire, leur conjonction n'est plus garantie. L'ordre non déterministe de la map rend aussi les expressions mixtes instables. La lecture du constructeur et du code GORM confirme l'absence de groupe protégeant les contraintes d'accès.

Les recherches d'identité réutilisent ce mécanisme : `Authenticate`, l'hydratation et les upserts de relations passent par `GetOneBy`. Le contrat v2 accepte une identité publique opaque. Une recherche d'identité devrait toujours rester une égalité stricte. L'impact JWT exige néanmoins un jeton correctement signé, une voie d'émission acceptant l'identité concernée et un type de colonne compatible : aucune fabrication de jeton sans clé n'est établie.

**Correction :** remplacer les conventions dans les chaînes par des types explicites d'opérateurs ; traiter les valeurs ordinaires comme des égalités ; isoler les filtres métier dans un groupe et appliquer les contraintes d'accès par une conjonction extérieure. Fournir une recherche d'identité dédiée, indépendante du langage de recherche.

**Critère de validation :** les valeurs restent littérales ; ajouter un filtre métier ne peut jamais élargir le périmètre serveur ; une recherche d'identité ne retourne que l'identité exacte.

### YSEC-02 — Construction SQL non sûre des noms et des tris

**Preuves :** [relations.go](../services/database/relations.go), lignes 36–42, 271–296. La version GORM installée traite les chaînes passées à `Order` comme du SQL brut.

`formatAlias` concatène des identifiants sans validation de schéma ni échappement complet. Les expressions de tri sont transmises directement à GORM. Les paramètres liés protègent les valeurs SQL, mais pas ces fragments. Une injection SQL est possible si l'application transmet des noms ou tris contrôlés par l'appelant. Le dépôt du framework ne fournit pas d'endpoint métier permettant d'affirmer une exposition universelle.

La [documentation des services](services.md), ligne 124, impose déjà une liste applicative fermée ; il s'agit donc d'une limite connue que l'API ne fait pas respecter. [GORM documente également les méthodes concernées](https://gorm.io/docs/security.html).

**Correction :** résoudre colonnes et relations contre le schéma et une liste de champs autorisés ; employer des identifiants structurés GORM et une direction de tri énumérée ; refuser tout nom inconnu avant génération SQL.

**Critère de validation :** seuls les champs et directions déclarés peuvent participer à une requête ; aucun fragment de nom fourni par le client ne devient du SQL brut.

### YSEC-03 — Mise à jour de la chaîne de compilation prioritaire

**Preuves :** [go.mod](../go.mod), outil local `go1.24.0 darwin/arm64` et [inventaire des avis](security-audit-2026-09-10/dependencies.csv).

Le scan source utilise effectivement **Go 1.24.0**, même si govulncheck lui-même a nécessité le téléchargement isolé de Go 1.26.8 pour sa compilation. Les versions de Yogourt n'ont pas été changées. La base consultée était datée du `2026-09-10T14:48:42Z`.

Parmi les 33 avis associés à des symboles du graphe, 31 concernent la bibliothèque standard, 1 `pgx/v5` et 1 `x/text`. Ce sont des candidats d'analyse statique, **pas 33 exploitations confirmées**. L'analyse d'une bibliothèque est conservatrice ; des chemins dépendent d'options, du système d'exploitation ou d'un usage absent de l'application finale. Le code retour nul de la sortie JSON ne signifie pas absence d'avis.

| Avis prioritaire | Qualification | Correction de l'avis |
| --- | --- | --- |
| [GO-2025-3563](https://pkg.go.dev/vuln/GO-2025-3563), CVE-2025-22871 | Parsing HTTP ; risque de désynchronisation conditionné au comportement d'un serveur intermédiaire | Go 1.24.2 à l'époque ; migrer aujourd'hui vers une branche maintenue |
| [GO-2026-4341](https://pkg.go.dev/vuln/GO-2026-4341), CVE-2025-61726 | Consommation mémoire du parsing de paramètres URL ; fonction dans le graphe | Go 1.24.12 à l'époque ; branche maintenue recommandée |
| [GO-2026-5004](https://pkg.go.dev/vuln/GO-2026-5004), CVE-2026-41889 | `pgx` : concerne le protocole simple, non activé par défaut, et une forme particulière de requête ; exposition non démontrée dans le provider standard | `pgx/v5 >= v5.9.2` |
| [GO-2026-5970](https://pkg.go.dev/vuln/GO-2026-5970), CVE-2026-56852 | `x/text` : boucle de normalisation ; chemins via `dairy.ToTitle` et dépendances DB ; contrôle effectif des entrées à vérifier dans l'application | `x/text >= v0.39.0` |

**Correction :** migrer vers une version Go maintenue et corrigée, par exemple Go 1.26.8 ou 1.27.1 au jour de l'audit, puis mettre à jour les dépendances compatibles et rescanner. Go 1.24 ne fait plus partie des deux branches maintenues selon la [politique officielle et l'historique des versions](https://go.dev/doc/devel/release). Reconstruire **le binaire et tous les plugins** avec une chaîne identique.

L'annexe répertorie les 86 avis distincts, leur niveau de détection, leurs versions et leurs liens. Les 25 avis au seul niveau module, notamment SSH dans `x/crypto`, ne sont pas présentés comme des usages vulnérables prouvés du framework. Certains avis, comme celui du paquet OpenPGP non maintenu, n'ont pas de version corrigée indiquée : supprimer l'usage concerné s'il existe dans une application, sans conclure que bcrypt est affecté.

**Critère de validation :** scan du binaire et des sources de chaque application après mise à jour ; aucun avis applicable laissé sans correction ou justification documentée ; chargement réel des plugins reconstruits.

### YSEC-04 — Le plafond d'upload arrive trop tard

**Preuves :** [fileService.go](../services/fileService.go), lignes 72, 79 et 88.

`Request.FormFile` déclenche le parsing multipart avant le contrôle de `fileHeader.Size`. La limite de relecture du fichier sélectionné intervient ensuite. La totalité du corps et les autres parties peuvent donc déjà avoir consommé mémoire et fichiers temporaires. Une limite de mémoire multipart n'est pas une limite de taille totale. Voir le [contrat Go de ParseMultipartForm](https://pkg.go.dev/net/http#Request.ParseMultipartForm).

**Correction :** poser un plafond total via `http.MaxBytesReader` avant tout parser ; borner les parties et fichiers ; privilégier le traitement progressif et maîtriser les fichiers temporaires. Conserver aussi le contrôle individuel déjà présent.

**Critère de validation :** une requête dépassant le quota total est interrompue avant consommation complète ; plusieurs fichiers ne peuvent pas dépasser le budget cumulé.

### YSEC-05 — Le compteur de tentatives fusionne des échecs

**Preuves :** [passwordService.go](../services/passwordService.go), lignes 56–61 et 70–75.

Le membre du sorted set Redis est un horodatage à la seconde. Des échecs survenant dans la même seconde pour un utilisateur mettent à jour le même membre au lieu d'ajouter des tentatives. Un verrouillage basé sur ce nombre sous-estime donc les échecs. Le framework fournit des helpers de comptage, sans admission ni limitation atomique autonome. Le comportement d'un membre existant est défini par [Redis ZADD](https://redis.io/docs/latest/commands/zadd/).

**Correction :** utiliser un compteur atomique avec expiration, ou un identifiant unique par événement ; faire respecter le budget de tentatives atomiquement ; combiner les dimensions compte et origine selon la politique métier.

**Critère de validation :** chaque événement concurrent est compté exactement une fois et la décision d'admission ne peut pas être dépassée par une course entre lecture et incrément.

### YSEC-06 — Cache non isolé entre moteurs d'autorisation

**Preuves :** [cache.go](../authorization/cache.go), lignes 16 et 124–143 ; [ginmw.go](../authorization/ginmw/ginmw.go), ligne 20.

La clé du cache ne contient que l'identité et le scope, sans moteur/provider. Deux moteurs utilisant le même contexte peuvent donc partager les grants du premier sans interroger la source d'autorité du second. L'API `MiddlewareFor` permet cette composition. Le chemin habituel de Yogourt publie un moteur unique : l'impact dépend d'une intégration à plusieurs moteurs.

**Correction :** isoler le cache par moteur avec un identifiant immuable ou une structure distincte, tout en gardant son périmètre par requête.

**Critère de validation :** les décisions de deux moteurs ayant des providers différents restent indépendantes sur un même contexte, sujet et scope.

### YSEC-07 — Expiration, émetteur et audience JWT

**Preuve :** [tokenService.go](../services/tokenService.go), lignes 64–66.

HS256 et la longueur minimale de clé sont contrôlés. `CreateToken` ajoute une expiration, mais `ValidToken` n'exige pas sa présence et ne vérifie ni issuer ni audience. Un jeton correctement signé pour un autre usage peut être accepté si la clé est partagée. Cela n'autorise pas la création d'un jeton sans connaître la clé. L'expiration facultative est explicitée dans le [contrat de la bibliothèque JWT](https://pkg.go.dev/github.com/golang-jwt/jwt/v5#WithExpirationRequired).

**Correction :** rendre l'expiration obligatoire, définir puis valider issuer/audience, séparer les clés par usage ; définir côté application durée, rotation, révocation et invalidation après changement de mot de passe ou désactivation de compte.

**Critère de validation :** seuls les jetons signés, non expirés, destinés à cette API et provenant de l'émetteur attendu sont acceptés.

### YSEC-08 — Délais HTTP et budgets de requête absents

**Preuves :** [routing.go](../routing/routing.go), ligne 180 ; [handlersServices.go](../routing/handlersServices.go), ligne 18.

`gin.Run` utilise `http.ListenAndServe` sans délais explicites. Le framework ne pose pas non plus de plafond global avant le binding JSON. Sans protection amont, des connexions lentes et corps volumineux peuvent monopoliser des ressources. Go possède bien une limite d'en-têtes par défaut : ils ne sont pas décrits ici comme illimités.

**Correction :** construire un `http.Server` configurable avec `ReadHeaderTimeout`, délais de lecture/écriture et `IdleTimeout` adaptés ; plafonner les corps par route. Propager les contextes et délais aux opérations DB/cache afin que le travail s'arrête aussi après abandon du client.

**Critère de validation :** délais et quotas sont effectifs au niveau du backend ; une annulation interrompt les opérations associées.

### YSEC-09 — Confiance excessive dans les en-têtes d'adresse IP

**Preuve :** [routing.go](../routing/routing.go), ligne 124 ; défauts de Gin v1.10.1 vérifiés dans le module installé.

`gin.Default()` approuve tous les proxies tant que `SetTrustedProxies` n'est pas appelé. Si les en-têtes d'adresse fournis par le client atteignent Gin, `ClientIP()` et les journaux peuvent refléter une valeur non fiable. Cela fragilise aussi toute limite applicative ou règle d'accès fondée sur cette IP. La [documentation Gin](https://github.com/gin-gonic/gin/blob/v1.10.1/docs/doc.md) demande une configuration explicite.

**Correction :** proposer une liste de proxies de confiance ; utiliser `SetTrustedProxies(nil)` en connexion directe ; exiger du proxy amont une normalisation des en-têtes.

**Critère de validation :** un pair non approuvé ne peut pas influencer l'adresse utilisée pour la journalisation ou le contrôle d'accès.

### YSEC-10 — Chiffrement et authentification du transport DB/cache

**Preuves :** [databaseProvider.go](../services/providers/databaseProvider.go), lignes 110, 141–145 et 242–247.

PostgreSQL utilise `sslmode=disable` par défaut. Redis est construit sans `TLSConfig` et le provider n'expose pas cette possibilité. Sur un réseau non fiable, les commandes et données peuvent être observées ou altérées ; le mot de passe Redis peut aussi circuler sans protection TLS. Ce constat n'affirme pas que les instances locales ou un socket Unix sont exposés.

**Correction :** TLS avec vérification d'identité pour Redis et PostgreSQL (`verify-full` pour PostgreSQL), configuration explicite des autorités et noms de serveur ; exceptions locales explicites. L'API HTTP nécessite aussi une terminaison TLS et un backend inaccessible directement si elle transporte des identifiants.

**Critère de validation :** une identité serveur non fiable est refusée ; aucun retour silencieux vers un transport en clair.

### YSEC-11 — Données métier présentes dans les logs SQL

**Preuve :** [databaseProvider.go](../services/providers/databaseProvider.go), ligne 94, avec `gorm.Config{}`.

Le logger par défaut de GORM v1.30.3 journalise les erreurs et requêtes lentes avec leurs paramètres. Des données de modèles ou de filtres, y compris sensibles, peuvent être copiées dans les logs. Le réglage Gin en production ne modifie pas ce logger SQL. [GORM documente ParameterizedQueries](https://gorm.io/docs/logger.html).

**Correction :** injecter un logger SQL qui masque les paramètres (`ParameterizedQueries: true`), définir les événements nécessaires et limiter accès/rétention. Ajouter un masquage des secrets aux logs HTTP, notamment query strings et en-têtes susceptibles d'être inclus lors d'un panic en debug.

**Critère de validation :** les erreurs et lenteurs restent diagnostiquables sans recopier les valeurs sensibles dans les sorties de journalisation.

### YSEC-12 — Stockage Redis des échecs sans borne de rétention

**Preuve :** [passwordService.go](../services/passwordService.go), lignes 48–75.

La lecture porte sur 24 heures, mais aucun TTL ni retrait des événements anciens n'est appliqué. Le cache croît durablement, avec les noms d'utilisateur directement employés comme clés. `ZRangeByScore` charge toutes les entrées sélectionnées uniquement pour les compter. Ce problème de rétention est distinct de l'exactitude du compteur YSEC-05.

**Correction :** expiration et purge atomique de la fenêtre, namespace dédié, minimisation des identifiants stockés et opération de comptage côté Redis.

**Critère de validation :** taille et durée de stockage restent bornées après expiration ; les autres usages du cache ne partagent pas les mêmes clés.

### YSEC-13 — Les hooks de décision ne voient pas les refus anonymes

**Preuve :** [ginmw.go](../authorization/ginmw/ginmw.go), lignes 49–56.

Le middleware retourne un 401 avant `HasPermission` lorsque le sujet est anonyme. Les hooks de décision du moteur ne reçoivent donc pas cet événement, alors que le moteur sait représenter une décision `unauthenticated`. Il s'agit d'un angle mort d'observabilité, sans contournement d'autorisation.

**Correction :** centraliser la notification des refus et vérifier son invocation par le middleware.

**Critère de validation :** chaque refus anonyme produit exactement un événement de décision exploitable par la journalisation et les métriques.

## Responsabilités d'intégration et durcissement complémentaire

Ces points sont pertinents pour une production, mais ne sont pas comptés comme des vulnérabilités indépendantes démontrées dans une application déployée.

| Domaine | Observation et action attendue |
| --- | --- |
| Activation RBAC | [loader.go](../routing/loader.go), lignes 111–117 et 229–239 : sans moteur, les permissions déclarées sont ignorées avec warning. Ajouter un mode strict refusant les déclarations sans authorizer et contrôler la surface publiée au démarrage. |
| ABAC et relations | Le CRUD et `UpsertRelations` n'appliquent pas automatiquement l'autorisation métier. Contrôler le parent **et chaque ressource liée** avant lecture exposée ou mutation ; filtrer les collections en base. Un UUID connu ne constitue pas une autorisation. |
| Binding et données sensibles | [handlersServices.go](../routing/handlersServices.go), lignes 17–106 : binding direct, hydratation par identité et conservation de l'objet sur absence. [relations.go](../services/database/relations.go), ligne 47 : préchargement de toutes les associations directes. Utiliser des DTO limités, contrôler les champs modifiables et sérialiser explicitement les données autorisées. Les IDs/audits du preset `Base` sont masqués en JSON, ce qui évite de qualifier tout binding de vulnérable. |
| Budgets SQL | [queries.go](../services/database/queries.go), ligne 224 : pagination désactivée pour page/taille inférieure à 1, pas de maximum ; [databaseProvider.go](../services/providers/databaseProvider.go), ligne 193 : pool facultatif. Borner pages, résultats, relations, temps de requête et connexions. |
| Mots de passe | Sans politique configurée, `IsPasswordValid` accepte toute valeur non vide ; le helper de hachage ne l'appelle pas. Définir une politique et la faire appliquer dans inscription/changement. L'exemple de login [services.md](services.md), lignes 313–323, évite bcrypt pour un compte absent : harmoniser le coût de comparaison et limiter les tentatives. |
| Fichiers persistés | `SaveFile`/`GenerateFile` ne remontent pas leurs erreurs et `os.Create` peut tronquer un fichier existant. Exiger une identité avant calcul du nom, propager les erreurs et utiliser une écriture atomique avec une politique d'écrasement explicite. |
| Formats et permissions de fichiers | Appliquer les formats autorisés selon le métier, vérifier le contenu et isoler le service des fichiers non fiables. Choisir des permissions disque restrictives et valider les limites/dossiers au démarrage. Aucun chemin de traversée de répertoire n'a été confirmé dans le flux documenté upload → création → sauvegarde. |
| Plugins | [compiler.go](../compiler/compiler.go), ligne 61, puis `plugin.Open` : existence vérifiée, sans garantie de fraîcheur/provenance. Déployer plugins et binaire immuables ensemble, avec manifeste d'intégrité et droits empêchant leur modification par le compte du service. Les plugins exécutent du code natif de confiance ; aucune exécution distante autonome n'est démontrée. Voir le [contrat Go plugin](https://pkg.go.dev/plugin). |
| Configuration | Les clés inconnues produisent des warnings ; les variables d'environnement manquantes peuvent devenir vides. Valider strictement en production les paramètres sensibles, durées, tailles et secrets. Ne pas confondre longueur minimale de clé JWT et entropie suffisante. |
| Cohérence documentaire | Le README indique encore qu'une configuration CORS vide autorise toutes les origines, alors que `buildCORSConfig` n'installe plus CORS sans origine. Corriger cette divergence ; le comportement réel est fermé pour les navigateurs. CORS ne remplace pas une autorisation. |
| Administration et audit | `GrantAdmin` est une API de stockage : protéger son exposition et l'administration déléguée. Les rôles sont globaux et leurs bindings portent le scope. L'audit optionnel après mutation ne garantit pas une persistance atomique durable. |
| Migrations | Migrations examinées sans application à une base existante. Pour `migrations/v1_to_v2.sql`, qualifier explicitement le schéma dans les opérations et restreindre la recherche de contraintes à la table cible ; prévoir sauvegarde et revue du périmètre avant migration réelle. |
| Automatisation | Aucun workflow de sécurité ni politique `SECURITY.md` trouvé dans cet état du dépôt. Ajouter tests, scan de dépendances et secrets en CI, reconstruire les plugins, documenter un canal de signalement privé. |

## Contrôles favorables vérifiés

- Algorithme JWT HS256 explicitement imposé ; clé d'au moins 32 octets exigée lors de création/validation ; échec du démarrage en production pour une clé trop courte.
- Réponses d'authentification génériques ; claims de sujet absents/vides/non textuels refusés ; panne de base distinguée d'une identité absente.
- bcrypt avec coût par défaut 12.
- Autorisation refusée par défaut ; scopes exacts dans les providers mémoire/GORM ; union globale explicite ; ABAC combiné par conjonction et non mémorisé.
- Requêtes du store d'autorisation paramétrées ; mutations transactionnelles ; clés étrangères et contraintes uniques.
- Isolation du cache entre requêtes, sujets et scopes ; copies défensives des grants ; révocation visible à la requête suivante. YSEC-06 concerne l'isolation supplémentaire entre moteurs.
- Erreurs de chargement des plugins/middlewares bloquantes ; validation des permissions ; RBAC inséré après les callbacks applicatifs. Les conventions `nil` et `^` de middleware ne retirent pas ce RBAC.
- CORS absent sans origine explicitement déclarée ; DSN PostgreSQL correctement échappé.
- `Create` réinitialise les identités générées du preset `Base` ; associations omises des écritures implicites ; update/delete exigent la PK complète ; suppression auditée transactionnelle ; absence et panne distinguées dans les upserts.
- Aucun client HTTP sortant trouvé dans `httpService.go` : le helper construit une URL depuis la configuration, sans point SSRF identifié.

## Ordre proposé pour les corrections

1. **Avant exposition sensible :** YSEC-01/02, recherche exacte des identités et protection des contraintes serveur ; YSEC-03, migration Go/dépendances et reconstruction des plugins ; YSEC-04/05, quotas avant parsing et limitation atomique des tentatives.
2. **Avant validation de la v2 :** isolation du cache entre moteurs, politique JWT, serveur HTTP configurable, proxies et TLS explicites, logs sans données sensibles, expiration Redis.
3. **Validation de livraison :** tests de non-régression des propriétés ci-dessus, tests PostgreSQL sur instance dédiée, vérification du chargement des plugins, scan des binaires réellement livrés et revue des contrôles d'accès de chaque application.
4. **Maintenance :** observabilité des refus, validation stricte de configuration, fiabilisation du stockage, CI et documentation de sécurité.

Les corrections doivent être réalisées sur une branche Git Flow dédiée et revues séparément des modifications fonctionnelles déjà présentes. Le présent audit ne les applique pas.

## Annexes et limites

- [Avis de dépendances, CSV](security-audit-2026-09-10/dependencies.csv) : 86 avis distincts classés par profondeur de détection ; liens vers la base officielle.
- [Éléments de vérification, JSON](security-audit-2026-09-10/evidence.json) : versions/outils, compteurs, résultats, périmètre du contrôle de secrets et empreintes des fichiers audités.
- Les journaux bruts du scan restent dans `/private/tmp/yogourt-govulncheck-2026-09-10.json` pendant cette session.
- L'audit ne couvre pas les ACL réelles de déploiement, clés de production, images de conteneurs, règles réseau, handlers et modèles des applications consommatrices, ni les configurations de leurs reverse proxies.
- Les chemins dynamiques et plugins des applications finales ne sont pas tous visibles dans ce module. Une absence d'avis au niveau symbole n'exclut pas un usage vulnérable dans un consommateur. Les [limites et le modèle d'analyse de govulncheck](https://go.dev/doc/security/vuln/) s'appliquent.
