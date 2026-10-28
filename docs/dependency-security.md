# Dépendances après l’audit du 10 septembre 2026

Vérification du 11 septembre 2026, après correction de YSEC-03.

- Go minimum : **1.26.8**, avec la même toolchain pour le binaire et les plugins.
- `github.com/jackc/pgx/v5` : **v5.9.2**.
- `golang.org/x/crypto` : **v0.56.0**.
- Graphe transitif compatible : `x/text v0.41.0`, `x/net v0.57.0`,
  `x/sys v0.47.0`, `x/sync v0.22.0`.

Les versions Go disponibles ont été vérifiées dans le [registre officiel](https://go.dev/dl/?mode=json).
La branche 1.26 est conservée pour limiter le changement de toolchain.

`govulncheck v1.8.0 -json ./...`, exécuté avec Go 1.26.8 et la base
`https://vuln.go.dev` datée du 10 septembre 2026 à 14:48:42 UTC, termine avec
le code 0 : aucun avis au niveau symbole ou paquet. Un avis au niveau module
reste présent : [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932), pour le
paquet abandonné `golang.org/x/crypto/openpgp`. Yogourt n’importe pas ce paquet ;
son utilisation de `x/crypto/bcrypt` justifie de conserver le module.

Le scan statique n’établit pas une absence universelle de vulnérabilités.
Relancer après chaque mise à jour de dépendance ou de toolchain :

```sh
go test ./...
go vet ./...
go mod verify
govulncheck ./...
```

Ces quatre contrôles passent sur la révision mise à jour. Les validations
d’intégration et avec détection de courses sont consignées dans le suivi
global des corrections.
