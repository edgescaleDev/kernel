-- Kernel migration rollback: remove EAV custom fields engine.
-- Dropping the tables completely to remove the schema and all associated data.
DROP TABLE IF EXISTS public.custom_field_values;
DROP TABLE IF EXISTS public.custom_field_definitions;
