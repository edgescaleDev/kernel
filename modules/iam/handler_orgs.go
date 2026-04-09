package iam

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.edgescale.dev/kernel/sdk"
)

// registerOrgRoutes mounts organization-related endpoints under /v1/iam/.
func registerOrgRoutes(m *Module, router *sdk.Router) {
	router.GET("/orgs", sdk.Self, m.listOrgs)
	router.POST("/orgs", "iam.orgs.manage", sdk.RateLimit("create_org", 5, time.Hour, m.ctx.Redis.Client()), m.createOrg)
	router.POST("/orgs/register", sdk.Self, sdk.RateLimit("register_org", 5, time.Hour, m.ctx.Redis.Client()), m.registerOrg)
	router.GET("/orgs/:id", "iam.orgs.read", m.getOrg)
	router.PATCH("/orgs/:id", "iam.orgs.manage", m.updateOrg)
	router.DELETE("/orgs/:id", "iam.orgs.manage", m.deleteOrg)
}

// ---- request DTOs ----------------------------------------------------------

type createOrgRequest struct {
	Name sdk.TranslatableField `json:"name" binding:"required"`
	Slug string                `json:"slug" binding:"required"`
}

type updateOrgRequest struct {
	Name    *sdk.TranslatableField `json:"name"`
	LogoURL *string                `json:"logo_url"`
	Status  *string                `json:"status" binding:"omitempty,oneof=active suspended"`
}

// ---- handlers --------------------------------------------------------------

func (m *Module) listOrgs(c *gin.Context) {
	sub := userSubject(c)
	provider := c.GetString("auth_provider")
	page := sdk.ParsePageRequest(c)

	result, err := sdk.Paginate[Organization](
		m.ctx.DB.
			Joins("JOIN org_members ON org_members.org_id = organizations.id AND org_members.deleted_at IS NULL").
			Joins("JOIN users ON users.id = org_members.user_id AND users.deleted_at IS NULL").
			Where("users.external_id = ? AND users.provider = ?", sub, provider).
			Where("organizations.status != 'platform'"),
		page,
	)
	if err != nil {
		sdk.Error(c, sdk.BadRequest(err.Error()))
		return
	}
	sdk.List(c, result.Items, result.Meta)
}

func (m *Module) createOrg(c *gin.Context) {
	var req createOrgRequest
	if !sdk.BindAndValidate(c, &req) {
		return
	}

	// Resolve the creating user.
	sub := userSubject(c)
	provider := c.GetString("auth_provider")
	var creator User
	if err := m.ctx.DB.Where("external_id = ? AND provider = ?", sub, provider).First(&creator).Error; err != nil {
		sdk.Error(c, sdk.Unauthorized("creating user not found"))
		return
	}

	org := Organization{
		Name: req.Name,
		Slug: req.Slug,
	}

	tx := m.ctx.DB.Begin()
	if err := provisionOrgForUser(tx, &org, creator.ID); err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Conflict("organization with this slug already exists"))
		return
	}
	if err := tx.Commit().Error; err != nil {
		sdk.Error(c, sdk.Internal("failed to create organization"))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditCreate, Resource: "organization", ResourceID: org.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.org.created", org)

	sdk.Created(c, org)
}

func (m *Module) registerOrg(c *gin.Context) {
	var req createOrgRequest
	if !sdk.BindAndValidate(c, &req) {
		return
	}

	sub := userSubject(c)
	provider := c.GetString("auth_provider")
	var creator User
	if err := m.ctx.DB.Where("external_id = ? AND provider = ?", sub, provider).First(&creator).Error; err != nil {
		sdk.Error(c, sdk.Unauthorized("user not found"))
		return
	}

	org := Organization{
		Name: req.Name,
		Slug: req.Slug,
	}

	tx := m.ctx.DB.Begin()
	if err := provisionOrgForUser(tx, &org, creator.ID); err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Conflict("organization with this slug already exists"))
		return
	}
	if err := tx.Commit().Error; err != nil {
		sdk.Error(c, sdk.Internal("failed to create organization"))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditCreate, Resource: "organization", ResourceID: org.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.org.created", org)

	sdk.Created(c, org)
}

func (m *Module) getOrg(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
		return
	}

	if uri.ID != orgID(c) {
		sdk.Error(c, sdk.Forbidden("cannot access other organizations"))
		return
	}

	var org Organization
	if err := m.ctx.DB.Where("id = ?", uri.ID).First(&org).Error; err != nil {
		sdk.Error(c, sdk.NotFound("organization", uri.ID))
		return
	}
	sdk.OK(c, org)
}

func (m *Module) updateOrg(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
		return
	}
	var req updateOrgRequest
	if !sdk.BindAndValidate(c, &req) {
		return
	}

	if uri.ID != orgID(c) {
		sdk.Error(c, sdk.Forbidden("cannot access other organizations"))
		return
	}

	var org Organization
	if err := m.ctx.DB.Where("id = ?", uri.ID).First(&org).Error; err != nil {
		sdk.Error(c, sdk.NotFound("organization", uri.ID))
		return
	}
	if org.Status == "platform" {
		sdk.Error(c, sdk.Forbidden("platform organization cannot be modified"))
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.LogoURL != nil {
		updates["logo_url"] = *req.LogoURL
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	if len(updates) > 0 {
		if err := m.ctx.DB.Model(&org).Updates(updates).Error; err != nil {
			sdk.Error(c, sdk.BadRequest(err.Error()))
			return
		}
		m.ctx.DB.First(&org, org.ID)
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditUpdate, Resource: "organization", ResourceID: org.ID.String(),
	})

	sdk.OK(c, org)
}

func (m *Module) deleteOrg(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
		return
	}

	if uri.ID != orgID(c) {
		sdk.Error(c, sdk.Forbidden("cannot access other organizations"))
		return
	}

	var org Organization
	if err := m.ctx.DB.Where("id = ?", uri.ID).First(&org).Error; err != nil {
		sdk.Error(c, sdk.NotFound("organization", uri.ID))
		return
	}
	if org.Status == "platform" {
		sdk.Error(c, sdk.Forbidden("platform organization cannot be deleted"))
		return
	}

	// Collect affected users before cascade (data still exists).
	var affectedExternalIDs []string
	if m.ctx.Redis.Client() != nil {
		m.ctx.DB.Model(&User{}).
			Joins("JOIN org_members ON org_members.user_id = users.id AND org_members.deleted_at IS NULL").
			Where("org_members.org_id = ?", org.ID).
			Pluck("external_id", &affectedExternalIDs)
	}

	tx := m.ctx.DB.Begin()

	// Cascade cleanup: soft-delete doesn't trigger SQL ON DELETE CASCADE.
	// Remove in dependency order: user_roles → role_permissions → roles → invitations → members → org.
	if err := tx.Where("org_id = ?", org.ID).Delete(&UserRole{}).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to clean up user roles"))
		return
	}
	// Delete role_permissions for roles in this org.
	if err := tx.Where("role_id IN (?)",
		tx.Model(&Role{}).Select("id").Where("org_id = ?", org.ID),
	).Delete(&RolePermission{}).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to clean up role permissions"))
		return
	}
	if err := tx.Where("org_id = ?", org.ID).Delete(&Role{}).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to clean up roles"))
		return
	}
	if err := tx.Where("org_id = ?", org.ID).Delete(&OrgInvitation{}).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to clean up invitations"))
		return
	}
	if err := tx.Where("org_id = ?", org.ID).Delete(&OrgMember{}).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to clean up members"))
		return
	}
	if err := tx.Delete(&org).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to delete organization"))
		return
	}

	if err := tx.Commit().Error; err != nil {
		sdk.Error(c, sdk.Internal("transaction failed"))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditDelete, Resource: "organization", ResourceID: org.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.org.deleted", org)

	// Invalidate middleware cache for affected users.
	if len(affectedExternalIDs) > 0 {
		keys := make([]string, len(affectedExternalIDs))
		for i, eid := range affectedExternalIDs {
			keys[i] = "middleware_user:" + eid + ":" + org.ID.String()
		}
		m.ctx.Redis.Client().Del(c.Request.Context(), keys...)
	}

	sdk.NoContent(c)
}
