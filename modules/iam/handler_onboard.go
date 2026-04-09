package iam

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.edgescale.dev/kernel/sdk"
)

func registerOnboardRoutes(m *Module, router *sdk.Router) {
	// Public endpoint because the user doesn't exist in our DB yet.
	// But it requires a valid IdP token, which we will validate manually.
	router.POST("/onboard", sdk.Public, sdk.RateLimit("onboard", 10, time.Minute, m.ctx.Redis.Client()), m.onboardUser)
}

type onboardUserRequest struct {
	Email           string                `json:"email"`
	Phone           string                `json:"phone"`
	Name            sdk.TranslatableField `json:"name" binding:"required"`
	InvitationToken string                `json:"invitation_token"`
}

func (m *Module) onboardUser(c *gin.Context) {
	header := c.GetHeader("Authorization")
	if header == "" {
		sdk.Error(c, sdk.Unauthorized("missing Authorization header"))
		return
	}

	token := header
	if len(header) > 7 && header[:7] == "Bearer " {
		token = header[7:]
	}

	identity, err := m.ctx.IdentityProvider.ValidateToken(c.Request.Context(), token)
	if err != nil {
		sdk.Error(c, sdk.Unauthorized("invalid or expired token"))
		return
	}

	var req onboardUserRequest
	if !sdk.BindAndValidate(c, &req) {
		return
	}

	tx := m.ctx.DB.Begin()

	user := User{
		ExternalID: identity.Subject,
		Provider:   identity.Provider,
		Email:      req.Email,
		Phone:      req.Phone,
		Name:       req.Name,
		Locale:     sdk.Locale(c),
	}
	if user.Provider == "" {
		user.Provider = "platform"
	}

	// Try to create the user structure.
	if err := tx.Create(&user).Error; err != nil {
		// If user already exists, we can still process the invitation token.
		if err := tx.Where("external_id = ? AND provider = ?", user.ExternalID, user.Provider).First(&user).Error; err != nil {
			tx.Rollback()
			sdk.Error(c, sdk.Internal("failed to onboard user"))
			return
		}
	} else {
		m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
			Action: sdk.AuditCreate, Resource: "user", ResourceID: user.ID.String(),
		})
		m.ctx.Bus.Publish(c.Request.Context(), "iam.user.created", user)
	}

	// If there is an invitation token, auto-create membership and assign roles.
	if req.InvitationToken != "" {
		var inv OrgInvitation
		if err := tx.Where("token = ? AND status = 'accepted'", hashToken(req.InvitationToken)).First(&inv).Error; err != nil {
			tx.Rollback()
			sdk.Error(c, sdk.BadRequest("invalid or unaccepted invitation token"))
			return
		}

		// Create org membership.
		member := OrgMember{
			OrgID:  inv.OrgID,
			UserID: user.ID,
		}
		if err := tx.Create(&member).Error; err == nil {
			m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
				Action: sdk.AuditCreate, Resource: "org_member", ResourceID: member.ID.String(),
			})
			m.ctx.Bus.Publish(c.Request.Context(), "iam.member.added", member)
		}

		// Assign the role specified in the invitation.
		var role Role
		if err := tx.Where("org_id = ? AND slug = ?", inv.OrgID, inv.Role).First(&role).Error; err == nil {
			userRole := UserRole{
				OrgID:  inv.OrgID,
				UserID: user.ID,
				RoleID: role.ID,
			}
			if err := tx.Create(&userRole).Error; err == nil {
				m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
					Action: sdk.AuditCreate, Resource: "user_roles", ResourceID: userRole.ID.String(),
				})
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		sdk.Error(c, sdk.Internal("failed to complete onboarding"))
		return
	}

	sdk.Created(c, user)
}
