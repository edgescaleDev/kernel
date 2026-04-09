package kernel

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"

	"go.edgescale.dev/kernel/modules/iam"
	"go.edgescale.dev/kernel/sdk"
	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var kernelMigrationsFS embed.FS

// KernelMigrations returns the embedded filesystem containing the kernel's
// SQL migration files. Used by the migration orchestrator.
func KernelMigrations() fs.FS {
	sub, _ := fs.Sub(kernelMigrationsFS, "migrations")
	return sub
}

// Kernel is the core runtime that manages modules, infrastructure,
// and the HTTP server lifecycle.
type Kernel struct {
	cfg    Config
	logger *slog.Logger

	// Infrastructure connections (set during Boot).
	db    *gorm.DB
	redis *redis.Client

	// Module management.
	modules   []sdk.Module
	manifests map[string]sdk.Manifest
	depOrder  []string // topological init order

	// HTTP server.
	engine     *gin.Engine
	httpServer *http.Server

	// EventBus + hook registry (shared across all modules).
	bus   sdk.EventBus
	hooks *sdk.HookRegistry

	// Reader registry (shared across all modules for cross-module access).
	readers *sdk.ReaderRegistry

	// Pluggable implementations (set by consumer before Boot).
	identityProvider sdk.IdentityProvider
	taskExecutor     sdk.TaskExecutor
	searchEngine     sdk.SearchEngine
	workflows        sdk.WorkflowRegistry
	activities       sdk.ActivityRegistry

	// Custom CLI commands registered by the consumer.
	customCommands []*cobra.Command

	// Platform org ID (cached during Serve).
	platformOrgID uuid.UUID

	// Shutdown coordination.
	shutdownOnce sync.Once
}

// New creates a new Kernel instance with the given configuration.
func New(cfg Config) *Kernel {
	logger := slog.Default().With("component", "kernel")

	return &Kernel{
		cfg:       cfg,
		logger:    logger,
		manifests: make(map[string]sdk.Manifest),
		hooks:     sdk.NewHookRegistry(),
		readers:   sdk.NewReaderRegistry(),
	}

}

// NewWithIAM creates a new IAM enabled Kernel instance with the given configuration.
func NewWithIAM(cfg Config) *Kernel {
	logger := slog.Default().With("component", "kernel")
	k := &Kernel{
		cfg:       cfg,
		logger:    logger,
		manifests: make(map[string]sdk.Manifest),
		hooks:     sdk.NewHookRegistry(),
		readers:   sdk.NewReaderRegistry(),
	}
	k.MustRegister(iam.New())

	return k
}

// Register adds a compiled-in module to the kernel.
// Must be called before Boot(). Returns an error on duplicate module IDs.
func (k *Kernel) Register(m sdk.Module) error {
	manifest := m.Manifest()
	if _, exists := k.manifests[manifest.ID]; exists {
		return fmt.Errorf("kernel: duplicate module ID %q", manifest.ID)
	}
	k.modules = append(k.modules, m)
	k.manifests[manifest.ID] = manifest
	k.logger.Info("registered module",
		"id", manifest.ID,
		"version", manifest.Version,
		"type", manifest.Type.String(),
	)
	return nil
}

// MustRegister is like Register but panics on error.
// Useful in main() where failure to register is unrecoverable.
func (k *Kernel) MustRegister(m sdk.Module) {
	if err := k.Register(m); err != nil {
		panic(err)
	}
}

// AddCommand allows consumers to register custom CLI commands.
// Must be called before Execute().
func (k *Kernel) AddCommand(cmd *cobra.Command) {
	k.customCommands = append(k.customCommands, cmd)
}

// SetTaskExecutor sets the pluggable task executor (Temporal, inline, etc.).
// Must be called before Boot().
func (k *Kernel) SetTaskExecutor(executor sdk.TaskExecutor) {
	k.taskExecutor = executor
}

// SetSearchEngine sets the pluggable search engine (Meilisearch, noop, etc.).
// Must be called before Boot().
func (k *Kernel) SetSearchEngine(engine sdk.SearchEngine) {
	k.searchEngine = engine
}

// SetIdentityProvider sets the pluggable identity provider (Firebase, Okta, Keycloak, etc.).
// Must be called before Boot(). If not called, all authentication requests are rejected.
func (k *Kernel) SetIdentityProvider(provider sdk.IdentityProvider) {
	k.identityProvider = provider
}

// SetEventBus sets the event bus implementation.
// Must be called before Boot(). If not called, a noop bus is used.
func (k *Kernel) SetEventBus(bus sdk.EventBus) {
	k.bus = bus
}

// Boot connects to infrastructure, validates the dependency graph,
// and prepares the kernel for serving requests.
// This must be called after all modules are registered and pluggable
// implementations are set.
func (k *Kernel) Boot() error {
	k.logger.Info("booting kernel", "modules", len(k.modules), "dev_mode", k.cfg.Dev.Mode)

	// 0. Validate config.
	if err := k.cfg.Validate(); err != nil {
		return fmt.Errorf("kernel: %w", err)
	}

	// 1. Connect to required infrastructure.
	if err := k.connectInfra(); err != nil {
		return fmt.Errorf("kernel: infrastructure connect failed: %w", err)
	}

	// 2. Validate dependency graph and compute topological order.
	order, err := k.validateAndSort()
	if err != nil {
		return fmt.Errorf("kernel: dependency validation failed: %w", err)
	}
	k.depOrder = order

	// 3. Install noop fallbacks for unset pluggable implementations.
	k.installFallbacks()

	k.logger.Info("boot complete", "init_order", k.depOrder)
	return nil
}

// installFallbacks sets noop implementations for pluggable interfaces
// that were not explicitly set by the consumer. This prevents nil
// dereferences when modules access Tasks, Search, or Bus.
func (k *Kernel) installFallbacks() {
	if k.identityProvider == nil {
		k.logger.Warn("no identity provider set - all authentication will be rejected")
		k.identityProvider = noopIdentityProvider{}
	}
	if k.bus == nil {
		k.logger.Warn("no event bus set - using noop bus")
		k.bus = noopEventBus{}
	}
	if k.taskExecutor == nil {
		k.logger.Warn("no task executor set - background tasks will be disabled")
		k.taskExecutor = noopTaskExecutor{}
	}
	if k.searchEngine == nil {
		k.logger.Warn("no search engine set - search will be disabled")
		k.searchEngine = noopSearchEngine{}
	}
}

// Modules returns the list of registered modules in dependency order.
// Returns registration order if Boot() has not been called.
func (k *Kernel) Modules() []sdk.Module {
	if len(k.depOrder) == 0 {
		return k.modules
	}

	moduleMap := make(map[string]sdk.Module, len(k.modules))
	for _, m := range k.modules {
		moduleMap[m.Manifest().ID] = m
	}

	ordered := make([]sdk.Module, 0, len(k.depOrder))
	for _, id := range k.depOrder {
		if m, ok := moduleMap[id]; ok {
			ordered = append(ordered, m)
		}
	}
	return ordered
}

// DB returns the kernel's database connection.
// Returns nil if Boot() has not been called.
func (k *Kernel) DB() *gorm.DB {
	return k.db
}

// Redis returns the kernel's Redis client.
// Returns nil if Boot() has not been called.
func (k *Kernel) Redis() *redis.Client {
	return k.redis
}

// DepOrder returns the topological sort order of module IDs.
// Returns nil if Boot() has not been called.
func (k *Kernel) DepOrder() []string {
	return k.depOrder
}

// Manifests returns a copy of the manifest map.
// Used by CLI commands to inspect registered modules.
func (k *Kernel) Manifests() map[string]sdk.Manifest {
	result := make(map[string]sdk.Manifest, len(k.manifests))
	maps.Copy(result, k.manifests)
	return result
}

// PlatformOrgID returns the cached platform organization ID.
// Returns uuid.Nil if not yet loaded.
func (k *Kernel) PlatformOrgID() uuid.UUID {
	return k.platformOrgID
}

// loadPlatformOrg discovers and caches the platform org ID.
// Called during Serve() after modules are initialized and migrations have run.
func (k *Kernel) loadPlatformOrg() error {
	var result struct {
		ID uuid.UUID
	}
	err := k.db.Raw(
		"SELECT id FROM module_iam.organizations WHERE status = 'platform' LIMIT 1",
	).Scan(&result).Error
	if err != nil {
		return fmt.Errorf("load platform org: %w", err)
	}
	if result.ID == uuid.Nil {
		k.logger.Warn("no platform org found — platform admin features disabled")
		return nil
	}
	k.platformOrgID = result.ID
	k.logger.Info("platform org loaded", "id", k.platformOrgID)
	return nil
}

// ValidPermissionKey returns true if the given key is declared
// by any registered module's manifest. Use this to validate
// permission keys at write-time (e.g., when assigning permissions to roles).
func (k *Kernel) ValidPermissionKey(key string) bool {
	for _, m := range k.manifests {
		for _, p := range m.Permissions {
			if p.Key == key {
				return true
			}
		}
	}
	return false
}

// Execute builds a Cobra command tree and runs it.
// This is the main entry point for consumer applications.
//
//	func main() {
//	    k := kernel.New(kernel.LoadConfig())
//	    k.Register(billing.New())
//	    k.Execute()
//	}
func (k *Kernel) Execute() {
	rootCmd := k.buildRootCommand()
	if err := rootCmd.Execute(); err != nil {
		k.logger.Error("command failed", "error", err)
	}
}
