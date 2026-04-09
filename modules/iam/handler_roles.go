package iam

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.edgescale.dev/kernel/sdk"
	"gorm.io/gorm/clause"
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
		// Re-read to get the updated values (GORM Updates doesn't refresh the struct).
		m.ctx.DB.Preload("Permissions").First(&role, role.ID)
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

	// Collect affected users before deleting assignments (needed for cache invalidation).
	var affectedExternalIDs []string
	if m.ctx.Redis.Client() != nil {
		m.ctx.DB.Model(&User{}).
			Joins("JOIN module_iam.user_roles ur ON ur.user_id = users.id").
			Where("ur.role_id = ?", role.ID).
			Pluck("external_id", &affectedExternalIDs)
	}

	tx := m.ctx.DB.Begin()

	// Remove user_roles referencing this role so users don't retain stale permissions.
	if err := tx.Where("role_id = ?", role.ID).Delete(&UserRole{}).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to clean up user role assignments"))
		return
	}

	// Remove role_permissions.
	if err := tx.Where("role_id = ?", role.ID).Delete(&RolePermission{}).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to clean up role permissions"))
		return
	}

	// Soft-delete the role itself.
	if err := tx.Delete(&role).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to delete role"))
		return
	}

	if err := tx.Commit().Error; err != nil {
		sdk.Error(c, sdk.Internal("transaction failed"))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditDelete, Resource: "role", ResourceID: uri.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.role.deleted", gin.H{"id": uri.ID, "org_id": oid})

	// Invalidate middleware cache for affected users.
	if len(affectedExternalIDs) > 0 {
		keys := make([]string, len(affectedExternalIDs))
		for i, eid := range affectedExternalIDs {
			keys[i] = "middleware_user:" + eid + ":" + oid.String()
		}
		m.ctx.Redis.Client().Del(c.Request.Context(), keys...)
	}

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

	// Validate all permission keys against registered module manifests.
	if m.ctx.ValidPermissionKey != nil {
		for _, key := range req.Permissions {
			if key != "*" && !m.ctx.ValidPermissionKey(key) {
				sdk.Error(c, sdk.BadRequest("unknown permission: "+key))
				return
			}
		}
	}

	// Replace: insert new first (superset), then delete stale.
	// This avoids a zero-permission window between delete and insert.
	tx := m.ctx.DB.Begin()

	if len(req.Permissions) > 0 {
		perms := make([]RolePermission, len(req.Permissions))
		for i, key := range req.Permissions {
			perms[i] = RolePermission{RoleID: role.ID, PermissionKey: key}
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&perms).Error; err != nil {
			tx.Rollback()
			sdk.Error(c, sdk.BadRequest("failed to assign permissions"))
			return
		}
	}

	// Delete permissions not in the new set.
	q := tx.Where("role_id = ?", role.ID)
	if len(req.Permissions) > 0 {
		q = q.Where("permission_key NOT IN ?", req.Permissions)
	}
	if err := q.Delete(&RolePermission{}).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to clean up stale permissions"))
		return
	}

	if err := tx.Commit().Error; err != nil {
		sdk.Error(c, sdk.Internal("transaction failed"))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditUpdate, Resource: "role_permissions", ResourceID: role.ID.String(),
	})

	// Invalidate middleware cache for all users holding this role.
	// Middleware keys are NOT namespaced, so use the raw client.
	if m.ctx.Redis.Client() != nil {
		var externalIDs []string
		m.ctx.DB.Model(&User{}).
			Joins("JOIN module_iam.user_roles ur ON ur.user_id = users.id").
			Where("ur.role_id = ?", role.ID).
			Pluck("external_id", &externalIDs)

		if len(externalIDs) > 0 {
			keys := make([]string, len(externalIDs))
			for i, eid := range externalIDs {
				keys[i] = "middleware_user:" + eid + ":" + oid.String()
			}
			m.ctx.Redis.Client().Del(c.Request.Context(), keys...)
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
	var roles []Role
	m.ctx.DB.Preload("Permissions").
		Joins("JOIN module_iam.user_roles ur ON ur.role_id = roles.id").
		Where("ur.user_id = ? AND ur.org_id = ?", uri.ID, oid).
		Find(&roles)

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

	// Validate all role IDs belong to this organization to prevent cross-org injection.
	if len(req.RoleIDs) > 0 {
		var validCount int64
		m.ctx.DB.Model(&Role{}).Where("id IN ? AND org_id = ? AND deleted_at IS NULL", req.RoleIDs, oid).Count(&validCount)
		if int(validCount) != len(req.RoleIDs) {
			sdk.Error(c, sdk.BadRequest("one or more roles do not belong to this organization"))
			return
		}
	}

	// Replace: delete existing, insert new.
	tx := m.ctx.DB.Begin()

	if err := tx.Where("user_id = ? AND org_id = ?", uri.ID, oid).Delete(&UserRole{}).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to clear existing role assignments"))
		return
	}

	if len(req.RoleIDs) > 0 {
		roles := make([]UserRole, len(req.RoleIDs))
		for i, roleID := range req.RoleIDs {
			roles[i] = UserRole{OrgID: oid, UserID: uri.ID, RoleID: roleID}
		}
		if err := tx.Create(&roles).Error; err != nil {
			tx.Rollback()
			sdk.Error(c, sdk.BadRequest("failed to assign roles"))
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		sdk.Error(c, sdk.Internal("transaction failed"))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditUpdate, Resource: "user_roles", ResourceID: uri.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.user_roles.updated", gin.H{"user_id": uri.ID, "org_id": oid})

	// Invalidate cache: middleware keys use raw client.
	if m.ctx.Redis.Client() != nil {
		var user User
		if m.ctx.DB.Unscoped().Where("id = ?", uri.ID).First(&user).Error == nil {
			m.ctx.Redis.Client().Del(c.Request.Context(), "middleware_user:"+user.ExternalID+":"+oid.String())
		}
	}

	sdk.NoContent(c)
}
