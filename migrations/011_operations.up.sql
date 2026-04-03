-- Kernel migration: long-running operations.
CREATE TABLE IF NOT EXISTS public.operations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    module_id TEXT NOT NULL,
    org_id UUID NOT NULL,
    user_id UUID NOT NULL,
    type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    progress INT NOT NULL DEFAULT 0,
    total_items INT NOT NULL DEFAULT 0,
    result JSONB,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_operations_org_status ON public.operations (org_id, status);
CREATE INDEX IF NOT EXISTS idx_operations_module ON public.operations (module_id, created_at DESC);
