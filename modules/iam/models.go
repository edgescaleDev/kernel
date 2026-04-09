package iam

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"go.edgescale.dev/kernel/sdk"
	"gorm.io/gorm"
)

// ---- Users ----------------------------------------------------------------

// User represents a platform-level identity mapped from an external IdP subject.
// Users are not scoped to an org — org membership is tracked via OrgMember.
type User struct {
	ID         uuid.UUID             `json:"id"          gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ExternalID string                `json:"external_id" gorm:"not null"`
	Provider   string                `json:"provider"    gorm:"not null;default:'platform'"`
	Email      string                `json:"email"       gorm:"not null;default:''"`
	Phone      string                `json:"phone"       gorm:"not null;default:''"`
	Name       sdk.TranslatableField `json:"name"        gorm:"type:jsonb;not null;default:'{}'"`
	AvatarURL  string                `json:"avatar_url"  gorm:"not null;default:''"`
	Locale     string                `json:"locale"      gorm:"not null;default:'en'"`
	Timezone   string                `json:"timezone"    gorm:"not null;default:'UTC'"`
	Status     string                `json:"status"      gorm:"not null;default:'active'"`
	Metadata   json.RawMessage       `json:"metadata"    gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt  time.Time             `json:"created_at"  gorm:"autoCreateTime"`
	UpdatedAt  time.Time             `json:"updated_at"  gorm:"autoUpdateTime"`
	DeletedAt  gorm.DeletedAt        `json:"deleted_at,omitempty" gorm:"index"`
}

func (User) TableName() string { return "public.users" }

// ErasePersonalData anonymises PII for GDPR compliance.
func (u *User) ErasePersonalData() error {
	u.Email = "erased@deleted.local"
	u.Phone = ""
	u.Name = sdk.TranslatableField{"en": "Deleted User"}
	u.AvatarURL = ""
	u.Metadata = json.RawMessage(`{}`)
	return nil
}

// ---- Organizations --------------------------------------------------------

// Organization represents a tenant on the platform.
type Organization struct {
	ID        uuid.UUID             `json:"id"         gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name      sdk.TranslatableField `json:"name"       gorm:"type:jsonb;not null;default:'{}'"`
	Slug      string                `json:"slug"       gorm:"not null;uniqueIndex"`
	ParentID  *uuid.UUID            `json:"parent_id,omitempty" gorm:"type:uuid;index"`
	LogoURL   string                `json:"logo_url"   gorm:"not null;default:''"`
	Status    string                `json:"status"     gorm:"not null;default:'active'"`
	Metadata  json.RawMessage       `json:"metadata"   gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt time.Time             `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time             `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt        `json:"deleted_at" gorm:"index"`
}

func (Organization) TableName() string { return "public.organizations" }

// ---- Membership -----------------------------------------------------------

// OrgMember represents a user's membership within an organization.
// Membership is purely about belonging — authorization is handled by
// user_roles → roles → role_permissions.
type OrgMember struct {
	ID        uuid.UUID      `json:"id"         gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID     uuid.UUID      `json:"org_id"     gorm:"type:uuid;not null;index"`
	UserID    uuid.UUID      `json:"user_id"    gorm:"type:uuid;not null;index"`
	JoinedAt  time.Time      `json:"joined_at"  gorm:"not null"`
	CreatedAt time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

func (OrgMember) TableName() string { return "public.org_members" }

// OrgInvitation tracks pending invitations to an organization.
// Channel determines the delivery method (email, sms, whatsapp).
// Recipient is the destination address (email address, phone number).
type OrgInvitation struct {
	ID         uuid.UUID  `json:"id"          gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID      uuid.UUID  `json:"org_id"      gorm:"type:uuid;not null;index"`
	Channel    string     `json:"channel"     gorm:"not null;default:'email'"`
	Recipient  string     `json:"recipient"   gorm:"not null"`
	RoleID     uuid.UUID  `json:"role_id"     gorm:"type:uuid"`
	InvitedBy  uuid.UUID  `json:"invited_by"  gorm:"type:uuid;not null"`
	Token      string     `json:"-"           gorm:"not null"`
	Status     string     `json:"status"      gorm:"not null;default:'pending'"`
	ExpiresAt  time.Time  `json:"expires_at"  gorm:"not null"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"  gorm:"autoCreateTime"`
	UpdatedAt  time.Time  `json:"updated_at"  gorm:"autoUpdateTime"`
}

func (OrgInvitation) TableName() string { return "module_iam.org_invitations" }

// ---- RBAC -----------------------------------------------------------------

// Role represents an org-scoped role that aggregates permissions.
// System roles (IsSystem=true) are seeded by the kernel and cannot be deleted.
type Role struct {
	ID          uuid.UUID             `json:"id"          gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID       uuid.UUID             `json:"org_id"      gorm:"type:uuid;not null;index"`
	Name        sdk.TranslatableField `json:"name"        gorm:"type:jsonb;not null;default:'{}'"`
	Slug        string                `json:"slug"        gorm:"not null;uniqueIndex:idx_roles_org_slug"`
	Description sdk.TranslatableField `json:"description" gorm:"type:jsonb;not null;default:'{}'"`
	IsSystem    bool                  `json:"is_system"   gorm:"not null;default:false"`
	CreatedAt   time.Time             `json:"created_at"  gorm:"autoCreateTime"`
	UpdatedAt   time.Time             `json:"updated_at"  gorm:"autoUpdateTime"`
	DeletedAt   gorm.DeletedAt        `json:"deleted_at,omitempty" gorm:"index"`

	// Eager-loaded associations.
	Permissions []RolePermission `json:"permissions,omitempty" gorm:"foreignKey:RoleID"`
}

func (Role) TableName() string { return "module_iam.roles" }

// RolePermission maps a role to a permission key declared in a module manifest.
// The PermissionKey is a string like "iam.users.read" — not a FK to a DB table.
type RolePermission struct {
	ID            uuid.UUID `json:"id"             gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	RoleID        uuid.UUID `json:"role_id"        gorm:"type:uuid;not null;index"`
	PermissionKey string    `json:"permission_key" gorm:"not null"`
	CreatedAt     time.Time `json:"created_at"     gorm:"autoCreateTime"`
}

func (RolePermission) TableName() string { return "module_iam.role_permissions" }

// UserRole assigns a role to a user within an org. A user can have multiple roles.
type UserRole struct {
	ID        uuid.UUID `json:"id"         gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID     uuid.UUID `json:"org_id"     gorm:"type:uuid;not null;index"`
	UserID    uuid.UUID `json:"user_id"    gorm:"type:uuid;not null"`
	RoleID    uuid.UUID `json:"role_id"    gorm:"type:uuid;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (UserRole) TableName() string { return "module_iam.user_roles" }
