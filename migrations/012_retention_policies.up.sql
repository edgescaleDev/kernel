-- Kernel migration: data retention policies.
CREATE TABLE IF NOT EXISTS public.retention_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    module_id TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    retention INTERVAL NOT NULL,
    action TEXT NOT NULL DEFAULT 'soft_delete',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (module_id, entity_type)
);
