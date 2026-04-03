-- Kernel migration: hash-chained audit events.
CREATE TABLE IF NOT EXISTS public.audit_events (
    id BIGSERIAL PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id UUID,
    org_id UUID,
    module_id TEXT NOT NULL,
    action TEXT NOT NULL,
    resource TEXT NOT NULL,
    resource_id TEXT,
    changes JSONB,
    ip_address INET,
    user_agent TEXT,
    request_id TEXT,
    prev_hash TEXT,
    hash TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_events_org ON public.audit_events (org_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_resource ON public.audit_events (resource, resource_id);
