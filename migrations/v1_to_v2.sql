-- Migration v1 -> v2 for public tables using the former interfaces.Base.
-- It renames the legacy primary-key constraint and creates the soft-delete
-- index introduced by v2. It does not move or rewrite application data.

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
        IF EXISTS (SELECT 1
                   FROM pg_constraint
                   WHERE conrelid = format('%I.%I', 'public', t)::regclass
                     AND contype = 'p'
                     AND conname = 'uni_' || t || '_id') THEN
            EXECUTE format('ALTER TABLE %I.%I RENAME CONSTRAINT %I TO %I',
                           'public', t, 'uni_' || t || '_id', t || '_pkey');
        END IF;
        EXECUTE format('CREATE INDEX IF NOT EXISTS %I ON %I.%I (deleted_at)',
                       'idx_' || t || '_deleted_at', 'public', t);
    END LOOP;
END $$;
