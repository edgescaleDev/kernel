-- Kernel migration rollback: remove idempotency key cache.
-- Dropping the table completely to remove the schema and all associated data.
DROP TABLE IF EXISTS public.idempotency_cache;
