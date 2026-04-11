-- Kernel migration rollback: remove OAuth2 app management.
-- Dropping the tables completely to remove the schema and all associated data.
DROP TABLE IF EXISTS public.oauth_installations;
DROP TABLE IF EXISTS public.oauth_tokens;
DROP TABLE IF EXISTS public.oauth_apps;
