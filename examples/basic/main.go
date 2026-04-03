package main

import (
	"io/fs"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"go.edgescale.dev/kernel"
	"go.edgescale.dev/kernel/sdk"
)

// HelloModule is a minimal implementation of an sdk.Module.
type HelloModule struct{}

// Manifest declares the module's identity and metadata.
func (m *HelloModule) Manifest() sdk.Manifest {
	return sdk.Manifest{
		ID:          "hello",
		Name:        "Hello Module",
		Version:     "1.0.0",
		Type:        sdk.TypeCore, // TypeCore is always active without tenant checks
		Schema:      "module_hello",
		Description: "A simple hello world module.",
	}
}

// Migrations provides the SQL schema updates. We use an empty FS here.
func (m *HelloModule) Migrations() fs.FS {
	return fstest.MapFS{}
}

// Init is called during kernel boot. Set up internal dependencies here.
func (m *HelloModule) Init(ctx sdk.Context) error {
	ctx.Logger.Info("Hello module initialized!")
	return nil
}

// RegisterRoutes mounts the module's API endpoints.
func (m *HelloModule) RegisterRoutes(router *sdk.Router) {
	// sdk.Public makes this accessible without authentication
	router.GET("/hello", sdk.Public, func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Hello from the new Kernel Framework!",
			"module":  "hello",
		})
	})
}

// RegisterEvents subscribes the module to asynchronous events.
func (m *HelloModule) RegisterEvents(bus sdk.EventBus) {}

// RegisterHooks registers synchronous interceptors across modules.
func (m *HelloModule) RegisterHooks(hooks *sdk.HookRegistry) {}

// RegisterWorkflows registers Temporal workflows.
func (m *HelloModule) RegisterWorkflows(reg sdk.WorkflowRegistry) {}

// RegisterActivities registers Temporal activities.
func (m *HelloModule) RegisterActivities(reg sdk.ActivityRegistry) {}

// Shutdown gracefully cleans up resources before exiting.
func (m *HelloModule) Shutdown() error {
	return nil
}

func main() {
	// Create a default configuration (usually loaded via Viper & env vars)
	cfg := kernel.DefaultConfig()
	cfg.Dev.Mode = true

	// Instantiate the kernel
	k := kernel.New(cfg)

	// Register our capability module
	k.MustRegister(&HelloModule{})

	// Start the CLI execution (runs server or migration commands)
	// Try running: go run main.go serve
	k.Execute()
}
