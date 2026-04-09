package iam

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.edgescale.dev/kernel/sdk"
)

// registerRoleRoutes mounts RBAC endpoints under /v1/iam/.
func registerRoleRoutes(m *Module, router *sdk.Router) {
	// Role CRUD
	router.GET("/roles", "iam.roles.read", m.listRoles)
	router.POST("/roles", "iam.roles.manage", m.createRole)
	router.GET("/roles/:id", "iam.roles.read", m.getRole)
	router.PATCH("/roles/:id", "iam.roles.manage", m.updateRole)
	router.DELETE("/roles/:id", "iam.roles.manage", m.deleteRole)

	// Role permission assignment
	router.PUT("/roles/:id/permissions", "iam.roles.manage", m.setRolePermissions)

	// User role assignment
	router.GET("/users/:id/roles", "iam.roles.read", m.listUserRoles)
	router.PUT("/users/:id/roles", "iam.roles.manage", m.setUserRoles)
}

// ---- request DTOs ----------------------------------------------------------

type createRoleRequest struct {
	Name        sdk.TranslatableField `json:"name"        binding:"required"`
	Slug        string                `json:"slug"        binding:"required"`
	Description sdk.TranslatableField `json:"description"`
}

type updateRoleRequest struct {
	Name        *sdk.TranslatableField `json:"name"`
	Description *sdk.TranslatableField `json:"description"`
}

type setRolePermissionsRequest struct {
	Permissions []string `json:"permissions" binding:"required"`
}

type setUserRolesRequest struct {
	RoleIDs []uuid.UUID `json:"role_ids" binding:"required"`
}

// ---- handlers: role CRUD ---------------------------------------------------

func (m *Module) listRoles(c *gin.Context) {
	oid := orgID(c)
	page := sdk.ParsePageRequest(c)

	result, err := sdk.Paginate[Role](
		m.ctx.DB.Preload("Permissions").Where("org_id = ?", oid),
		page,
	)
	if err != nil {
		sdk.Error(c, sdk.BadRequest(err.Error()))
		return
	}
	sdk.List(c, result.Items, result.Meta)
}

func (m *Module) createRole(c *gin.Context) {
	var req createRoleRequest
	if !sdk.BindAndValidate(c, &req) {
		return
	}

	oid := orgID(c)
	role := Role{
		OrgID:       oid,
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
	}

	if err := m.ctx.DB.Create(&role).Error; err != nil {
		sdk.Error(c, sdk.Conflict("role with this slug already exists in this org"))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditCreate, Resource: "role", ResourceID: role.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.role.created", role)

	sdk.Created(c, role)
}

func (m *Module) getRole(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
		return
	}

	oid := orgID(c)
	var role Role
	if err := m.ctx.DB.Preload("Permissions").
		Where("id = ? AND org_id = ?", uri.ID, oid).
		First(&role).Error; err != nil {
		sdk.Error(c, sdk.NotFound("role", uri.ID))
		return
	}
	sdk.OK(c, role)
}

func (m *Module) updateRole(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
		return
	}
	var req updateRoleRequest
	if !sdk.BindAndValidate(c, &req) {
		return
	}

	oid := orgID(c)
	var role Role
	if err := m.ctx.DB.Where("id = ? AND org_id = ?", uri.ID, oid).First(&role).Error; err != nil {
		sdk.Error(c, sdk.NotFound("role", uri.ID))
		return
	}
	if role.IsSystem {
		sdk.Error(c, sdk.BadRequest("system roles cannot be modified"))
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}

	if len(updates) > 0 {
		if err := m.ctx.DB.Model(&role).Updates(updates).Error; err != nil {
			sdk.Error(c, sdk.BadRequest(err.Error()))
			return
		}
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditUpdate, Resource: "role", ResourceID: role.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.role.updated", role)

	sdk.OK(c, role)
}

func (m *Module) deleteRole(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
		return
	}

	oid := orgID(c)
	var role Role
	if err := m.ctx.DB.Where("id = ? AND org_id = ?", uri.ID, oid).First(&role).Error; err != nil {
		sdk.Error(c, sdk.NotFound("role", uri.ID))
		return
	}
	if role.IsSystem {
		sdk.Error(c, sdk.BadRequest("system roles cannot be deleted"))
		return
	}

	if err := m.ctx.DB.Delete(&role).Error; err != nil {
		sdk.Error(c, sdk.BadRequest(err.Error()))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditDelete, Resource: "role", ResourceID: uri.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.role.deleted", gin.H{"id": uri.ID, "org_id": oid})

	sdk.NoContent(c)
}

// ---- handlers: role permissions --------------------------------------------

// setRolePermissions replaces all permissions for a role (idempotent PUT).
func (m *Module) setRolePermissions(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
		return
	}
	var req setRolePermissionsRequest
	if !sdk.BindAndValidate(c, &req) {
		return
	}

	oid := orgID(c)
	var role Role
	if err := m.ctx.DB.Where("id = ? AND org_id = ?", uri.ID, oid).First(&role).Error; err != nil {
		sdk.Error(c, sdk.NotFound("role", uri.ID))
		return
	}

	// Replace: delete existing, insert new.
	tx := m.ctx.DB.Begin()
	tx.Where("role_id = ?", role.ID).Delete(&RolePermission{})

	for _, key := range req.Permissions {
		tx.Create(&RolePermission{
			RoleID:        role.ID,
			PermissionKey: key,
		})
	}

	if err := tx.Commit().Error; err != nil {
		sdk.Error(c, sdk.BadRequest(err.Error()))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditUpdate, Resource: "role_permissions", ResourceID: role.ID.String(),
	})

	// Invalidate middleware cache for all users holding this role
	if m.ctx.Redis.Client() != nil {
		var users []User
		m.ctx.DB.Joins("JOIN module_iam.user_roles ur ON ur.user_id = users.id").
			Where("ur.role_id = ?", role.ID).
			Find(&users)

		for _, u := range users {
			m.ctx.Redis.Del(c.Request.Context(), "middleware_user:"+u.ExternalID+":"+oid.String())
		}
	}

	// Return the updated role with permissions.
	m.ctx.DB.Preload("Permissions").First(&role, role.ID)
	m.ctx.Bus.Publish(c.Request.Context(), "iam.role.permissions.updated", role)

	sdk.OK(c, role)
}

// ---- handlers: user roles --------------------------------------------------

func (m *Module) listUserRoles(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
		return
	}

	oid := orgID(c)
	var userRoles []UserRole
	if err := m.ctx.DB.Where("user_id = ? AND org_id = ?", uri.ID, oid).
		Find(&userRoles).Error; err != nil {
		sdk.Error(c, sdk.BadRequest(err.Error()))
		return
	}

	// Collect role IDs and fetch full roles with permissions.
	roleIDs := make([]uuid.UUID, len(userRoles))
	for i, ur := range userRoles {
		roleIDs[i] = ur.RoleID
	}

	var roles []Role
	if len(roleIDs) > 0 {
		m.ctx.DB.Preload("Permissions").Where("id IN ?", roleIDs).Find(&roles)
	}

	sdk.OK(c, roles)
}

// setUserRoles replaces all role assignments for a user in this org (idempotent PUT).
func (m *Module) setUserRoles(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
		return
	}
	var req setUserRolesRequest
	if !sdk.BindAndValidate(c, &req) {
		return
	}

	oid := orgID(c)

	// Verify the user is a member of this org.
	var member OrgMember
	if err := m.ctx.DB.Where("user_id = ? AND org_id = ?", uri.ID, oid).First(&member).Error; err != nil {
		sdk.Error(c, sdk.NotFound("user", uri.ID))
		return
	}

	// Replace: delete existing, insert new.
	tx := m.ctx.DB.Begin()
	tx.Where("user_id = ? AND org_id = ?", uri.ID, oid).Delete(&UserRole{})

	for _, roleID := range req.RoleIDs {
		tx.Create(&UserRole{
			OrgID:  oid,
			UserID: uri.ID,
			RoleID: roleID,
		})
	}

	if err := tx.Commit().Error; err != nil {
		sdk.Error(c, sdk.BadRequest(err.Error()))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditUpdate, Resource: "user_roles", ResourceID: uri.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.user_roles.updated", gin.H{"user_id": uri.ID, "org_id": oid})

	// Invalidate cache
	if m.ctx.Redis.Client() != nil {
		var user User
		if m.ctx.DB.Unscoped().Where("id = ?", uri.ID).First(&user).Error == nil {
			m.ctx.Redis.Del(c.Request.Context(), "middleware_user:"+user.ExternalID+":"+oid.String())
		}
	}

	sdk.NoContent(c)
}
