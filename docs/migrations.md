# Migrations

Yogourt versionne le schéma de la base avec des migrations écrites en Go. Chaque migration est une paire de fonctions <code>Up</code> / <code>Down</code> identifiée par un horodatage. Le framework se charge de les ordonner, de les exécuter et de mémoriser celles qui ont déjà été appliquées ; **le contenu des migrations, lui, est entièrement à la charge du développeur** : aucune requête SQL n’est générée à partir des modèles.

Le mécanisme repose sur deux briques :

- le package <code>github.com/goyourt/yogourt/migrations</code>, qui exécute les migrations et tient la table de suivi ;
- les commandes <code>make-migration</code> et <code>migrate</code> de <code>yogourt-cli</code>, qui génèrent les fichiers et lancent l’exécution.

## Arborescence

Les migrations vivent dans le dossier <code>migrations/</code> à la racine du projet. C’est un package <code>main</code> autonome, exécuté par <code>go run</code> :

~~~text
.
├── migrations/
│   ├── migrate.go          ← point d’entrée, registre des migrations
│   ├── 202610261030.go     ← une migration
│   └── 202610281415.go     ← une autre migration
└── yogourt.yaml
~~~

Les commandes doivent être lancées depuis la racine du projet : la connexion à la base utilise le fichier <code>./yogourt.yaml</code> du répertoire courant (voir [Configuration](configuration.md)).

## La table <code>migrations</code>

Au premier appel de <code>make-migration</code> ou de <code>migrate</code>, le CLI appelle <code>migrations.InitMigrations()</code>, qui crée (ou met à jour) la table <code>migrations</code> dans la base configurée :

| Colonne | Type | Rôle |
| --- | --- | --- |
| <code>id</code> | entier, clé primaire | identifiant technique |
| <code>timestamp</code> | texte | horodatage de la migration, par exemple <code>202610261030</code> |
| <code>executed_at</code> | date | moment où la migration a été appliquée |

Une ligne est insérée après chaque <code>Up</code> réussi et supprimée après chaque <code>Down</code>. C’est cette table, et elle seule, qui détermine si une migration est considérée comme appliquée. Ne la modifiez à la main qu’en connaissance de cause.

## Commandes du CLI

### <code>yogourt make-migration</code>

~~~sh
yogourt make-migration
~~~

La commande :

1. crée le dossier <code>migrations/</code> et son fichier <code>migrate.go</code> s’ils n’existent pas ;
2. crée la table <code>migrations</code> en base si besoin ;
3. génère un fichier <code>migrations/&lt;horodatage&gt;.go</code>, l’horodatage étant la date courante au format <code>AAAAMMJJhhmm</code> ;
4. enregistre les deux nouvelles fonctions dans les maps <code>Up</code> et <code>Down</code> de <code>migrate.go</code>.

Le fichier généré contient deux fonctions **vides** :

~~~go
package main

import "gorm.io/gorm"

func Up202610261030(db *gorm.DB) {
}

func Down202610261030(db *gorm.DB) {
}
~~~

Et <code>migrate.go</code> est mis à jour :

~~~go
package main

import (
	"github.com/goyourt/yogourt/migrations"
	"gorm.io/gorm"

	"os"
)

var Up = map[string]func(*gorm.DB){"202610261030": Up202610261030, }

var Down = map[string]func(*gorm.DB){"202610261030": Down202610261030, }

func main() {
	migrations.Migrate(Up, Down, os.Args[1:])
}
~~~

> [!NOTE]
> L’horodatage est à la minute près. Deux <code>make-migration</code> lancés dans la même minute produisent le même nom de fichier : le second écrase le premier. Attendez la minute suivante.

> [!TIP]
> Ne renommez pas les fonctions et ne modifiez pas la forme des déclarations <code>var Up = …</code> / <code>var Down = …</code> : le CLI les retrouve par expression régulière pour y ajouter les nouvelles migrations.

### <code>yogourt migrate</code>

La commande compile et exécute le package <code>migrations/</code> (<code>go run ./migrations/</code>) en lui transmettant les arguments.

| Commande | Effet |
| --- | --- |
| <code>yogourt migrate</code> | applique toutes les migrations <code>Up</code> pas encore exécutées, dans l’ordre croissant des horodatages |
| <code>yogourt migrate 202610261030</code> | applique uniquement la migration <code>Up</code> indiquée, si elle n’a pas déjà été exécutée |
| <code>yogourt migrate 202610261030 --up</code> | identique à la ligne précédente ; <code>--up</code> est le comportement par défaut |
| <code>yogourt migrate 202610261030 --down</code> | annule la migration indiquée en exécutant sa fonction <code>Down</code>, si elle a été appliquée |

Règles à connaître :

- une migration déjà appliquée est ignorée par <code>Up</code> et une migration jamais appliquée est ignorée par <code>Down</code> ; le terminal affiche alors <code>migration &lt;horodatage&gt; skipped</code> ;
- <code>--down</code> exige toujours un horodatage : il n’existe pas de rollback global. Pour revenir plusieurs crans en arrière, annulez les migrations une par une, de la plus récente à la plus ancienne ;
- un horodatage absent des maps provoque l’erreur <code>migration &lt;horodatage&gt; not found</code>.

Exemple de cycle complet :

~~~sh
yogourt make-migration                # crée migrations/202610261030.go
# … écrire le SQL dans Up202610261030 et Down202610261030 …
yogourt migrate                       # applique la migration
yogourt migrate 202610261030 --down   # l’annule
yogourt migrate 202610261030          # la réapplique
~~~

## Écrire une migration

Chaque fonction reçoit la connexion GORM partagée (<code>*gorm.DB</code>). Le plus simple est d’y exécuter du SQL brut avec <code>db.Exec</code>. La fonction <code>Down</code> doit défaire exactement ce que fait <code>Up</code>, dans l’ordre inverse.

### Gérer les erreurs

Les fonctions ne retournent pas d’erreur : **si une requête échoue sans arrêter le programme, la migration est quand même marquée comme appliquée**. Arrêtez donc l’exécution dès qu’une requête échoue, par exemple avec <code>panic</code> : le processus s’interrompt avant l’écriture dans la table <code>migrations</code>, et la migration pourra être relancée une fois corrigée.

Un petit utilitaire dans le package évite de répéter ce contrôle :

~~~go
// migrations/helpers.go
package main

import "gorm.io/gorm"

func mustExec(db *gorm.DB, sql string, values ...any) {
	if err := db.Exec(sql, values...).Error; err != nil {
		panic(err)
	}
}
~~~

Les exemples suivants l’utilisent.

### Utiliser une transaction

Le framework n’ouvre pas de transaction autour d’une migration. Si une migration contient plusieurs requêtes, enveloppez-les dans <code>db.Transaction</code> pour éviter un schéma à moitié modifié en cas d’échec (PostgreSQL gère les instructions DDL en transaction) :

~~~go
func Up202610261030(db *gorm.DB) {
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`ALTER TABLE users ADD COLUMN phone VARCHAR(20)`).Error; err != nil {
			return err
		}
		return tx.Exec(`CREATE INDEX idx_users_phone ON users (phone)`).Error
	})
	if err != nil {
		panic(err)
	}
}
~~~

### Exemple : créer une table

~~~go
func Up202610261030(db *gorm.DB) {
	mustExec(db, `
		CREATE TABLE articles (
			id            SERIAL PRIMARY KEY,
			uuid          UUID NOT NULL DEFAULT gen_random_uuid() UNIQUE,
			title         VARCHAR(255) NOT NULL,
			content       TEXT,
			author_id     INTEGER REFERENCES users (id) ON DELETE SET NULL,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
			deleted_at    TIMESTAMPTZ
		)
	`)
	mustExec(db, `CREATE INDEX idx_articles_deleted_at ON articles (deleted_at)`)
}

func Down202610261030(db *gorm.DB) {
	mustExec(db, `DROP TABLE IF EXISTS articles`)
}
~~~

Les colonnes doivent correspondre aux champs du modèle Go correspondant (voir [Services](services.md)) : la migration ne lit pas le modèle, c’est au développeur de les garder alignés.

### Exemple : ajouter et supprimer une colonne

~~~go
func Up202610271200(db *gorm.DB) {
	mustExec(db, `ALTER TABLE articles ADD COLUMN published BOOLEAN NOT NULL DEFAULT false`)
}

func Down202610271200(db *gorm.DB) {
	mustExec(db, `ALTER TABLE articles DROP COLUMN published`)
}
~~~

### Exemple : renommer une colonne

~~~go
func Up202610271230(db *gorm.DB) {
	mustExec(db, `ALTER TABLE articles RENAME COLUMN content TO body`)
}

func Down202610271230(db *gorm.DB) {
	mustExec(db, `ALTER TABLE articles RENAME COLUMN body TO content`)
}
~~~

### Exemple : table de jointure many-to-many

~~~go
func Up202610281000(db *gorm.DB) {
	mustExec(db, `
		CREATE TABLE article_tags (
			article_id INTEGER NOT NULL REFERENCES articles (id) ON DELETE CASCADE,
			tag_id     INTEGER NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
			PRIMARY KEY (article_id, tag_id)
		)
	`)
}

func Down202610281000(db *gorm.DB) {
	mustExec(db, `DROP TABLE IF EXISTS article_tags`)
}
~~~

### Exemple : migrer des données

Une migration peut aussi transformer des données existantes. Utilisez des paramètres liés (<code>?</code>) plutôt que de concaténer des valeurs dans la requête :

~~~go
func Up202610281415(db *gorm.DB) {
	mustExec(db, `ALTER TABLE users ADD COLUMN status VARCHAR(20)`)
	mustExec(db, `UPDATE users SET status = ? WHERE deleted_at IS NULL`, "active")
	mustExec(db, `UPDATE users SET status = ? WHERE deleted_at IS NOT NULL`, "archived")
	mustExec(db, `ALTER TABLE users ALTER COLUMN status SET NOT NULL`)
}

func Down202610281415(db *gorm.DB) {
	mustExec(db, `ALTER TABLE users DROP COLUMN status`)
}
~~~

Une transformation de données n’est pas toujours réversible. Dans ce cas, faites échouer <code>Down</code> explicitement plutôt que de le laisser vide, sinon la migration serait marquée comme annulée sans que rien n’ait changé :

~~~go
func Down202610281415(db *gorm.DB) {
	panic("migration 202610281415 is irreversible")
}
~~~

## Bonnes pratiques

- **Une migration appliquée ne se modifie plus.** Si elle a tourné sur un autre environnement, corrigez le schéma avec une nouvelle migration.
- **Testez le <code>Down</code>** : lancez <code>migrate</code>, puis <code>migrate &lt;horodatage&gt; --down</code>, puis <code>migrate</code> à nouveau sur une base de développement.
- **Une migration, un changement cohérent** : plusieurs petites migrations se relisent et s’annulent plus facilement qu’une grosse.
- **Sauvegardez la base** avant d’appliquer des migrations en production.
- **Versionnez le dossier <code>migrations/</code>** avec le reste du code : les horodatages garantissent le même ordre d’exécution sur tous les environnements.
