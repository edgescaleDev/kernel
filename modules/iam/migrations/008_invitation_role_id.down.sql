-- Reverse: re-add role slug column, populate from role_id, drop role_id.
ALTER TABLE module_iam.org_invitations
ADD COLUMN role TEXT NOT NULL DEFAULT 'member';
UPDATE module_iam.org_invitations inv
SET role = r.slug
FROM module_iam.roles r
WHERE r.id = inv.role_id;
ALTER TABLE module_iam.org_invitations DROP COLUMN role_id;
