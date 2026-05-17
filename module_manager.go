package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/edgescaleDev/kernel/internal"
	"github.com/google/uuid"
	"github.com/kernel-contrib/sdk"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// moduleManager implements sdk.ModuleManager using the kernel's in-memory
// manifests and the module_activations table (with Redis caching).
type moduleManager struct {
	manifests map[string]sdk.Manifest
	modules   []sdk.Module
	depOrder  []string
	db        *gorm.DB
	redis     *redis.Client
	cacheTTL  time.Duration
}

// newModuleManager creates a ModuleManager backed by the kernel's state.
func newModuleManager(k *Kernel) *moduleManager {
	ttl := time.Minute
	if k.cfg.Server.CacheTTL > 0 {
		ttl = k.cfg.Server.CacheTTL
	}
	return &moduleManager{
		manifests: k.manifests,
		modules:   k.modules,
		depOrder:  k.depOrder,
		db:        k.db,
		redis:     k.redis,
		cacheTTL:  ttl,
	}
}

// ── Discovery ────────────────────────────────────────────────────────────────

func (mm *moduleManager) List(_ context.Context) []sdk.ModuleInfo {
	ordered := mm.orderedManifests()
	result := make([]sdk.ModuleInfo, 0, len(ordered))
	for _, m := range ordered {
		result = append(result, toModuleInfoSDK(m))
	}
	return result
}

func (mm *moduleManager) Get(_ context.Context, moduleID string) (*sdk.ModuleInfo, error) {
	m, ok := mm.manifests[moduleID]
	if !ok {
		return nil, sdk.NotFound("module", moduleID)
	}
	info := toModuleInfoSDK(m)
	return &info, nil
}

// ── Activation queries ───────────────────────────────────────────────────────

func (mm *moduleManager) IsActive(_ context.Context, moduleID string, tenantID uuid.UUID) (bool, error) {
	manifest, exists := mm.manifests[moduleID]
	if !exists {
		return false, nil
	}
	if manifest.Type.IsCore() {
		return true, nil
	}
	return mm.checkActive(moduleID, tenantID), nil
}

func (mm *moduleManager) GetStatus(ctx context.Context, moduleID string, tenantID uuid.UUID) (*sdk.ModuleActivation, error) {
	manifest, exists := mm.manifests[moduleID]
	if !exists {
		return nil, sdk.NotFound("module", moduleID)
	}

	// Core modules are always active with no DB record.
	if manifest.Type.IsCore() {
		return &sdk.ModuleActivation{
			ModuleID:   moduleID,
			ModuleName: manifest.Name,
			ModuleType: manifest.Type,
			TenantID:   tenantID,
			Active:     true,
		}, nil
	}

	var activation internal.ModuleActivation
	result := mm.db.WithContext(ctx).
		Where("module_id = ? AND tenant_id = ?", moduleID, tenantID.String()).
		First(&activation)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, sdk.NotFound("activation", moduleID)
		}
		return nil, fmt.Errorf("kernel: get activation status: %w", result.Error)
	}

	return mm.toSDKActivation(activation), nil
}

func (mm *moduleManager) ListActive(ctx context.Context, tenantID uuid.UUID) ([]sdk.ModuleActivation, error) {
	var result []sdk.ModuleActivation
	now := time.Now()

	// Core modules are always active.
	for _, m := range mm.orderedManifests() {
		if m.Type.IsCore() {
			result = append(result, sdk.ModuleActivation{
				ModuleID:   m.ID,
				ModuleName: m.Name,
				ModuleType: m.Type,
				TenantID:   tenantID,
				Active:     true,
			})
		}
	}

	// Query active non-core activations from DB.
	var activations []internal.ModuleActivation
	if err := mm.db.WithContext(ctx).
		Where("tenant_id = ? AND active = true", tenantID.String()).
		Find(&activations).Error; err != nil {
		return nil, fmt.Errorf("kernel: list active modules: %w", err)
	}

	for _, a := range activations {
		manifest, ok := mm.manifests[a.ModuleID]
		if !ok || manifest.Type.IsCore() {
			continue // skip core (already included) and orphaned records
		}
		// Check expiry.
		if a.ExpiresAt != nil && now.After(*a.ExpiresAt) {
			continue
		}
		result = append(result, *mm.toSDKActivation(a))
	}

	return result, nil
}

func (mm *moduleManager) ListInactive(ctx context.Context, tenantID uuid.UUID) ([]sdk.ModuleInfo, error) {
	// Build the set of active module IDs for this tenant.
	activeSet := make(map[string]bool)
	now := time.Now()

	var activations []internal.ModuleActivation
	if err := mm.db.WithContext(ctx).
		Where("tenant_id = ? AND active = true", tenantID.String()).
		Find(&activations).Error; err != nil {
		return nil, fmt.Errorf("kernel: list inactive modules: %w", err)
	}

	for _, a := range activations {
		if a.ExpiresAt == nil || now.Before(*a.ExpiresAt) {
			activeSet[a.ModuleID] = true
		}
	}

	// Return non-core modules that are not in the active set.
	var result []sdk.ModuleInfo
	for _, m := range mm.orderedManifests() {
		if m.Type.IsCore() {
			continue
		}
		if !activeSet[m.ID] {
			result = append(result, toModuleInfoSDK(m))
		}
	}

	return result, nil
}

// ── Activation mutations ─────────────────────────────────────────────────────

func (mm *moduleManager) Activate(ctx context.Context, tenantID uuid.UUID, activatedBy uuid.UUID, moduleIDs ...string) error {
	return mm.activateBatch(ctx, tenantID, activatedBy, nil, moduleIDs)
}

func (mm *moduleManager) ActivateWithExpiry(ctx context.Context, tenantID uuid.UUID, activatedBy uuid.UUID, expiresAt time.Time, moduleIDs ...string) error {
	return mm.activateBatch(ctx, tenantID, activatedBy, &expiresAt, moduleIDs)
}

func (mm *moduleManager) Deactivate(ctx context.Context, tenantID uuid.UUID, moduleIDs ...string) error {
	if err := mm.validateModuleIDs(moduleIDs); err != nil {
		return err
	}

	result := mm.db.WithContext(ctx).
		Model(&internal.ModuleActivation{}).
		Where("module_id IN ? AND tenant_id = ?", moduleIDs, tenantID.String()).
		Update("active", false)
	if result.Error != nil {
		return fmt.Errorf("kernel: deactivate modules: %w", result.Error)
	}

	mm.bustCache(ctx, tenantID, moduleIDs)
	return nil
}

func (mm *moduleManager) ExtendTrial(ctx context.Context, tenantID uuid.UUID, newExpiry time.Time, moduleIDs ...string) error {
	if err := mm.validateModuleIDs(moduleIDs); err != nil {
		return err
	}

	// Verify all targets have an existing trial.
	var activations []internal.ModuleActivation
	if err := mm.db.WithContext(ctx).
		Where("module_id IN ? AND tenant_id = ? AND active = true", moduleIDs, tenantID.String()).
		Find(&activations).Error; err != nil {
		return fmt.Errorf("kernel: extend trial: %w", err)
	}

	found := make(map[string]bool)
	for _, a := range activations {
		if a.ExpiresAt == nil {
			return sdk.BadRequest("module " + a.ModuleID + " is permanently activated, not a trial")
		}
		found[a.ModuleID] = true
	}
	for _, id := range moduleIDs {
		if !found[id] {
			return sdk.BadRequest("module " + id + " has no active trial for this tenant")
		}
	}

	result := mm.db.WithContext(ctx).
		Model(&internal.ModuleActivation{}).
		Where("module_id IN ? AND tenant_id = ? AND active = true AND expires_at IS NOT NULL", moduleIDs, tenantID.String()).
		Updates(map[string]any{
			"expires_at": newExpiry,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("kernel: extend trial: %w", result.Error)
	}

	mm.bustCache(ctx, tenantID, moduleIDs)
	return nil
}

// ── Internal helpers ─────────────────────────────────────────────────────────

// activateBatch upserts activation records for multiple modules in one transaction.
func (mm *moduleManager) activateBatch(ctx context.Context, tenantID uuid.UUID, activatedBy uuid.UUID, expiresAt *time.Time, moduleIDs []string) error {
	if err := mm.validateModuleIDs(moduleIDs); err != nil {
		return err
	}

	now := time.Now()

	err := mm.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, moduleID := range moduleIDs {
			activation := internal.ModuleActivation{
				ModuleID:    moduleID,
				TenantID:    tenantID.String(),
				Active:      true,
				ActivatedBy: activatedBy.String(),
				ExpiresAt:   expiresAt,
				CreatedAt:   now,
				UpdatedAt:   now,
			}

			result := tx.
				Where("module_id = ? AND tenant_id = ?", moduleID, tenantID.String()).
				Assign(internal.ModuleActivation{
					Active:      true,
					ActivatedBy: activatedBy.String(),
					ExpiresAt:   expiresAt,
					UpdatedAt:   now,
				}).
				FirstOrCreate(&activation)
			if result.Error != nil {
				return fmt.Errorf("activate module %q: %w", moduleID, result.Error)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("kernel: activate modules: %w", err)
	}

	mm.bustCache(ctx, tenantID, moduleIDs)
	return nil
}

// checkActive checks whether a module is active for the given tenant.
// Core modules always return true. Feature/integration modules are checked
// against the module_activations table (Redis cached).
func (mm *moduleManager) checkActive(moduleID string, tenantID uuid.UUID) bool {
	manifest, exists := mm.manifests[moduleID]
	if !exists {
		return false
	}

	// Core modules are always active.
	if manifest.Type.IsCore() {
		return true
	}

	now := time.Now()

	// Check Redis cache first.
	if mm.redis != nil {
		cacheKey := fmt.Sprintf("module:%s:active:%s", moduleID, tenantID)
		val, err := mm.redis.Get(context.Background(), cacheKey).Result()
		if err == nil {
			switch val {
			case "0":
				return false
			case "1":
				return true
			default:
				if exp, parseErr := time.Parse(time.RFC3339, val); parseErr == nil {
					return now.Before(exp)
				}
			}
		}
	}

	// Fall back to database.
	var activation internal.ModuleActivation
	result := mm.db.
		Where("module_id = ? AND tenant_id = ? AND active = true", moduleID, tenantID.String()).
		First(&activation)
	if result.Error != nil {
		return false
	}

	active := activation.ExpiresAt == nil || now.Before(*activation.ExpiresAt)

	// Cache the result.
	if mm.redis != nil {
		cacheKey := fmt.Sprintf("module:%s:active:%s", moduleID, tenantID)
		var val string
		if !active {
			val = "0"
		} else if activation.ExpiresAt == nil {
			val = "1"
		} else {
			val = activation.ExpiresAt.Format(time.RFC3339)
		}
		mm.redis.Set(context.Background(), cacheKey, val, mm.cacheTTL)
	}

	return active
}

// bustCache invalidates the Redis cache for the given modules.
func (mm *moduleManager) bustCache(ctx context.Context, tenantID uuid.UUID, moduleIDs []string) {
	if mm.redis == nil {
		return
	}
	for _, id := range moduleIDs {
		cacheKey := fmt.Sprintf("module:%s:active:%s", id, tenantID)
		mm.redis.Del(ctx, cacheKey)
	}
}

// validateModuleIDs checks that all provided IDs are registered.
func (mm *moduleManager) validateModuleIDs(moduleIDs []string) error {
	for _, id := range moduleIDs {
		if _, ok := mm.manifests[id]; !ok {
			return sdk.NotFound("module", id)
		}
	}
	return nil
}

// orderedManifests returns manifests in dependency order.
func (mm *moduleManager) orderedManifests() []sdk.Manifest {
	if len(mm.depOrder) == 0 {
		// Fallback: sort by ID for stable output.
		ids := make([]string, 0, len(mm.manifests))
		for id := range mm.manifests {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		result := make([]sdk.Manifest, 0, len(ids))
		for _, id := range ids {
			result = append(result, mm.manifests[id])
		}
		return result
	}

	result := make([]sdk.Manifest, 0, len(mm.depOrder))
	for _, id := range mm.depOrder {
		if m, ok := mm.manifests[id]; ok {
			result = append(result, m)
		}
	}
	return result
}

// toModuleInfoSDK converts a manifest to an sdk.ModuleInfo.
func toModuleInfoSDK(m sdk.Manifest) sdk.ModuleInfo {
	return sdk.ModuleInfo{
		ID:          m.ID,
		Name:        m.Name,
		Version:     m.Version,
		Type:        m.Type,
		Description: m.Description,
		DependsOn:   m.DependsOn,
		Tags:        m.Tags,
	}
}

// toSDKActivation converts an internal activation record to an sdk.ModuleActivation.
func (mm *moduleManager) toSDKActivation(a internal.ModuleActivation) *sdk.ModuleActivation {
	tenantID, _ := uuid.Parse(a.TenantID)
	activatedBy, _ := uuid.Parse(a.ActivatedBy)

	cfg := sdk.ModuleConfig(a.Config)
	if cfg == nil {
		cfg = sdk.ModuleConfig{}
	}

	result := &sdk.ModuleActivation{
		ModuleID:    a.ModuleID,
		TenantID:    tenantID,
		Active:      a.Active,
		Config:      cfg,
		ExpiresAt:   a.ExpiresAt,
		ActivatedBy: activatedBy,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}

	// Enrich with manifest metadata.
	if m, ok := mm.manifests[a.ModuleID]; ok {
		result.ModuleName = m.Name
		result.ModuleType = m.Type
	}

	return result
}

// ── Config management ────────────────────────────────────────────────────────

func (mm *moduleManager) GetConfig(ctx context.Context, moduleID string, tenantID uuid.UUID) (sdk.ModuleConfig, error) {
	if _, ok := mm.manifests[moduleID]; !ok {
		return nil, sdk.NotFound("module", moduleID)
	}

	// Check Redis cache first.
	if mm.redis != nil {
		cacheKey := fmt.Sprintf("config:%s:%s", moduleID, tenantID)
		if cached, err := mm.redis.Get(ctx, cacheKey).Bytes(); err == nil {
			var cfg sdk.ModuleConfig
			if json.Unmarshal(cached, &cfg) == nil {
				return cfg, nil
			}
		}
	}

	// Fall back to database.
	var activation internal.ModuleActivation
	result := mm.db.WithContext(ctx).
		Select("config").
		Where("module_id = ? AND tenant_id = ?", moduleID, tenantID.String()).
		First(&activation)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return sdk.ModuleConfig{}, nil
		}
		return nil, fmt.Errorf("kernel: get config: %w", result.Error)
	}

	cfg := sdk.ModuleConfig(activation.Config)
	if cfg == nil {
		cfg = sdk.ModuleConfig{}
	}

	// Cache the result.
	if mm.redis != nil {
		cacheKey := fmt.Sprintf("config:%s:%s", moduleID, tenantID)
		if data, marshalErr := json.Marshal(cfg); marshalErr == nil {
			mm.redis.Set(ctx, cacheKey, data, mm.cacheTTL)
		}
	}

	return cfg, nil
}

func (mm *moduleManager) SetConfig(ctx context.Context, moduleID string, tenantID uuid.UUID, values sdk.ModuleConfig) error {
	manifest, ok := mm.manifests[moduleID]
	if !ok {
		return sdk.NotFound("module", moduleID)
	}

	// Validate required fields against the manifest schema.
	for _, field := range manifest.Config {
		if !field.Required {
			continue
		}
		v, exists := values[field.Key]
		if !exists || v == nil || v == "" {
			return sdk.BadRequest("required config field missing: " + field.Key)
		}
	}

	// Update the config column.
	result := mm.db.WithContext(ctx).
		Model(&internal.ModuleActivation{}).
		Where("module_id = ? AND tenant_id = ?", moduleID, tenantID.String()).
		Updates(map[string]any{
			"config":     values,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("kernel: set config: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return sdk.NotFound("activation", moduleID)
	}

	// Bust the config cache.
	if mm.redis != nil {
		cacheKey := fmt.Sprintf("config:%s:%s", moduleID, tenantID)
		mm.redis.Del(ctx, cacheKey)
	}

	return nil
}
