-- Roles: org-scoped, each organization can define custom roles.
-- System roles (is_system=true) are seeded by the kernel and cannot be deleted.
CREATE TABLE IF NOT EXISTS module_iam.roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES module_iam.organizations(id) ON DELETE CASCADE,
    name JSONB NOT NULL DEFAULT '{}',
    slug TEXT NOT NULL,
    description JSONB NOT NULL DEFAULT '{}',
    is_system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_roles_org_slug UNIQUE (org_id, slug)
);
CREATE INDEX IF NOT EXISTS idx_roles_org_id ON module_iam.roles (org_id)
WHERE deleted_at IS NULL;
-- Role permissions: maps a role to permission keys declared in module manifests.
-- permission_key references the static key from sdk.Permission (e.g., "iam.users.read").
CREATE TABLE IF NOT EXISTS module_iam.role_permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id UUID NOT NULL REFERENCES module_iam.roles(id) ON DELETE CASCADE,
    permission_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_role_permissions_role_key UNIQUE (role_id, permission_key)
);
CREATE INDEX IF NOT EXISTS idx_role_permissions_role_id ON module_iam.role_permissions (role_id);
-- User roles: maps a user to roles within an org. A user can have multiple roles.
CREATE TABLE IF NOT EXISTS module_iam.user_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES module_iam.organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES module_iam.users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES module_iam.roles(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_user_roles_org_user_role UNIQUE (org_id, user_id, role_id)
);
CREATE INDEX IF NOT EXISTS idx_user_roles_org_user ON module_iam.user_roles (org_id, user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_role_id ON module_iam.user_roles (role_id);
