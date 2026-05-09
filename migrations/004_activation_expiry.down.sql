-- Rollback: remove trial expiry from module activations.
DROP INDEX IF EXISTS idx_activations_expiring;
ALTER TABLE public.module_activations DROP COLUMN IF EXISTS expires_at;
