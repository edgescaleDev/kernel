-- Kernel migration: module registry + per-tenant activation.
CREATE TABLE IF NOT EXISTS public.module_registry (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    type TEXT NOT NULL,
    schema_name TEXT NOT NULL,
    description TEXT,
    depends_on TEXT [],
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS public.module_activations (
    module_id TEXT NOT NULL,
    tenant_id UUID NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    config JSONB NOT NULL DEFAULT '{}',
    activated_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (module_id, tenant_id)
);
