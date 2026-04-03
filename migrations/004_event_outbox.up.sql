-- Kernel migration: transactional event outbox.
CREATE TABLE IF NOT EXISTS public.event_outbox (
    id BIGSERIAL PRIMARY KEY,
    module_id TEXT NOT NULL,
    org_id UUID,
    subject TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_event_outbox_pending ON public.event_outbox (created_at)
WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_event_outbox_org ON public.event_outbox (org_id)
WHERE status = 'pending';
