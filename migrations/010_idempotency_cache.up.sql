-- Kernel migration: idempotency key cache (DB fallback for Redis).
CREATE TABLE IF NOT EXISTS public.idempotency_cache (
    idempotency_key TEXT NOT NULL,
    module_id TEXT NOT NULL,
    org_id UUID NOT NULL,
    http_status INT NOT NULL,
    response_body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (module_id, org_id, idempotency_key)
);
-- Periodic cleanup of expired entries.
CREATE INDEX IF NOT EXISTS idx_idempotency_cache_expires ON public.idempotency_cache (expires_at);
