package kernel

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.edgescale.dev/kernel/sdk"
	"gorm.io/gorm"
)

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
	taskExecutor sdk.TaskExecutor
	searchEngine sdk.SearchEngine
	workflows    sdk.WorkflowRegistry
	activities   sdk.ActivityRegistry

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
