# Yogourt

## Installation du CLI

Pour installer le CLI de Yogourt globalement sur votre machine:

```sh
go install github.com/goyourt/yogourt/cli@latest
```

Cela installera le binaire dans votre `$GOPATH/bin` (ou `$GOBIN` si défini). Assurez-vous que ce dossier est dans votre `PATH` pour pouvoir utiliser la commande depuis n'importe où.

## Utilisation

Après installation, vous pouvez lancer le CLI avec:

```sh
yogourt
```

ou selon le nom du binaire généré (par défaut, le nom du dossier `cli`).

Pour afficher l'aide:

```sh
yogourt --help
```
