# Suivi des corrections — 12 septembre 2026

Les 13 constats du [rapport du 10 septembre](security-audit-2026-09-10.md)
ont reçu un correctif. Ce suivi décrit le code validé au commit `0ed7146`,
sur `feature/v2`. Il ne constitue pas un audit d’une application déployée.
Les [résultats structurés](security-remediation-2026-09-12.json) accompagnent
ce document.

## Correctifs et commits

| Constat | Changement | Commit intégré |
| --- | --- | --- |
| YSEC-01 | Chaînes littérales, opérateurs typés, identités liées par égalité stricte | `fe649eb` |
| YSEC-02 | Colonnes/relations validées contre GORM, tri typé | `fe649eb` |
| YSEC-03 | Go 1.26.8 et dépendances corrigées | `b2c9a69` |
| YSEC-04 | Limite totale avant parsing multipart, nettoyage des fichiers temporaires | `0ed7146` |
| YSEC-05 | Événements Redis uniques, écriture et total atomiques | `c30cfc6` |
| YSEC-06 | Cache de grants séparé par moteur, sujet et scope | `a26fffb` |
| YSEC-07 | JWT : expiration obligatoire, issuer et audience vérifiés | `a26fffb` |
| YSEC-08 | Délais HTTP, budgets d’en-têtes et de corps | `0ed7146` |
| YSEC-09 | Aucun proxy de confiance par défaut | `0ed7146` |
| YSEC-10 | PostgreSQL vérifie l’identité réseau, option TLS Redis vérifiée | `0ed7146` |
| YSEC-11 | Logger GORM avec paramètres masqués | `0ed7146` |
| YSEC-12 | Namespace Redis, identités hachées, purge et TTL de 24 heures | `c30cfc6` |
| YSEC-13 | Hook de décision appelé pour les refus anonymes | `a26fffb` |

Le travail initial du mainteneur a été sauvegardé avant les corrections dans
`c9bd648`. L’audit est conservé dans `1893765`. Deux tâches complémentaires
sont intégrées : validation reproductible de la migration (`2f32de4`) et
couverture des invariants d’identité, d’audit et de persistance (`2c0606e`).

Les agents ont travaillé dans des worktrees séparés ; leurs changements ont
été relus, puis committés et intégrés par l’agent principal. Sol avec effort
élevé a été utilisé pour les modifications transversales ; Terra pour la
finalisation ciblée, les tests et Redis. Luna avec effort moyen a préparé le
renommage mécanique du module, ensuite écarté à la demande du mainteneur.

## Validation exécutée

Sur macOS arm64 avec Go 1.26.8, PostgreSQL 15.16 et Redis 8.10.1 temporaires :

- `go test -count=1 ./...` : succès, avec PostgreSQL, migration et Redis activés.
- `go test -race -count=1 ./...` : succès avec les mêmes intégrations.
- `go vet ./...` et `go mod verify` : succès.
- `TestPluginRoutesEndToEnd` : cinq plugins compilés et chargés, sans skip.
- Migration : comparaison du schéma v1 migré avec une création v2 par
  `pg_dump`, application répétée et conservation des données.
- Redis : 32 écritures concurrentes dénombrées individuellement, TTL borné,
  conservation d’un événement vieux de 12 heures, suppression à 25 heures.
- `govulncheck v1.8.0` : code de sortie 0 ; aucun avis au niveau paquet ou
  symbole. Seul subsiste [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932)
  au niveau module, concernant `x/crypto/openpgp`, non importé par Yogourt.

Pour reproduire les intégrations, configurer des instances réservées aux tests :

```sh
export YOGOURT_TEST_DSN='host=/chemin/socket port=5432 user=test dbname=yogourt_test sslmode=disable'
export YOGOURT_MIGRATION_TEST_DSN='host=/chemin/socket port=5432 user=test dbname=yogourt_migration_test sslmode=disable'
export YOGOURT_TEST_REDIS_ADDR='127.0.0.1:6379'
go test -race -count=1 ./...
```

`pg_dump` doit être dans `PATH`. Le test de migration utilise une base distincte,
car le script agit sur les tables compatibles du schéma `public`.

## Choix et limites maintenus

Le chemin `github.com/goyourt/yogourt` est conservé **sans `/v2`**, par décision
du mainteneur. La préversion reste épinglable par commit. Un tag Go `v2.x`
avec le `go.mod` actuel demanderait un suffixe majeur ; le versionnement de
publication est donc une décision séparée. Aucun push ni tag n’a été effectué.

Les changements exigent une adaptation des applications : nouveaux champs JWT,
réémission des tokens, filtres et tris typés, TLS PostgreSQL ou exception socket
explicite, limites HTTP et configuration des proxies. Voir la
[migration](migration-v2.md), la [politique JWT](auth-policy.md), le
[runtime](runtime-security.md) et le [compteur Redis](login-failures.md).

Redis TLS reste une option à activer pour les connexions réseau. Le compteur
mesure les échecs ; une admission ou un verrouillage atomique reste du ressort
de l’application. Le hash d’une identité prévisible n’est pas une anonymisation.
Les anciennes clés Redis ne sont pas supprimées automatiquement.

Les responsabilités applicatives recensées dans l’audit restent valables :
contrôle d’accès métier, politique de mots de passe, révocation des tokens,
validation des fichiers et gestion des erreurs de stockage, intégrité des
plugins et protection des journaux. Les connexions TLS à une infrastructure
de production n’ont pas été testées ; la configuration TLS et ses refus sont
couverts par les tests locaux.
