-- Kernel migration: add trial expiry to module activations.
ALTER TABLE public.module_activations
ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
-- Partial index for the reaper cron: only scan active rows with an expiry.
CREATE INDEX IF NOT EXISTS idx_activations_expiring ON public.module_activations (expires_at)
WHERE active = true
  AND expires_at IS NOT NULL;
