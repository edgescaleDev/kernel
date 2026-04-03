-- Kernel migration: per-org feature flags.
CREATE TABLE IF NOT EXISTS public.feature_flags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL,
    flag_key TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    metadata JSONB,
    UNIQUE (org_id, flag_key)
);
