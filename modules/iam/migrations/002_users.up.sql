CREATE TABLE IF NOT EXISTS module_iam.users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES module_iam.organizations(id),
    external_id TEXT NOT NULL,
    provider TEXT NOT NULL DEFAULT 'platform',
    email TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL DEFAULT '',
    avatar_url TEXT NOT NULL DEFAULT '',
    locale TEXT NOT NULL DEFAULT 'en',
    timezone TEXT NOT NULL DEFAULT 'UTC',
    status TEXT NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_users_org_external UNIQUE (org_id, provider, external_id)
);
CREATE INDEX IF NOT EXISTS idx_users_org_id ON module_iam.users (org_id);
CREATE INDEX IF NOT EXISTS idx_users_email ON module_iam.users (email)
WHERE email != ''
    AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_phone ON module_iam.users (phone)
WHERE phone != ''
    AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_status ON module_iam.users (org_id, status)
WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_deleted ON module_iam.users (deleted_at)
WHERE deleted_at IS NOT NULL;
