CREATE SCHEMA IF NOT EXISTS module_iam;
CREATE TABLE IF NOT EXISTS public.organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name JSONB NOT NULL DEFAULT '{}',
    slug TEXT NOT NULL UNIQUE,
    parent_id UUID REFERENCES public.organizations(id),
    logo_url TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_organizations_slug ON public.organizations (slug)
WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_organizations_parent_id ON public.organizations (parent_id)
WHERE parent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_organizations_status ON public.organizations (status)
WHERE deleted_at IS NULL;
