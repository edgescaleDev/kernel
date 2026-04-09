-- Move platform tables back to module_iam schema
ALTER TABLE public.users
SET SCHEMA module_iam;
ALTER TABLE public.organizations
SET SCHEMA module_iam;
ALTER TABLE public.org_members
SET SCHEMA module_iam;
