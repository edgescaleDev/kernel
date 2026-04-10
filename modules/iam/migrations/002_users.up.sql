CREATE TABLE IF NOT EXISTS public.users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id TEXT NOT NULL,
    provider TEXT NOT NULL DEFAULT 'platform',
    email TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    name JSONB NOT NULL DEFAULT '{}',
    avatar_url TEXT NOT NULL DEFAULT '',
    locale TEXT NOT NULL DEFAULT 'en',
    timezone TEXT NOT NULL DEFAULT 'UTC',
    status TEXT NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_users_external UNIQUE (provider, external_id)
);
CREATE INDEX IF NOT EXISTS idx_users_email ON public.users (email)
WHERE email != ''
    AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_phone ON public.users (phone)
WHERE phone != ''
    AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_status ON public.users (status)
WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_deleted ON public.users (deleted_at)
WHERE deleted_at IS NOT NULL;
