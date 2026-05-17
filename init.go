package kernel

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/kernel-contrib/sdk"
)

// initModules builds an sdk.Context for each module and calls Init() in
// topological dependency order. This ensures that when a module initializes,
// all its dependencies are already available.
func (k *Kernel) initModules() error {
	for _, m := range k.orderedModules() {
		manifest := m.Manifest()
		moduleID := manifest.ID

		ctx := k.buildContext(manifest)

		k.logger.Info("initializing module", "id", moduleID)
		if err := m.Init(ctx); err != nil {
			return fmt.Errorf("init %q: %w", moduleID, err)
		}

		// Optional: let the module register its EventBus subscriptions.
		if em, ok := m.(sdk.EventModule); ok {
			em.RegisterEvents(k.bus)
		}

		// Optional: let the module register its sync hooks.
		if hm, ok := m.(sdk.HookModule); ok {
			hm.RegisterHooks(k.hooks)
		}

		// Optional: let the module register Temporal workflows/activities.
		if wm, ok := m.(sdk.WorkflowModule); ok {
			if k.workflows != nil {
				wm.RegisterWorkflows(k.workflows)
			}
			if k.activities != nil {
				wm.RegisterActivities(k.activities)
			}
		}

		k.logger.Info("module ready", "id", moduleID)
	}
	return nil
}

// buildContext creates an sdk.Context scoped to a specific module.
// Each module gets its own isolated database schema, namespaced Redis,
// and a logger tagged with the module ID.
func (k *Kernel) buildContext(manifest sdk.Manifest) sdk.Context {
	moduleID := manifest.ID
	logger := slog.Default().With("module", moduleID)

	// Use pluggable implementations with noop fallbacks.
	audit := k.auditLogger
	if audit == nil {
		audit = &sdk.TestAuditLogger{}
	}

	var outbox sdk.OutboxWriter
	if k.outboxWriter != nil {
		outbox = k.outboxWriter
	}

	ctx := sdk.Context{
		PublicDB:           k.db,
		Logger:             logger,
		Bus:                k.bus,
		Tasks:              k.taskExecutor,
		Search:             k.searchEngine,
		Hooks:              k.hooks,
		IdentityProvider:   k.identityProvider,
		Audit:              audit,
		Outbox:             outbox,
		Operations:         k.operationTracker,
		Features:           k.featureFlags,
		Lock:               k.lockProvider,
		Storage:            k.objectStore,
		Modules:            newModuleManager(k),
		ServiceID:          moduleID,
		ValidPermissionKey: k.validPermissionKey,
		AllPermissions:     k.allPermissions,
	}

	// Platform tenant resolver: delegates to the pluggable implementation
	// or returns a clear error when no resolver is configured.
	if k.platformTenantResolver != nil {
		ctx.PlatformTenantID = k.platformTenantResolver.ResolvePlatformTenantID
	} else {
		ctx.PlatformTenantID = func(_ context.Context) (uuid.UUID, error) {
			return uuid.Nil, fmt.Errorf("kernel: no platform tenant resolver configured; call SetPlatformTenantResolver before Boot()")
		}
	}

	ctx.SetReaders(k.readers)

	// Scoped Redis: all keys prefixed with "module:{id}:".
	if k.redis != nil {
		ctx.Redis = sdk.NewNamespacedRedis(k.redis, moduleID)
	}

	// Scoped DB: use a session callback to set search_path per-query.
	if k.db != nil && manifest.Schema != "" && manifest.Schema != "public" {
		ctx.DB = scopedDB(k.db, manifest.Schema)
	} else {
		ctx.DB = k.db
	}

	return ctx
}
