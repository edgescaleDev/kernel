-- Kernel migration rollback: drop webhook endpoints and delivery tracking tables.
DROP TABLE IF EXISTS public.webhook_endpoints;
DROP TABLE IF EXISTS public.webhook_deliveries;
