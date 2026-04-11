-- Kernel migration rollback: clear hash-chained audit events.
DROP TABLE IF NOT EXISTS public.audit_events;
