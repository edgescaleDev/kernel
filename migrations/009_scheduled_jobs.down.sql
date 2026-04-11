-- Kernel migration rollback: remove cron-like scheduled jobs.
-- Dropping the table completely to remove the schema and all associated data.
DROP TABLE IF EXISTS public.scheduled_jobs;
