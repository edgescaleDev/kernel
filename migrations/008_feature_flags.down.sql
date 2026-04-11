-- Kernel migration rollback: remove per-org feature flags.
-- Dropping the table completely to remove the schema and all associated data.
DROP TABLE IF EXISTS public.feature_flags;
