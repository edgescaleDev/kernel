-- The existing partial index on (token) WHERE status = 'pending' doesn't cover
-- the onboardUser query which filters by status = 'accepted'. Add a matching index.
CREATE INDEX IF NOT EXISTS idx_org_invitations_token_accepted ON module_iam.org_invitations (token)
WHERE status = 'accepted';
