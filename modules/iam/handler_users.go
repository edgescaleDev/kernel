package iam

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.edgescale.dev/kernel/sdk"
)

// registerUserRoutes mounts user-related endpoints under /v1/iam/.
func registerUserRoutes(m *Module, router *sdk.Router) {
	router.GET("/users", "iam.users.read", m.listUsers)
	router.POST("/users", "iam.users.manage", m.createUser)
	router.GET("/users/:id", "iam.users.read", m.getUser)
	router.PATCH("/users/:id", "iam.users.manage", m.updateUser)
	router.DELETE("/users/:id", "iam.users.manage", m.deleteUser)

	// Self-service profile — queries by IdP subject, not UUID.
	router.GET("/me", sdk.Self, m.getMe)
	router.PATCH("/me", sdk.Self, m.updateMe)
}

// ---- request / response DTOs -----------------------------------------------

type createUserRequest struct {
	Email      string                `json:"email"`
	Phone      string                `json:"phone"`
	Name       sdk.TranslatableField `json:"name"        binding:"required"`
	ExternalID string                `json:"external_id" binding:"required"`
	Provider   string                `json:"provider"`
}

type updateUserRequest struct {
	Name      *sdk.TranslatableField `json:"name"`
	Email     *string                `json:"email"`
	Phone     *string                `json:"phone"`
	AvatarURL *string                `json:"avatar_url"`
	Locale    *string                `json:"locale"`
	Timezone  *string                `json:"timezone"`
	Status    *string                `json:"status"`
}

type updateMeRequest struct {
	Name      *sdk.TranslatableField `json:"name"`
	AvatarURL *string                `json:"avatar_url"`
	Locale    *string                `json:"locale"`
	Timezone  *string                `json:"timezone"`
}

type resourceURI struct {
	ID uuid.UUID `uri:"id" binding:"required"`
}

// ---- handlers: admin CRUD --------------------------------------------------

// listUsers returns users that are members of the requesting org.
// Queries through org_members JOIN since users are platform-level identities.
func (m *Module) listUsers(c *gin.Context) {
	oid := orgID(c)
	page := sdk.ParsePageRequest(c)

	result, err := sdk.Paginate[User](
		m.ctx.DB.Joins("JOIN org_members ON org_members.user_id = users.id AND org_members.org_id = ? AND org_members.deleted_at IS NULL", oid),
		page,
	)
	if err != nil {
		sdk.Error(c, sdk.BadRequest(err.Error()))
		return
	}
	sdk.List(c, result.Items, result.Meta)
}

// createUser creates a platform-level user and adds them as a member of the requesting org.
func (m *Module) createUser(c *gin.Context) {
	var req createUserRequest
	if !sdk.BindAndValidate(c, &req) {
		return
	}

	oid := orgID(c)
	user := User{
		ExternalID: req.ExternalID,
		Provider:   req.Provider,
		Email:      req.Email,
		Phone:      req.Phone,
		Name:       req.Name,
	}
	if user.Provider == "" {
		user.Provider = "platform"
	}

	tx := m.ctx.DB.Begin()

	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Conflict("user already exists or constraint violation"))
		return
	}

	// Auto-create org membership for the requesting org.
	member := OrgMember{
		OrgID:  oid,
		UserID: user.ID,
	}
	if err := tx.Create(&member).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Conflict("user is already a member of this organization"))
		return
	}

	if err := tx.Commit().Error; err != nil {
		sdk.Error(c, sdk.Internal("failed to create user"))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditCreate, Resource: "user", ResourceID: user.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.user.created", user)

	sdk.Created(c, user)
}

// getUser returns a user by ID, verifying membership in the requesting org.
func (m *Module) getUser(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
		return
	}

	oid := orgID(c)
	cacheKey := fmt.Sprintf("user_org_membership:%s:%s", uri.ID, oid)

	// Attempt to retrieve from cache
	if m.ctx.Redis.Client() != nil {
		if cached, err := m.ctx.Redis.Get(c.Request.Context(), cacheKey).Bytes(); err == nil {
			var user User
			if json.Unmarshal(cached, &user) == nil {
				sdk.OK(c, user)
				return
			}
		}
	}

	var user User
	if err := m.ctx.DB.
		Joins("JOIN org_members ON org_members.user_id = users.id AND org_members.org_id = ? AND org_members.deleted_at IS NULL", oid).
		Where("users.id = ?", uri.ID).
		First(&user).Error; err != nil {
		sdk.Error(c, sdk.NotFound("user", uri.ID))
		return
	}

	// Store in cache
	if m.ctx.Redis.Client() != nil && m.cacheTTL > 0 {
		if data, err := json.Marshal(&user); err == nil {
			m.ctx.Redis.Set(c.Request.Context(), cacheKey, data, m.cacheTTL)
		}
	}

	sdk.OK(c, user)
}

// updateUser updates a user by ID, verifying membership in the requesting org.
func (m *Module) updateUser(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
		return
	}
	var req updateUserRequest
	if !sdk.BindAndValidate(c, &req) {
		return
	}

	oid := orgID(c)
	var user User
	if err := m.ctx.DB.
		Joins("JOIN org_members ON org_members.user_id = users.id AND org_members.org_id = ? AND org_members.deleted_at IS NULL", oid).
		Where("users.id = ?", uri.ID).
		First(&user).Error; err != nil {
		sdk.Error(c, sdk.NotFound("user", uri.ID))
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Email != nil {
		updates["email"] = *req.Email
	}
	if req.Phone != nil {
		updates["phone"] = *req.Phone
	}
	if req.AvatarURL != nil {
		updates["avatar_url"] = *req.AvatarURL
	}
	if req.Locale != nil {
		updates["locale"] = *req.Locale
	}
	if req.Timezone != nil {
		updates["timezone"] = *req.Timezone
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	if len(updates) > 0 {
		if err := m.ctx.DB.Model(&user).Updates(updates).Error; err != nil {
			sdk.Error(c, sdk.BadRequest(err.Error()))
			return
		}
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditUpdate, Resource: "user", ResourceID: user.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.user.updated", user)

	// Invalidate caches
	if m.ctx.Redis.Client() != nil {
		m.ctx.Redis.Del(c.Request.Context(), fmt.Sprintf("user_org_membership:%s:%s", uri.ID, oid))
		m.ctx.Redis.Del(c.Request.Context(), "middleware_user:"+user.ExternalID+":"+oid.String())
	}

	sdk.OK(c, user)
}

// deleteUser removes a user from the requesting org by deleting their membership
// and associated role assignments. The platform-level User record is preserved
// because the user may belong to other organizations.
func (m *Module) deleteUser(c *gin.Context) {
	var uri resourceURI
	if !sdk.BindURI(c, &uri) {
		return
	}

	oid := orgID(c)

	// Verify membership before deleting.
	var member OrgMember
	if err := m.ctx.DB.Where("user_id = ? AND org_id = ?", uri.ID, oid).First(&member).Error; err != nil {
		sdk.Error(c, sdk.NotFound("user", uri.ID))
		return
	}

	// Use a transaction to atomically remove membership and associated roles.
	tx := m.ctx.DB.Begin()

	if err := tx.Delete(&member).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to remove member"))
		return
	}

	// Clean up any roles assigned to this user within this organization.
	if err := tx.Where("user_id = ? AND org_id = ?", uri.ID, oid).Delete(&UserRole{}).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to clean up member roles"))
		return
	}

	if err := tx.Commit().Error; err != nil {
		sdk.Error(c, sdk.Internal("transaction failed"))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditDelete, Resource: "org_member", ResourceID: member.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.member.removed", gin.H{"id": member.ID, "org_id": oid, "user_id": uri.ID})

	// Invalidate caches
	if m.ctx.Redis.Client() != nil {
		m.ctx.Redis.Del(c.Request.Context(), fmt.Sprintf("user_org_membership:%s:%s", uri.ID, oid))

		var user User
		if m.ctx.DB.Where("id = ?", uri.ID).First(&user).Error == nil {
			m.ctx.Redis.Del(c.Request.Context(), "middleware_user:"+user.ExternalID+":"+oid.String())
		}
	}

	sdk.NoContent(c)
}

// ---- handlers: self-service (/me) ------------------------------------------
// These query by external_id (IdP Subject) + provider, not by UUID.
// The user_id in context is the IdP subject set by authenticate() middleware.

// getMe returns the authenticated user's profile.
// Looks up by IdP subject (platform-level), then verifies org membership.
func (m *Module) getMe(c *gin.Context) {
	sub := userSubject(c)
	oid := orgID(c)
	provider := c.GetString("auth_provider")

	var user User
	if err := m.ctx.DB.Where(
		"external_id = ? AND provider = ?", sub, provider,
	).First(&user).Error; err != nil {
		sdk.Error(c, sdk.NotFound("user", sub))
		return
	}

	// Verify org membership.
	var member OrgMember
	if err := m.ctx.DB.Where("user_id = ? AND org_id = ?", user.ID, oid).First(&member).Error; err != nil {
		sdk.Error(c, sdk.Forbidden("user is not a member of this organization"))
		return
	}

	sdk.OK(c, user)
}

// updateMe updates the authenticated user's own profile.
func (m *Module) updateMe(c *gin.Context) {
	sub := userSubject(c)
	oid := orgID(c)
	provider := c.GetString("auth_provider")

	var req updateMeRequest
	if !sdk.BindAndValidate(c, &req) {
		return
	}

	var user User
	if err := m.ctx.DB.Where(
		"external_id = ? AND provider = ?", sub, provider,
	).First(&user).Error; err != nil {
		sdk.Error(c, sdk.NotFound("user", sub))
		return
	}

	// Verify org membership.
	var member OrgMember
	if err := m.ctx.DB.Where("user_id = ? AND org_id = ?", user.ID, oid).First(&member).Error; err != nil {
		sdk.Error(c, sdk.Forbidden("user is not a member of this organization"))
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.AvatarURL != nil {
		updates["avatar_url"] = *req.AvatarURL
	}
	if req.Locale != nil {
		updates["locale"] = *req.Locale
	}
	if req.Timezone != nil {
		updates["timezone"] = *req.Timezone
	}

	if len(updates) > 0 {
		if err := m.ctx.DB.Model(&user).Updates(updates).Error; err != nil {
			sdk.Error(c, sdk.BadRequest(err.Error()))
			return
		}
	}

	// Invalidate caches
	if m.ctx.Redis.Client() != nil {
		m.ctx.Redis.Del(c.Request.Context(), fmt.Sprintf("user_org_membership:%s:%s", user.ID, oid))
		m.ctx.Redis.Del(c.Request.Context(), "middleware_user:"+user.ExternalID+":"+oid.String())
	}

	sdk.OK(c, user)
}
