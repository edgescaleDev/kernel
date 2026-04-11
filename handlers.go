package kernel

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.edgescale.dev/kernel/sdk"
)

// handleHealthz is a liveness probe. Returns 200 if the kernel process is running.
// Kubernetes uses this to decide whether to restart the pod.
func (k *Kernel) handleHealthz(c *gin.Context) {
	sdk.OK(c, gin.H{"status": "ok"})
}

// handleReadyz is a readiness probe. Returns 200 only if the kernel
// can serve traffic (database and Redis are reachable).
func (k *Kernel) handleReadyz(c *gin.Context) {
	// Check database.
	if k.db != nil {
		sqlDB, err := k.db.DB()
		if err != nil || sqlDB.Ping() != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "reason": "database unreachable"})
			return
		}
	}

	// Check Redis.
	if k.redis != nil {
		if err := k.redis.Ping(c.Request.Context()).Err(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "reason": "redis unreachable"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

// moduleInfo is the JSON shape returned by the /v1/modules endpoints.
type moduleInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

func toModuleInfo(m sdk.Manifest) moduleInfo {
	return moduleInfo{
		ID:          m.ID,
		Name:        m.Name,
		Version:     m.Version,
		Type:        m.Type.String(),
		Description: m.Description,
		DependsOn:   m.DependsOn,
		Tags:        m.Tags,
	}
}

// permissionInfo is the JSON shape returned by the /v1/permissions endpoint.
type permissionInfo struct {
	Module string `json:"module"`
	Key    string `json:"key"`
	Label  string `json:"label"`
}

// handleListPermissions returns all permissions declared by all registered modules.
// GET /v1/permissions
func (k *Kernel) handleListPermissions(c *gin.Context) {
	var perms []permissionInfo
	for _, m := range k.Modules() {
		manifest := m.Manifest()
		for _, p := range manifest.Permissions {
			perms = append(perms, permissionInfo{
				Module: manifest.ID,
				Key:    p.Key,
				Label:  p.Label,
			})
		}
	}

	sdk.OK(c, perms)
}

// handleListModules returns metadata for all registered modules.
// GET /v1/modules
func (k *Kernel) handleListModules(c *gin.Context) {
	modules := make([]moduleInfo, 0, len(k.modules))
	for _, m := range k.Modules() {
		modules = append(modules, toModuleInfo(m.Manifest()))
	}

	c.JSON(http.StatusOK, sdk.Envelope{
		Success: true,
		Result:  modules,
	})
}

// handleActiveModules returns modules that are active for the requesting org.
// GET /v1/modules/active
// Core modules are always included. Feature modules are filtered by the
// module_activations table via IsModuleActive (Redis cached).
func (k *Kernel) handleActiveModules(c *gin.Context) {
	orgIDVal, hasOrg := c.Get("org_id")

	modules := make([]moduleInfo, 0, len(k.modules))
	for _, m := range k.Modules() {
		manifest := m.Manifest()

		// If we have an org context, filter by activation status.
		if hasOrg {
			orgID, _ := orgIDVal.(string)
			if !k.IsModuleActive(manifest.ID, orgID) {
				continue
			}
		}

		modules = append(modules, toModuleInfo(manifest))
	}

	c.JSON(http.StatusOK, sdk.Envelope{
		Success: true,
		Result:  modules,
	})
}

// handleMe returns the authenticated user's profile, permissions,
// and active modules for the current org.
// GET /v1/me
func (k *Kernel) handleMe(c *gin.Context) {
	response := gin.H{
		"authenticated": true,
	}

	// Include permissions if loaded.
	if permsVal, exists := c.Get("permissions"); exists {
		if ps, ok := permsVal.(*PermissionSet); ok {
			response["permissions"] = ps.Permissions()
		}
	}

	// Include org_id if resolved.
	if orgID, exists := c.Get("org_id"); exists {
		response["org_id"] = orgID
	}

	// Attempt to load full user profile via any module that provides
	// an sdk.UserProfileReader (e.g., IAM). The kernel never imports the
	// module directly — Go's implicit interface satisfaction handles it.
	if reader, err := sdk.GetReader[sdk.UserProfileReader](k.readers, "iam"); err == nil {
		subject := c.GetString("user_id")
		provider := c.GetString("auth_provider")

		if user, err := reader.GetUserByExternalID(c.Request.Context(), provider, subject); err == nil {
			response["user"] = user
		} else {
			// Fallback: just include user_id if fetch fails.
			response["user_id"] = subject
		}
	} else {
		// Fallback: if no user profile reader is registered.
		if userID, exists := c.Get("user_id"); exists {
			response["user_id"] = userID
		}
	}

	c.JSON(http.StatusOK, sdk.Envelope{
		Success: true,
		Result:  response,
	})
}
