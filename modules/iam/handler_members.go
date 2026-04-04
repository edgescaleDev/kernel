package iam

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.edgescale.dev/kernel/sdk"
)

// registerMemberRoutes mounts membership endpoints under /v1/iam/.
func registerMemberRoutes(m *Module, router *sdk.Router) {
	router.GET("/members", "iam.members.read", m.listMembers)
	router.POST("/members", "iam.members.manage", m.addMember)
	router.DELETE("/members/:id", "iam.members.manage", m.removeMember)
	router.PATCH("/members/:id", "iam.members.manage", m.updateMemberRole)
}

// ---- request DTOs ----------------------------------------------------------

type addMemberRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
	Role   string    `json:"role"    binding:"required,oneof=owner admin member"`
}

type updateMemberRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=owner admin member"`
}

// ---- handlers --------------------------------------------------------------

func (m *Module) listMembers(c *gin.Context) {
	oid := orgID(c)
	page := sdk.ParsePageRequest(c)

	result, err := sdk.Paginate[OrgMember](
		m.ctx.DB.Where("org_id = ?", oid),
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
	member := OrgMember{
		OrgID:  oid,
		UserID: req.UserID,
		Role:   req.Role,
	}

	if err := m.ctx.DB.Create(&member).Error; err != nil {
		sdk.Error(c, sdk.Conflict("user is already a member of this organization"))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditCreate, Resource: "org_member", ResourceID: member.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.member.added", member)

	sdk.Created(c, member)
}

func (m *Module) removeMember(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
		return
	}

	oid := orgID(c)
	result := m.ctx.DB.Where("id = ? AND org_id = ?", uri.ID, oid).Delete(&OrgMember{})
	if result.RowsAffected == 0 {
		sdk.Error(c, sdk.NotFound("member", uri.ID))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditDelete, Resource: "org_member", ResourceID: uri.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.member.removed", gin.H{"id": uri.ID, "org_id": oid})

	sdk.NoContent(c)
}

func (m *Module) updateMemberRole(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
		return
	}
	var req updateMemberRoleRequest
	if !sdk.BindAndValidate(c, &req) {
		return
	}

	oid := orgID(c)
	var member OrgMember
	if err := m.ctx.DB.Where("id = ? AND org_id = ?", uri.ID, oid).First(&member).Error; err != nil {
		sdk.Error(c, sdk.NotFound("member", uri.ID))
		return
	}

	if err := m.ctx.DB.Model(&member).Update("role", req.Role).Error; err != nil {
		sdk.Error(c, sdk.BadRequest(err.Error()))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditUpdate, Resource: "org_member", ResourceID: member.ID.String(),
	})

	sdk.OK(c, member)
}
