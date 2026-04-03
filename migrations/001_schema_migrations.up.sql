-- Kernel migration: schema_migrations tracking table.
-- This table is usually bootstrapped by GORM AutoMigrate,
-- but we include it here for explicit schema management.
CREATE TABLE IF NOT EXISTS public.schema_migrations (
    module_id TEXT NOT NULL,
    version INT NOT NULL,
    filename TEXT NOT NULL,
    checksum TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (module_id, version)
);
