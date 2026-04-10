package iam

import (
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
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
	router.DELETE("/me", sdk.Self, m.eraseMe)
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
	Status    *string                `json:"status"     binding:"omitempty,oneof=active suspended"`
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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			sdk.Error(c, sdk.Conflict("user already exists"))
		} else {
			sdk.Error(c, sdk.Internal("failed to create user"))
		}
		return
	}

	// Auto-create org membership for the requesting org.
	member := OrgMember{
		OrgID:    oid,
		UserID:   user.ID,
		JoinedAt: time.Now(),
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

	user, err := sdk.Cache(c.Request.Context(), m.ctx.Redis, cacheKey, m.cacheTTL, func() (User, error) {
		var u User
		err := m.ctx.DB.
			Joins("JOIN org_members ON org_members.user_id = users.id AND org_members.org_id = ? AND org_members.deleted_at IS NULL", oid).
			Where("users.id = ?", uri.ID).
			First(&u).Error
		return u, err
	})
	if err != nil {
		sdk.Error(c, sdk.NotFound("user", uri.ID))
		return
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
		// Re-read to get the updated values (GORM Updates doesn't refresh the struct).
		m.ctx.DB.First(&user, user.ID)
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditUpdate, Resource: "user", ResourceID: user.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.user.updated", user)

	// Invalidate caches: module-owned keys use namespaced Redis,
	// middleware keys use the raw client (no prefix).
	if m.ctx.Redis.Client() != nil {
		ctx := c.Request.Context()
		m.ctx.Redis.Del(ctx, fmt.Sprintf("user_org_membership:%s:%s", uri.ID, oid))
		m.ctx.Redis.Client().Del(ctx, "middleware_user:"+user.ExternalID+":"+oid.String())
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

	if err := m.removeMembership(c, uri.ID, oid); err != nil {
		return // error already sent to client
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
	if err := m.ctx.DB.
		Joins("JOIN public.org_members org_members ON org_members.user_id = users.id AND org_members.org_id = ? AND org_members.deleted_at IS NULL", oid).
		Where("users.external_id = ? AND users.provider = ?", sub, provider).
		First(&user).Error; err != nil {
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
	if err := m.ctx.DB.
		Joins("JOIN public.org_members org_members ON org_members.user_id = users.id AND org_members.org_id = ? AND org_members.deleted_at IS NULL", oid).
		Where("users.external_id = ? AND users.provider = ?", sub, provider).
		First(&user).Error; err != nil {
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
		// Re-read to get the updated values (GORM Updates doesn't refresh the struct).
		m.ctx.DB.First(&user, user.ID)
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditUpdate, Resource: "user", ResourceID: user.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.user.updated", user)

	// Invalidate caches: module-owned keys use namespaced Redis,
	// middleware keys use the raw client (no prefix).
	if m.ctx.Redis.Client() != nil {
		ctx := c.Request.Context()
		m.ctx.Redis.Del(ctx, fmt.Sprintf("user_org_membership:%s:%s", user.ID, oid))
		m.ctx.Redis.Client().Del(ctx, "middleware_user:"+user.ExternalID+":"+oid.String())
	}

	sdk.OK(c, user)
}

// eraseMe handles the GDPR right-to-erasure for the authenticated user.
// Anonymises PII, soft-deletes the user record, and removes all org memberships.
func (m *Module) eraseMe(c *gin.Context) {
	sub := userSubject(c)
	provider := c.GetString("auth_provider")

	var user User
	if err := m.ctx.DB.Where("external_id = ? AND provider = ?", sub, provider).First(&user).Error; err != nil {
		sdk.Error(c, sdk.NotFound("user", sub))
		return
	}

	// Prevent erasure if the user is the sole owner of any organization.
	// Find orgs where this user has the owner role.
	var ownedOrgIDs []uuid.UUID
	m.ctx.DB.Model(&UserRole{}).
		Select("user_roles.org_id").
		Joins("JOIN module_iam.roles ON roles.id = user_roles.role_id").
		Where("user_roles.user_id = ? AND roles.slug = 'owner'", user.ID).
		Pluck("org_id", &ownedOrgIDs)

	for _, oid := range ownedOrgIDs {
		var otherOwners int64
		m.ctx.DB.Model(&UserRole{}).
			Joins("JOIN module_iam.roles ON roles.id = user_roles.role_id").
			Where("user_roles.org_id = ? AND roles.slug = 'owner' AND user_roles.user_id != ?", oid, user.ID).
			Count(&otherOwners)
		if otherOwners == 0 {
			sdk.Error(c, sdk.BadRequest("cannot erase account: you are the sole owner of an organization — transfer ownership first"))
			return
		}
	}

	// Collect org IDs before deleting memberships (needed for cache invalidation).
	var orgIDs []uuid.UUID
	if m.ctx.Redis.Client() != nil {
		m.ctx.DB.Model(&OrgMember{}).Where("user_id = ?", user.ID).Pluck("org_id", &orgIDs)
	}

	// Anonymise PII.
	user.ErasePersonalData()

	tx := m.ctx.DB.Begin()

	// Save anonymised data.
	if err := tx.Save(&user).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to erase personal data"))
		return
	}

	// Soft-delete the user.
	if err := tx.Delete(&user).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to delete user"))
		return
	}

	// Remove all org memberships and role assignments.
	if err := tx.Where("user_id = ?", user.ID).Delete(&OrgMember{}).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to remove memberships"))
		return
	}
	if err := tx.Where("user_id = ?", user.ID).Delete(&UserRole{}).Error; err != nil {
		tx.Rollback()
		sdk.Error(c, sdk.Internal("failed to remove role assignments"))
		return
	}

	if err := tx.Commit().Error; err != nil {
		sdk.Error(c, sdk.Internal("failed to complete erasure"))
		return
	}

	m.ctx.Audit.Log(c.Request.Context(), sdk.AuditEntry{
		Action: sdk.AuditDelete, Resource: "user", ResourceID: user.ID.String(),
	})
	m.ctx.Bus.Publish(c.Request.Context(), "iam.user.erased", gin.H{"id": user.ID})

	// Invalidate cache keys for every org the user belonged to.
	if len(orgIDs) > 0 {
		ctx := c.Request.Context()
		keys := make([]string, 0, len(orgIDs)*2)
		for _, oid := range orgIDs {
			keys = append(keys,
				fmt.Sprintf("user_org_membership:%s:%s", user.ID, oid),
			)
		}
		m.ctx.Redis.Del(ctx, keys...)

		// Middleware keys are not namespaced — use raw client.
		mwKeys := make([]string, len(orgIDs))
		for i, oid := range orgIDs {
			mwKeys[i] = "middleware_user:" + user.ExternalID + ":" + oid.String()
		}
		m.ctx.Redis.Client().Del(ctx, mwKeys...)
	}

	sdk.NoContent(c)
}
