-- Kernel migration rollback: remove data retention policies.
-- Dropping the table completely to remove the schema and all associated data.
DROP TABLE IF EXISTS public.retention_policies;
