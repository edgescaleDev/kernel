// Package iam implements the IAM core module for the edgescale kernel.
// It handles user profiles, organization management, membership,
// invitations, and access control. Authentication is delegated to the
// kernel's pluggable IdentityProvider — IAM consumes the canonical
// Identity from the request context.
package iam

import (
	"embed"
	"io/fs"

	"go.edgescale.dev/kernel/sdk"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Module implements sdk.Module for identity & access management.
type Module struct {
	ctx sdk.Context
}

func New() *Module { return &Module{} }

// Manifest returns immutable metadata for the IAM module.
func (m *Module) Manifest() sdk.Manifest {
	return sdk.Manifest{
		ID:          "iam",
		Name:        "Identity & Access Management",
		Version:     "0.1.0",
		Type:        sdk.TypeCore,
		Schema:      "public",
		Description: "Users, organizations, membership, and authorization.",
		Permissions: []sdk.Permission{
			{Key: "iam.users.read", Label: "View users"},
			{Key: "iam.users.manage", Label: "Manage users"},
			{Key: "iam.orgs.read", Label: "View organizations"},
			{Key: "iam.orgs.manage", Label: "Manage organizations"},
			{Key: "iam.members.read", Label: "View org members"},
			{Key: "iam.members.manage", Label: "Manage org members"},
			{Key: "iam.invitations.read", Label: "View invitations"},
			{Key: "iam.invitations.manage", Label: "Manage invitations"},
		},
		PublicEvents: []sdk.EventDef{
			{Subject: "iam.user.created", Description: "Fired when a new user is created"},
			{Subject: "iam.user.updated", Description: "Fired when a user profile is updated"},
			{Subject: "iam.user.deleted", Description: "Fired when a user is soft-deleted"},
			{Subject: "iam.member.added", Description: "Fired when a user is added to an organization"},
			{Subject: "iam.member.removed", Description: "Fired when a user is removed from an organization"},
			{Subject: "iam.invitation.created", Description: "Fired when an invitation is created"},
			{Subject: "iam.invitation.accepted", Description: "Fired when an invitation is accepted"},
		},
		Config: []sdk.ConfigFieldDef{
			{
				Key:         "allowed_sign_in_providers",
				Type:        "multiselect",
				Default:     []string{"phone"},
				Label:       "Allowed sign-in methods",
				Description: "Sign-in providers permitted for this org. Tokens from unlisted providers are rejected.",
				Options: []sdk.ConfigOption{
					{Value: "phone", Label: "Phone"},
					{Value: "password", Label: "Email/Password"},
					{Value: "google.com", Label: "Google"},
					{Value: "apple.com", Label: "Apple"},
					{Value: "facebook.com", Label: "Facebook"},
					{Value: "twitter.com", Label: "Twitter"},
					{Value: "github.com", Label: "GitHub"},
					{Value: "microsoft.com", Label: "Microsoft"},
				},
				Group: "Authentication",
			},
		},
	}
}

// Migrations returns the embedded SQL migration files.
func (m *Module) Migrations() fs.FS {
	sub, _ := fs.Sub(migrations, "migrations")
	return sub
}

// Init stores the wired context and registers the IAMReader for cross-module access.
func (m *Module) Init(ctx sdk.Context) error {
	m.ctx = ctx
	ctx.RegisterReader(&iamReader{ctx: ctx})
	ctx.Logger.Info("iam module initialised")
	return nil
}

// RegisterRoutes mounts IAM HTTP endpoints.
func (m *Module) RegisterRoutes(router *sdk.Router) {
	registerUserRoutes(m, router)
	registerOrgRoutes(m, router)
	registerMemberRoutes(m, router)
	registerInvitationRoutes(m, router)
}

// RegisterEvents subscribes to relevant event bus subjects.
func (m *Module) RegisterEvents(bus sdk.EventBus) {}

// RegisterHooks registers sync interceptors for IAM lifecycle events.
func (m *Module) RegisterHooks(hooks *sdk.HookRegistry) {}

// RegisterWorkflows registers Temporal workflows.
func (m *Module) RegisterWorkflows(reg sdk.WorkflowRegistry) {}

// RegisterActivities registers Temporal activities.
func (m *Module) RegisterActivities(reg sdk.ActivityRegistry) {}

// Shutdown performs graceful cleanup.
func (m *Module) Shutdown() error { return nil }
