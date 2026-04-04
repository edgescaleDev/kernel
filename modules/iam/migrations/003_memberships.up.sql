CREATE TABLE IF NOT EXISTS public.org_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_org_members_org_user UNIQUE (org_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_org_members_org_id ON public.org_members (org_id);
CREATE INDEX IF NOT EXISTS idx_org_members_user_id ON public.org_members (user_id);
CREATE INDEX IF NOT EXISTS idx_org_members_role ON public.org_members (org_id, role)
WHERE deleted_at IS NULL;
CREATE TABLE IF NOT EXISTS public.org_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
    channel TEXT NOT NULL DEFAULT 'email',
    recipient TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'member',
    invited_by UUID NOT NULL REFERENCES public.users(id),
    token TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_org_invitations_org_channel_recipient UNIQUE (org_id, channel, recipient)
);
CREATE INDEX IF NOT EXISTS idx_org_invitations_org_id ON public.org_invitations (org_id);
CREATE INDEX IF NOT EXISTS idx_org_invitations_token ON public.org_invitations (token)
WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_org_invitations_recipient ON public.org_invitations (recipient);
