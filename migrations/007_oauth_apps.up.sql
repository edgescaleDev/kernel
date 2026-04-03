-- Kernel migration: OAuth2 app management.
CREATE TABLE IF NOT EXISTS public.oauth_apps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT,
    client_id TEXT UNIQUE NOT NULL,
    client_secret TEXT NOT NULL,
    redirect_uris TEXT [] NOT NULL DEFAULT '{}',
    scopes TEXT [] NOT NULL DEFAULT '{}',
    app_type TEXT NOT NULL DEFAULT 'private',
    org_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS public.oauth_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id UUID NOT NULL REFERENCES public.oauth_apps(id) ON DELETE CASCADE,
    org_id UUID NOT NULL,
    user_id UUID,
    token_hash TEXT NOT NULL,
    scopes TEXT [] NOT NULL DEFAULT '{}',
    grant_type TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_oauth_tokens_hash ON public.oauth_tokens (token_hash)
WHERE revoked_at IS NULL;
CREATE TABLE IF NOT EXISTS public.oauth_installations (
    app_id UUID NOT NULL REFERENCES public.oauth_apps(id) ON DELETE CASCADE,
    org_id UUID NOT NULL,
    installed_by UUID NOT NULL,
    scopes TEXT [] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (app_id, org_id)
);
