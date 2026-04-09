-- Composite index for the common query pattern: list invitations by org + status.
CREATE INDEX IF NOT EXISTS idx_org_invitations_org_status ON module_iam.org_invitations (org_id, status);
