package iam

import (
	"github.com/gin-gonic/gin"
	"go.edgescale.dev/kernel/sdk"
)

// registerOrgRoutes mounts organization-related endpoints under /v1/iam/.
func registerOrgRoutes(m *Module, router *sdk.Router) {
	router.GET("/orgs", "iam.orgs.read", m.listOrgs)
	router.POST("/orgs", "iam.orgs.manage", m.createOrg)
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
	Status  *string                `json:"status"`
}

// ---- handlers --------------------------------------------------------------

func (m *Module) listOrgs(c *gin.Context) {
	page := sdk.ParsePageRequest(c)

	result, err := sdk.Paginate[Organization](m.ctx.DB.Where("status != 'platform'"), page)
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

	org := Organization{
		Name: req.Name,
		Slug: req.Slug,
	}

	if err := m.ctx.DB.Create(&org).Error; err != nil {
		sdk.Error(c, sdk.Conflict("organization with this slug already exists"))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditCreate, Resource: "organization", ResourceID: org.ID.String(),
	})

	sdk.Created(c, org)
}

func (m *Module) getOrg(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
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

	var org Organization
	if err := m.ctx.DB.Where("id = ?", uri.ID).First(&org).Error; err != nil {
		sdk.Error(c, sdk.NotFound("organization", uri.ID))
		return
	}
	if org.Status == "platform" {
		sdk.Error(c, sdk.Forbidden("platform organization cannot be deleted"))
		return
	}

	if err := m.ctx.DB.Delete(&org).Error; err != nil {
		sdk.Error(c, sdk.Internal("failed to delete organization"))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditDelete, Resource: "organization", ResourceID: org.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.org.deleted", org)

	sdk.NoContent(c)
}
