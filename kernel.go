package kernel

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"

	"github.com/edgescaleDev/kernel/internal"
	"github.com/edgescaleDev/kernel/sdk"
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
	userResolver     sdk.UserResolver
	adminResolver    sdk.AdminResolver
	auditLogger      sdk.AuditLogger
	outboxWriter     sdk.OutboxWriter
	operationTracker sdk.OperationTracker
	featureFlags     sdk.FeatureFlags
	lockProvider     sdk.LockProvider

	// Cron scheduler.
	cronRunner  *cronRunner
	cronEntries []cronEntry

	// Custom CLI commands registered by the consumer.
	customCommands []*cobra.Command

	// Shutdown coordination.
	shutdownOnce sync.Once
}

// New creates a new Kernel instance with the given configuration.
func New(cfg Config) *Kernel {
	logger := slog.Default().With("component", "kernel")

	return &Kernel{
		cfg:              cfg,
		logger:           logger,
		manifests:        make(map[string]sdk.Manifest),
		hooks:            sdk.NewHookRegistry(),
		readers:          sdk.NewReaderRegistry(),
		identityProvider: internal.NoopIdentityProvider{},
	}

}

// Register adds a compiled-in module to the kernel.
// Must be called before Boot(). Returns an error on duplicate or reserved module IDs.
func (k *Kernel) Register(m sdk.Module) error {
	manifest := m.Manifest()
	if err := validateModuleID(manifest.ID); err != nil {
		return fmt.Errorf("kernel: %w", err)
	}
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

// validateModuleID checks that a module ID won't collide with kernel routes.
func validateModuleID(id string) error {
	if id == "" {
		return fmt.Errorf("module ID must not be empty")
	}
	if strings.HasPrefix(id, "_") {
		return fmt.Errorf("module ID %q is reserved (starts with _)", id)
	}
	if id == "kernel" {
		return fmt.Errorf("module ID %q is reserved", id)
	}
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

// SetUserResolver sets the pluggable user resolver (IAM module, external IdP, etc.).
// Must be called before Serve(). If not set, resolveUser middleware will reject all requests.
func (k *Kernel) SetUserResolver(resolver sdk.UserResolver) {
	k.userResolver = resolver
}

// SetAdminResolver sets the pluggable admin resolver.
// Must be called before Serve(). If not set, requirePlatformAdmin middleware will reject all requests.
func (k *Kernel) SetAdminResolver(resolver sdk.AdminResolver) {
	k.adminResolver = resolver
}

// SetAuditLogger sets the pluggable audit logger (audit module, etc.).
// Must be called before Serve(). If not set, audit logging silently discards entries.
func (k *Kernel) SetAuditLogger(logger sdk.AuditLogger) {
	k.auditLogger = logger
}

// SetOutboxWriter sets the pluggable outbox writer (outbox module, etc.).
// Must be called before Serve(). If not set, outbox writes are silently discarded.
func (k *Kernel) SetOutboxWriter(writer sdk.OutboxWriter) {
	k.outboxWriter = writer
}

// SetOperationTracker sets the pluggable operation tracker (operations module, etc.).
// Must be called before Serve(). If not set, operation tracking is unavailable.
func (k *Kernel) SetOperationTracker(tracker sdk.OperationTracker) {
	k.operationTracker = tracker
}

// SetFeatureFlags sets the pluggable feature flags (featureflags module, etc.).
// Must be called before Serve(). If not set, all feature checks return false.
func (k *Kernel) SetFeatureFlags(flags sdk.FeatureFlags) {
	k.featureFlags = flags
}

// SetLockProvider sets the distributed lock provider.
// Must be called before Boot(). Used by the cron runner for deduplication
// and available to modules via sdk.Context.Lock.
// If not set, a noop lock that always acquires is used (single-instance mode).
func (k *Kernel) SetLockProvider(provider sdk.LockProvider) {
	k.lockProvider = provider
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

// modules returns the list of registered modules in dependency order.
// Returns registration order if Boot() has not been called.
func (k *Kernel) orderedModules() []sdk.Module {
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

// manifests returns a copy of the manifest map.
// Used by CLI commands to inspect registered modules.
func (k *Kernel) allManifests() map[string]sdk.Manifest {
	result := make(map[string]sdk.Manifest, len(k.manifests))
	maps.Copy(result, k.manifests)
	return result
}

// validPermissionKey returns true if the given key is declared
// by any registered module's manifest. Use this to validate
// permission keys at write-time (e.g., when assigning permissions to roles).
func (k *Kernel) validPermissionKey(key string) bool {
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
