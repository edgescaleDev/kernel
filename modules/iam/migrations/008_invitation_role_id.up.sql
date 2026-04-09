-- Replace role slug with role_id UUID for immutability and custom role support.
ALTER TABLE module_iam.org_invitations
ADD COLUMN role_id UUID REFERENCES module_iam.roles(id);
-- Migrate existing data: resolve slug to role_id.
UPDATE module_iam.org_invitations inv
SET role_id = r.id
FROM module_iam.roles r
WHERE r.org_id = inv.org_id
    AND r.slug = inv.role;
-- Drop the old text column after migration.
ALTER TABLE module_iam.org_invitations DROP COLUMN role;
