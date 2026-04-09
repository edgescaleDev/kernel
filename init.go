package kernel

import (
	"fmt"
	"log/slog"

	"go.edgescale.dev/kernel/sdk"
	"gorm.io/gorm"
)

// initModules builds an sdk.Context for each module and calls Init() in
// topological dependency order. This ensures that when a module initializes,
// all its dependencies are already available.
func (k *Kernel) initModules() error {
	for _, m := range k.Modules() {
		manifest := m.Manifest()
		moduleID := manifest.ID

		ctx := k.buildContext(manifest)

		k.logger.Info("initializing module", "id", moduleID)
		if err := m.Init(ctx); err != nil {
			return fmt.Errorf("init %q: %w", moduleID, err)
		}

		// Let the module register its EventBus subscriptions.
		m.RegisterEvents(k.bus)

		// Let the module register its sync hooks.
		m.RegisterHooks(k.hooks)

		// Let the module register Temporal workflows/activities.
		if k.workflows != nil {
			m.RegisterWorkflows(k.workflows)
		}
		if k.activities != nil {
			m.RegisterActivities(k.activities)
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

	ctx := sdk.Context{
		PublicDB:           k.db,
		Logger:             logger,
		Bus:                k.bus,
		Tasks:              k.taskExecutor,
		Search:             k.searchEngine,
		Hooks:              k.hooks,
		IdentityProvider:   k.identityProvider,
		Audit:              newAuditLogger(k.db, moduleID),
		Outbox:             &outboxWriter{db: k.db, moduleID: moduleID},
		ServiceID:          moduleID,
		ValidPermissionKey: k.ValidPermissionKey,
	}
	ctx.SetReaders(k.readers)

	// Scoped Redis: all keys prefixed with "module:{id}:".
	if k.redis != nil {
		ctx.Redis = sdk.NewNamespacedRedis(k.redis, moduleID)
	}

	// Scoped DB: set search_path to the module's schema.
	if k.db != nil && manifest.Schema != "" && manifest.Schema != "public" {
		ctx.DB = k.db.Session(&gorm.Session{}).Exec(
			fmt.Sprintf("SET search_path TO %s, public", manifest.Schema),
		)
	} else {
		ctx.DB = k.db
	}

	return ctx
}
