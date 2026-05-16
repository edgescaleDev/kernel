// Package main demonstrates a minimal kernel application.
//
// Run with: go run main.go serve
package main

import (
	"io/fs"
	"testing/fstest"

	"github.com/edgescaleDev/kernel"
	"github.com/kernel-contrib/sdk"
	"github.com/gin-gonic/gin"
)

// ─────────────────────────────────────────────────────────────────────────────
// Module: Hello
// A minimal module that exposes a single public endpoint.
// ─────────────────────────────────────────────────────────────────────────────

// HelloModule implements sdk.Module and sdk.HttpModule.
type HelloModule struct{}

func (m *HelloModule) Manifest() sdk.Manifest {
	return sdk.Manifest{
		ID:          "hello",
		Name:        "Hello Module",
		Version:     "1.0.0",
		Type:        sdk.TypeCore,
		Schema:      "module_hello",
		Description: "A simple hello world module.",
	}
}

func (m *HelloModule) Migrations() fs.FS {
	return fstest.MapFS{}
}

func (m *HelloModule) Init(ctx sdk.Context) error {
	ctx.Logger.Info("Hello module initialized!")
	return nil
}

func (m *HelloModule) RouteHandlers() []sdk.RouteHandler {
	return []sdk.RouteHandler{
		{
			Type: sdk.RouteClient,
			Register: func(r *sdk.Router) {
				// Public endpoint — no authentication required.
				r.GET("/hello", sdk.Public, func(c *gin.Context) {
					c.JSON(200, gin.H{
						"message": "Hello from the Kernel!",
						"module":  "hello",
					})
				})
			},
		},
	}
}

func (m *HelloModule) Shutdown() error {
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Main
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	cfg := kernel.DefaultConfig()
	cfg.Dev.Mode = true

	k := kernel.New(cfg)

	// Register modules.
	k.MustRegister(&HelloModule{})

	// Pluggable implementations — set before Boot().
	// These are provided by external packages (IAM module, Firebase, etc.):
	//
	//   k.SetIdentityProvider(firebase.New(ctx))
	//   k.SetUserResolver(iam.NewUserResolver(db))
	//   k.SetAdminResolver(iam.NewAdminResolver(db))
	//   k.SetAuditLogger(audit.NewLogger(db))
	//   k.SetOutboxWriter(outbox.NewWriter(db))
	//   k.SetEventBus(nats.NewBus(conn))
	//   k.SetTaskExecutor(temporal.New(client))
	//   k.SetSearchEngine(meilisearch.New(host, key))
	//   k.SetOperationTracker(operations.NewTracker(db))
	//   k.SetFeatureFlags(featureflags.New(db, rdb))
	//   k.SetPlatformTenantResolver(iamModule)  // exposes platform tenant ID to all modules

	// Start: go run main.go serve
	// Migrate: go run main.go migrate
	// List modules: go run main.go module list
	k.Execute()
}
