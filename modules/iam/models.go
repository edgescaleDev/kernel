package iam

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ---- Users ----------------------------------------------------------------

// User represents a platform user mapped from an external IdP subject.
type User struct {
	ID         uuid.UUID       `json:"id"          gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID      uuid.UUID       `json:"org_id"      gorm:"type:uuid;not null;index"`
	ExternalID string          `json:"external_id" gorm:"not null"`
	Provider   string          `json:"provider"    gorm:"not null;default:'platform'"`
	Email      string          `json:"email"       gorm:"not null;default:''"`
	Phone      string          `json:"phone"       gorm:"not null;default:''"`
	Name       string          `json:"name"        gorm:"not null;default:''"`
	AvatarURL  string          `json:"avatar_url"  gorm:"not null;default:''"`
	Locale     string          `json:"locale"      gorm:"not null;default:'en'"`
	Timezone   string          `json:"timezone"    gorm:"not null;default:'UTC'"`
	Status     string          `json:"status"      gorm:"not null;default:'active'"`
	Metadata   json.RawMessage `json:"metadata"    gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt  time.Time       `json:"created_at"  gorm:"autoCreateTime"`
	UpdatedAt  time.Time       `json:"updated_at"  gorm:"autoUpdateTime"`
	DeletedAt  gorm.DeletedAt  `json:"deleted_at,omitempty" gorm:"index"`
}

func (User) TableName() string { return "public.users" }

// ErasePersonalData anonymises PII for GDPR compliance.
func (u *User) ErasePersonalData() error {
	u.Email = "erased@deleted.local"
	u.Phone = ""
	u.Name = "Deleted User"
	u.AvatarURL = ""
	u.Metadata = json.RawMessage(`{}`)
	return nil
}

// ---- Organizations --------------------------------------------------------

// Organization represents a tenant on the platform.
type Organization struct {
	ID        uuid.UUID       `json:"id"         gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name      string          `json:"name"       gorm:"not null"`
	Slug      string          `json:"slug"       gorm:"not null;uniqueIndex"`
	ParentID  *uuid.UUID      `json:"parent_id,omitempty" gorm:"type:uuid;index"`
	LogoURL   string          `json:"logo_url"   gorm:"not null;default:''"`
	Status    string          `json:"status"     gorm:"not null;default:'active'"`
	Metadata  json.RawMessage `json:"metadata"   gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt time.Time       `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time       `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt  `json:"deleted_at,omitempty" gorm:"index"`
}

func (Organization) TableName() string { return "public.organizations" }

// ---- Membership -----------------------------------------------------------

// OrgMember represents a user's membership within an organization.
type OrgMember struct {
	ID        uuid.UUID      `json:"id"         gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID     uuid.UUID      `json:"org_id"     gorm:"type:uuid;not null;index"`
	UserID    uuid.UUID      `json:"user_id"    gorm:"type:uuid;not null;index"`
	Role      string         `json:"role"       gorm:"not null;default:'member'"`
	JoinedAt  time.Time      `json:"joined_at"  gorm:"autoCreateTime"`
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
	Role       string     `json:"role"        gorm:"not null;default:'member'"`
	InvitedBy  uuid.UUID  `json:"invited_by"  gorm:"type:uuid;not null"`
	Token      string     `json:"-"           gorm:"not null"`
	Status     string     `json:"status"      gorm:"not null;default:'pending'"`
	ExpiresAt  time.Time  `json:"expires_at"  gorm:"not null"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"  gorm:"autoCreateTime"`
	UpdatedAt  time.Time  `json:"updated_at"  gorm:"autoUpdateTime"`
}

func (OrgInvitation) TableName() string { return "public.org_invitations" }
