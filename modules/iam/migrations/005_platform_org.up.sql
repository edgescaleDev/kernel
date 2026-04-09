-- Ensure only one platform org can exist.
CREATE UNIQUE INDEX IF NOT EXISTS uq_organizations_platform ON module_iam.organizations (status)
WHERE status = 'platform';
-- Seed the platform org.
INSERT INTO module_iam.organizations (name, slug, status)
VALUES ('Platform', 'platform', 'platform') ON CONFLICT (slug) DO NOTHING;
-- Seed system roles for the platform org.
DO $$
DECLARE platform_org_id UUID;
admin_role_id UUID;
viewer_role_id UUID;
BEGIN
SELECT id INTO platform_org_id
FROM module_iam.organizations
WHERE status = 'platform';
-- platform_admin: full cross-org access.
INSERT INTO module_iam.roles (org_id, name, description, is_system)
VALUES (
        platform_org_id,
        'platform_admin',
        'Full platform administration access',
        true
    ) ON CONFLICT (org_id, name) DO NOTHING
RETURNING id INTO admin_role_id;
-- platform_viewer: read-only cross-org access.
INSERT INTO module_iam.roles (org_id, name, description, is_system)
VALUES (
        platform_org_id,
        'platform_viewer',
        'Read-only platform access',
        true
    ) ON CONFLICT (org_id, name) DO NOTHING
RETURNING id INTO viewer_role_id;
-- Grant wildcard permission to platform_admin.
IF admin_role_id IS NOT NULL THEN
INSERT INTO module_iam.role_permissions (role_id, permission_key)
VALUES (admin_role_id, '*') ON CONFLICT (role_id, permission_key) DO NOTHING;
END IF;
END $$;
