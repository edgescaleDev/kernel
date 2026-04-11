-- Kernel migration rollback: clear schema_migrations tracking data.
-- We truncate rather than drop to preserve the table structure,
-- maintaining compatibility with GORM AutoMigrate expectations.
TRUNCATE TABLE public.schema_migrations;
