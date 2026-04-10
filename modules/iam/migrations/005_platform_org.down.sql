-- Remove seeded platform data in reverse dependency order.
DO $$
DECLARE platform_org_id UUID;
BEGIN
SELECT id INTO platform_org_id
FROM public.organizations
WHERE status = 'platform';
IF platform_org_id IS NOT NULL THEN -- Remove role_permissions for platform roles.
DELETE FROM module_iam.role_permissions
WHERE role_id IN (
        SELECT id
        FROM module_iam.roles
        WHERE org_id = platform_org_id
    );
-- Remove platform roles.
DELETE FROM module_iam.roles
WHERE org_id = platform_org_id;
-- Remove any platform org memberships.
DELETE FROM public.org_members
WHERE org_id = platform_org_id;
-- Remove the platform org itself.
DELETE FROM public.organizations
WHERE id = platform_org_id;
END IF;
END $$;
-- Remove the partial unique index.
DROP INDEX IF EXISTS public.uq_organizations_platform;
