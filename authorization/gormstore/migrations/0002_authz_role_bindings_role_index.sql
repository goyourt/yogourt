-- 0002_authz_role_bindings_role_index.sql
-- Index driving the administration reads that start from a role
-- (Store.RoleBindings: "who holds this role?"). The UNIQUE (subject_id, scope,
-- role_id) constraint of 0001 cannot serve them, as role_id is not its leading
-- column; DeleteRole benefits from it too, since the ON DELETE CASCADE of the
-- bindings looks rows up by role_id.
--
-- Idempotent (IF NOT EXISTS) like every migration, and additive: 0001 is never
-- modified, as deployed databases have already applied it.

CREATE INDEX IF NOT EXISTS idx_authz_role_bindings_role
    ON authz_role_bindings (role_id);
