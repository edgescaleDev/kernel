package iam

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.edgescale.dev/kernel/sdk"
)

// registerMemberRoutes mounts membership endpoints under /v1/iam/.
// Membership is about belonging to an org — authorization is handled
// separately via the roles/user_roles system.
func registerMemberRoutes(m *Module, router *sdk.Router) {
	router.GET("/members", "iam.members.read", m.listMembers)
	router.POST("/members", "iam.members.manage", m.addMember)
	router.DELETE("/members/:id", "iam.members.manage", m.removeMember)
}

// ---- request DTOs ----------------------------------------------------------

type addMemberRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
}

// ---- handlers --------------------------------------------------------------

func (m *Module) listMembers(c *gin.Context) {
	oid := orgID(c)
	page := sdk.ParsePageRequest(c)

	result, err := sdk.Paginate[OrgMember](
		m.ctx.DB.Preload("User").Where("org_id = ?", oid),
		page,
	)
	if err != nil {
		sdk.Error(c, sdk.BadRequest(err.Error()))
		return
	}
	sdk.List(c, result.Items, result.Meta)
}

func (m *Module) addMember(c *gin.Context) {
	var req addMemberRequest
	if !sdk.BindAndValidate(c, &req) {
		return
	}

	oid := orgID(c)

	// Verify that the user exists before creating membership.
	var userExists int64
	m.ctx.DB.Model(&User{}).Where("id = ?", req.UserID).Count(&userExists)
	if userExists == 0 {
		sdk.Error(c, sdk.NotFound("user", req.UserID))
		return
	}

	member := OrgMember{
		OrgID:    oid,
		UserID:   req.UserID,
		JoinedAt: time.Now(),
	}

	if err := m.ctx.DB.Create(&member).Error; err != nil {
		sdk.Error(c, sdk.Conflict("user is already a member of this organization"))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditCreate, Resource: "org_member", ResourceID: member.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.member.added", member)

	if m.ctx.Redis.Client() != nil {
		m.ctx.Redis.Del(c.Request.Context(), fmt.Sprintf("user_org_membership:%s:%s", req.UserID, oid))
	}

	sdk.Created(c, member)
}

func (m *Module) removeMember(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
		return
	}

	oid := orgID(c)

	// Fetch the member first to get the associated UserID.
	var member OrgMember
	if err := m.ctx.DB.Where("id = ? AND org_id = ?", uri.ID, oid).First(&member).Error; err != nil {
		sdk.Error(c, sdk.NotFound("member", uri.ID))
		return
	}

	if err := m.removeMembership(c, member.UserID, oid); err != nil {
		return // error already sent to client
	}
	sdk.NoContent(c)
}

// removeMembership is the shared core for both deleteUser (by user_id) and
// removeMember (by member_id). It handles self-removal prevention, transactional
// cleanup of membership + user_roles, audit, events, and cache invalidation.
// Returns an error if the operation failed (response already sent to client).
func (m *Module) removeMembership(c *gin.Context, userID, oid uuid.UUID) error {
	// Prevent self-removal.
	sub := userSubject(c)
	provider := c.GetString("auth_provider")
	var caller User
	if m.ctx.DB.Where("external_id = ? AND provider = ?", sub, provider).First(&caller).Error == nil {
		if caller.ID == userID {
			sdk.Error(c, sdk.BadRequest("cannot remove yourself from the organization"))
			return fmt.Errorf("self-removal")
		}
	}

	// Lookup the membership.
	var member OrgMember
	if err := m.ctx.DB.Where("user_id = ? AND org_id = ?", userID, oid).First(&member).Error; err != nil {
		sdk.Error(c, sdk.NotFound("user", userID))
		return err
	}

	// Transactionally remove membership and associated roles.
	tx := m.ctx.DB.Begin()
	if err := tx.Delete(&member).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to remove member"))
		return err
	}
	if err := tx.Where("user_id = ? AND org_id = ?", userID, oid).Delete(&UserRole{}).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to clean up member roles"))
		return err
	}
	if err := tx.Commit().Error; err != nil {
		sdk.Error(c, sdk.Internal("transaction failed"))
		return err
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditDelete, Resource: "org_member", ResourceID: member.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.member.removed", gin.H{
		"id": member.ID, "org_id": oid, "user_id": userID,
	})

	// Cache invalidation.
	if m.ctx.Redis.Client() != nil {
		ctx := c.Request.Context()
		m.ctx.Redis.Del(ctx, fmt.Sprintf("user_org_membership:%s:%s", userID, oid))
		var user User
		if m.ctx.DB.Where("id = ?", userID).First(&user).Error == nil {
			m.ctx.Redis.Client().Del(ctx, "middleware_user:"+user.ExternalID+":"+oid.String())
		}
	}

	return nil
}
