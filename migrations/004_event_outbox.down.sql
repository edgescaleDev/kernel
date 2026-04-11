-- Kernel migration rollback: drops transactional event outbox table.
CREATE TABLE IF NOT EXISTS public.event_outbox
