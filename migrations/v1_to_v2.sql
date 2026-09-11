-- Migration de schéma v1 -> v2 pour les tables portant l'ancien interfaces.Base.
-- Aucune donnée déplacée. Deux écarts, mesurés entre AutoMigrate v1 et v2
-- (GORM 1.30.3 / PostgreSQL 15) :
--   1. le tag v1 `unique` sur la clé primaire renommait la contrainte PK en
--      uni_<table>_id ; la v2 revient au nom par défaut <table>_pkey ;
--   2. la v2 indexe deleted_at (chaque requête soft-delete filtre dessus),
--      sous le nom qu'AutoMigrate v2 lui donne.
-- Les tables ciblées portent la signature de Base v1 : colonnes id, uuid et
-- deleted_at dans le schéma public.
-- Après migration, seul l'ordre physique des colonnes peut différer d'un
-- AutoMigrate v2 sur base vierge (les briques v2 groupent timestamps et
-- audit) : sans effet sémantique, et le réordonner exigerait une réécriture
-- de table. Validé le 2026-08-28 par comparaison de dumps (PG 15).

DO $$
DECLARE
    t text;
BEGIN
    FOR t IN
        SELECT c.table_name
        FROM information_schema.columns c
        WHERE c.table_schema = 'public'
          AND c.column_name = 'deleted_at'
          AND EXISTS (SELECT 1 FROM information_schema.columns c2
                      WHERE c2.table_schema = 'public'
                        AND c2.table_name = c.table_name
                        AND c2.column_name = 'id')
          AND EXISTS (SELECT 1 FROM information_schema.columns c3
                      WHERE c3.table_schema = 'public'
                        AND c3.table_name = c.table_name
                        AND c3.column_name = 'uuid')
    LOOP
        IF EXISTS (SELECT 1 FROM pg_constraint
                   WHERE conname = 'uni_' || t || '_id') THEN
            EXECUTE format('ALTER TABLE %I RENAME CONSTRAINT %I TO %I',
                           t, 'uni_' || t || '_id', t || '_pkey');
        END IF;
        EXECUTE format('CREATE INDEX IF NOT EXISTS %I ON %I (deleted_at)',
                       'idx_' || t || '_deleted_at', t);
    END LOOP;
END $$;
