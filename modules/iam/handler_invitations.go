package iam

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.edgescale.dev/kernel/sdk"
)

// registerInvitationRoutes mounts invitation endpoints under /v1/iam/.
func registerInvitationRoutes(m *Module, router *sdk.Router) {
	router.GET("/invitations", "iam.invitations.read", m.listInvitations)
	router.POST("/invitations", "iam.invitations.manage", m.createInvitation)
	router.DELETE("/invitations/:id", "iam.invitations.manage", m.revokeInvitation)

	// Public endpoint — the user accepting an invitation may not be authenticated yet.
	router.POST("/invitations/accept", sdk.Public, sdk.RateLimit("accept", 10, time.Minute, m.ctx.Redis.Client()), m.acceptInvitation)
}

// ---- request DTOs ----------------------------------------------------------

type createInvitationRequest struct {
	Channel   string `json:"channel"   binding:"required,oneof=email sms whatsapp"`
	Recipient string `json:"recipient" binding:"required,max=320"`
	Role      string `json:"role"      binding:"required,oneof=owner admin member"`
}

type acceptInvitationRequest struct {
	Token string `json:"token" binding:"required"`
}

// ---- handlers --------------------------------------------------------------

func (m *Module) listInvitations(c *gin.Context) {
	oid := orgID(c)
	page := sdk.ParsePageRequest(c)

	result, err := sdk.Paginate[OrgInvitation](
		m.ctx.DB.Where("org_id = ?", oid),
		page,
	)
	if err != nil {
		sdk.Error(c, sdk.BadRequest(err.Error()))
		return
	}
	sdk.List(c, result.Items, result.Meta)
}

func (m *Module) createInvitation(c *gin.Context) {
	var req createInvitationRequest
	if !sdk.BindAndValidate(c, &req) {
		return
	}

	oid := orgID(c)

	// Resolve the inviting user's internal ID from their IdP subject.
	sub := userSubject(c)
	provider := c.GetString("auth_provider")
	var inviter User
	if err := m.ctx.DB.Where(
		"external_id = ? AND provider = ?", sub, provider,
	).First(&inviter).Error; err != nil {
		sdk.Error(c, sdk.Unauthorized("inviting user not found"))
		return
	}

	token, err := generateSecureToken()
	if err != nil {
		sdk.Error(c, sdk.Internal("failed to generate invitation token"))
		return
	}

	inv := OrgInvitation{
		OrgID:     oid,
		Channel:   req.Channel,
		Recipient: req.Recipient,
		Role:      req.Role,
		InvitedBy: inviter.ID,
		Token:     hashToken(token),
		ExpiresAt: defaultExpiresAt(),
	}

	// Check for an existing invitation to allow re-sends.
	var existing OrgInvitation
	if err := m.ctx.DB.Where(
		"org_id = ? AND channel = ? AND recipient = ?", oid, req.Channel, req.Recipient,
	).First(&existing).Error; err == nil {
		// Only allow re-send if the previous invitation is no longer active.
		if existing.Status == "pending" {
			sdk.Error(c, sdk.Conflict("a pending invitation already exists for this recipient"))
			return
		}
		// Re-use the existing row: reset token, status, expiry, and role.
		updates := map[string]any{
			"token":      inv.Token,
			"status":     "pending",
			"role":       inv.Role,
			"invited_by": inv.InvitedBy,
			"expires_at": inv.ExpiresAt,
		}
		if err := m.ctx.DB.Model(&existing).Updates(updates).Error; err != nil {
			sdk.Error(c, sdk.Internal("failed to re-send invitation"))
			return
		}
		inv = existing
		inv.Status = "pending"
		inv.Role = req.Role
	} else {
		if err := m.ctx.DB.Create(&inv).Error; err != nil {
			sdk.Error(c, sdk.Internal("failed to create invitation"))
			return
		}
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditCreate, Resource: "org_invitation", ResourceID: inv.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.invitation.created", inv)

	// Return the raw token in the response so the caller can deliver it.
	// The model's Token field is json:"-" (stores the hash), so we overlay it.
	sdk.Created(c, gin.H{
		"id":         inv.ID,
		"org_id":     inv.OrgID,
		"channel":    inv.Channel,
		"recipient":  inv.Recipient,
		"role":       inv.Role,
		"invited_by": inv.InvitedBy,
		"token":      token,
		"status":     inv.Status,
		"expires_at": inv.ExpiresAt,
		"created_at": inv.CreatedAt,
	})
}

func (m *Module) revokeInvitation(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
		return
	}

	oid := orgID(c)
	result := m.ctx.DB.Model(&OrgInvitation{}).
		Where("id = ? AND org_id = ? AND status = 'pending'", uri.ID, oid).
		Update("status", "revoked")
	if result.Error != nil {
		sdk.Error(c, sdk.Internal("failed to revoke invitation"))
		return
	}
	if result.RowsAffected == 0 {
		sdk.Error(c, sdk.NotFound("invitation", uri.ID))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditDelete, Resource: "org_invitation", ResourceID: uri.ID.String(),
	})

	sdk.NoContent(c)
}

// acceptInvitation is a public endpoint (no auth required).
// Validates the token, checks expiry, marks the invitation as accepted,
// and returns the org details so the client can proceed with onboarding.
func (m *Module) acceptInvitation(c *gin.Context) {
	var req acceptInvitationRequest
	if !sdk.BindAndValidate(c, &req) {
		return
	}

	tx := m.ctx.DB.Begin()

	var inv OrgInvitation
	if err := tx.Set("gorm:query_option", "FOR UPDATE SKIP LOCKED").
		Where("token = ? AND status = 'pending'", hashToken(req.Token)).
		First(&inv).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.NotFound("invitation", "token"))
		return
	}

	// Check expiry.
	if time.Now().After(inv.ExpiresAt) {
		tx.Model(&inv).Update("status", "expired")
		tx.Commit()
		sdk.Error(c, sdk.BadRequest("invitation has expired"))
		return
	}

	// Mark as accepted.
	now := time.Now()
	if err := tx.Model(&inv).Updates(map[string]any{
		"status":      "accepted",
		"accepted_at": &now,
	}).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to accept invitation"))
		return
	}

	if err := tx.Commit().Error; err != nil {
		sdk.Error(c, sdk.Internal("failed to accept invitation"))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditActivate, Resource: "org_invitation", ResourceID: inv.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.invitation.accepted", inv)

	sdk.OK(c, gin.H{
		"org_id": inv.OrgID,
		"role":   inv.Role,
	})
}
