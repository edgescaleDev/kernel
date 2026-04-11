-- Kernel migration rollback: clear module registry and activation data.
DROP TABLE IF EXISTS public.module_activations;
DROP TABLE IF EXISTS public.module_registry;
