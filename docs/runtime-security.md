# Sécurité du runtime v2

Cette page décrit les protections actives du serveur HTTP et des providers. Les
valeurs zéro ou absentes de la configuration sélectionnent les limites
ci-dessous ; elles ne désactivent pas les protections.

## Budgets HTTP et proxies

Le serveur applique les valeurs par défaut suivantes :

| Clé | Défaut | Maximum accepté |
| --- | ---: | ---: |
| `server.read_header_timeout` | `5s` | `10m` |
| `server.read_timeout` | `30s` | `10m` |
| `server.write_timeout` | `30s` | `10m` |
| `server.idle_timeout` | `2m` | `10m` |
| `server.max_header_bytes` | 1 Mio | 8 Mio |
| `server.max_body_bytes` | 32 Mio | 1 Gio |

Une durée négative, une taille négative ou une valeur supérieure au maximum
arrête le démarrage. Un corps dont `Content-Length` dépasse la limite reçoit
immédiatement une réponse `413`. Les corps diffusés sans taille déclarée sont
bornés pendant leur lecture et `routing.HandleRequest` traduit ce dépassement
en `413`.

`server.trusted_proxies` contient les adresses ou réseaux CIDR des reverse
proxies autorisés à fournir l’adresse du client. La liste absente ou vide ne
fait confiance à aucun proxy : les en-têtes `X-Forwarded-For` et apparentés ne
remplacent pas l’adresse du pair TCP.

~~~yaml
server:
  trusted_proxies:
    - "10.20.0.0/16"
  read_header_timeout: 5s
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 2m
  max_header_bytes: 1048576
  max_body_bytes: 33554432
~~~

`services.ReadUploadedFile` pose le plafond du corps avant le parsing multipart.
Il refuse un formulaire déjà parsé si le framework ne peut pas prouver qu’un
plafond était actif. Plusieurs fichiers peuvent être lus successivement dans la
même requête. Le framework supprime les fichiers multipart temporaires à la fin
du traitement, en complément du nettoyage assuré par `net/http`.

Lors de la migration, les requêtes dépassant ces budgets peuvent désormais
échouer avec `413` ou sur délai. Les applications qui utilisaient l’adresse
transmise par un reverse proxy doivent déclarer ce proxy dans
`server.trusted_proxies` ; sans cette déclaration, Gin utilise l’adresse du pair
TCP. Un middleware applicatif qui parse un formulaire avant le plafond du
framework doit être déplacé après celui-ci.

## PostgreSQL

Une connexion réseau utilise `sslmode=verify-full` lorsque
`database.ssl_mode` est absent. Sur TCP, seul `verify-full` est accepté : les
modes opportunistes ou sans vérification d’identité arrêtent l’initialisation.
Configurez `database.ssl_root_cert` pour une autorité privée, et
`database.ssl_cert` avec `database.ssl_key` lorsque le serveur exige un
certificat client.

Le transport en clair est réservé aux sockets Unix locaux déclarés par un
chemin absolu. Il exige une exception explicite et ne s’applique jamais à un
hôte réseau ni à un `database.host` vide, car les variables libpq pourraient
alors sélectionner un hôte TCP :

~~~yaml
database:
  host: "/var/run/postgresql"
  ssl_mode: "disable"
  allow_insecure_local_socket: true
~~~

Ce changement rompt les configurations TCP qui dépendaient du précédent défaut
`sslmode=disable`. Elles doivent activer PostgreSQL TLS avec un certificat dont
l’identité correspond à `database.host`, ou utiliser l’exception socket locale.

Les logs GORM conservent la forme de la requête SQL pour le diagnostic mais
retirent ses paramètres, y compris dans les erreurs et les requêtes lentes.

## Redis

Redis TLS est activé avec `cache.tls.enabled`. La vérification du certificat et
du nom est toujours active, avec TLS 1.2 au minimum. `server_name` remplace le
nom de `cache.host` pour la vérification, `ca_file` ajoute une autorité privée,
et `cert_file`/`key_file` configurent un certificat client. Les deux fichiers
client sont indissociables.

~~~yaml
cache:
  host: "redis.internal"
  port: "6379"
  tls:
    enabled: true
    server_name: "redis.internal"
    ca_file: "/etc/yogourt/redis-ca.pem"
    cert_file: "/etc/yogourt/redis-client.pem"
    key_file: "/etc/yogourt/redis-client-key.pem"
~~~

TLS Redis reste désactivé lorsque `cache.tls.enabled` est absent afin de
préserver les instances locales existantes. Déclarer un certificat ou un nom de
serveur sans activer TLS est une erreur. L’API HTTP doit être placée derrière
une terminaison TLS lorsqu’elle transporte des identifiants ou des données
sensibles.
