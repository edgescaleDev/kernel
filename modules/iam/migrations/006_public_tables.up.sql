-- Move platform tables from module_iam to public schema
ALTER TABLE module_iam.users
SET SCHEMA public;
ALTER TABLE module_iam.organizations
SET SCHEMA public;
ALTER TABLE module_iam.org_members
SET SCHEMA public;
