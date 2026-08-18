-- 0001_authz_tables.sql
-- Initial schema of the Yogourt RBAC store (authorization/gormstore).
-- Every statement is idempotent (IF NOT EXISTS) so the migration can be
-- re-applied safely. Applied explicitly via gormstore.Migrate — never
-- automatically.

CREATE TABLE IF NOT EXISTS authz_permissions (
    id BIGSERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS authz_roles (
    id BIGSERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS authz_role_permissions (
    role_id BIGINT NOT NULL REFERENCES authz_roles (id) ON DELETE CASCADE,
    permission_id BIGINT NOT NULL REFERENCES authz_permissions (id) ON DELETE CASCADE,
    UNIQUE (role_id, permission_id)
);

-- subject_id is the stable identity of the subject (UUID string), never an
-- application SQL id.
CREATE TABLE IF NOT EXISTS authz_role_bindings (
    id BIGSERIAL PRIMARY KEY,
    subject_id TEXT NOT NULL,
    scope TEXT NOT NULL,
    role_id BIGINT NOT NULL REFERENCES authz_roles (id) ON DELETE CASCADE,
    UNIQUE (subject_id, scope, role_id)
);

-- Index driving Resolve: bindings are always looked up by (subject, scope).
CREATE INDEX IF NOT EXISTS idx_authz_role_bindings_subject_scope
    ON authz_role_bindings (subject_id, scope);
