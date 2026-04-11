-- Kernel migration rollback: remove long-running operations tracking.
-- Dropping the table completely to remove the schema and all associated data.
DROP TABLE IF EXISTS public.operations;
