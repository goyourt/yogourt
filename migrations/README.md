# v1 to v2 Base migration

`v1_to_v2.sql` applies only to `public` tables that have the legacy Base
signature: `id`, `uuid`, and `deleted_at`. It renames the legacy primary-key
constraint `uni_<table>_id` to `<table>_pkey` and adds the v2 soft-delete
index `idx_<table>_deleted_at`. It does not change rows.

Back up the target database, then apply the file with the PostgreSQL client
used for that database, for example:

```sh
psql "$DATABASE_URL" -f migrations/v1_to_v2.sql
```

The integration test creates temporary tables in the `public` schema and must
therefore use a dedicated, disposable database. Set
`YOGOURT_MIGRATION_TEST_DSN` to that database's PostgreSQL DSN and run:

```sh
go test ./migrations -run TestV1ToV2
```

The test creates the actual v1 Base table shape with GORM, applies the SQL
twice, checks a preserved row, and compares normalized `pg_dump --schema-only`
output with a fresh v2 Base table. `pg_dump` must be available on `PATH`.
