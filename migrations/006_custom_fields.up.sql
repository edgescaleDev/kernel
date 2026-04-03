-- Kernel migration: EAV custom fields engine.
CREATE TABLE IF NOT EXISTS public.custom_field_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL,
    entity_type TEXT NOT NULL,
    field_key TEXT NOT NULL,
    field_type TEXT NOT NULL,
    label JSONB NOT NULL,
    placeholder JSONB,
    required BOOLEAN NOT NULL DEFAULT FALSE,
    options JSONB,
    sort_order INT NOT NULL DEFAULT 0,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, entity_type, field_key)
);
CREATE TABLE IF NOT EXISTS public.custom_field_values (
    entity_type TEXT NOT NULL,
    entity_id UUID NOT NULL,
    org_id UUID NOT NULL,
    field_key TEXT NOT NULL,
    value JSONB NOT NULL,
    PRIMARY KEY (entity_type, entity_id, field_key)
);
CREATE INDEX IF NOT EXISTS idx_custom_field_values_org ON public.custom_field_values (org_id, entity_type);
