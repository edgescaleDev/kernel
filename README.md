# Kernel Framework

The **Kernel Framework** (`go.edgescale.dev/kernel`) is a domain-agnostic, microkernel-based modular Go framework designed to power the SaaS OS. It provides a robust, enterprise-grade foundation for building a modular monolith with strict tenant isolation, isolated module boundaries, and standardized infrastructure patterns.

## Features

- **Microkernel Architecture**: Core infrastructure lifecycle (Postgres, Redis, Web Server), module discovery, and dependency injection.
- **Module Provider Interface (MPI)**: Strict interfaces `sdk.Module` and `sdk.Manifest` for module definitions, ensuring encapsulated logic, self-contained migrations, and explicit cross-module dependencies.
- **Multi-Tenancy & Tenant Isolation**: Built-in tenant scoping strategies, global Row-Level Security (RLS) enforcement, and custom per-tenant module activation toggles.
- **Pluggable Event-Driven Primitives**:
  - **Hook Registry**: Synchronous interceptors spanning across modules.
  - **Event Bus**: Asynchronous publish/subscribe message bus.
  - **Task & Workflow Engine**: Seamless integration points for registering Temporal workflows and activities.
- **Infrastructure Standardization**:
  - GORM-powered Postgres abstractions.
  - Redis integration for caching, idempotency, and outbox patterns.
  - Robust transactional outbox implementation for reliable event delivery.
- **Built-in Developer CLI**: Uses Cobra to offer operational commands out of the box (`serve`, `migrate`, `module`, `org`).

## Module Configuration

To create a module in this framework, you implement the `sdk.Module` interface and provide your declarative `sdk.Manifest`.

```go
type Manifest struct {
  ID                  string
  Name                string
  Version             string
  Type                sdk.ModuleType // Core, Feature, or Integration
  Schema              string         // Custom isolated postgres schema matching the module
  DependsOn           []string       // Explicit dependency graph formulation
  PublicEvents        []sdk.EventDef
  Permissions         []sdk.Permission
  // ... config, UI nav mapping, and more.
}
```

## CLI Usage

The kernel provides built-in commands designed for operations, tenant management, and deployment via the framework's Cobra integration.

### Starting the Server

```bash
./kernel serve
```

### Migrations

The kernel orchestrates and runs migrations across all registered modules in topological dependency order.

```bash
./kernel migrate          # Run all pending migrations
./kernel migrate status   # Display the migration footprint across modules
```

### Module Management

Managing the registered capabilities and auditing their dependency graph.

```bash
./kernel module list
./kernel module deps
./kernel module enable [module-id] --org [org-id]
./kernel module disable [module-id] --org [org-id]
./kernel module status --org [org-id]
```

### Organization (Tenant) Provisioning

```bash
./kernel org provision [org-id]
./kernel org list
./kernel org deprovision [org-id] --confirm
```

## Getting Started

1. Set up dependencies: `go mod download`.
2. Ensure you have the required infrastructure (Postgres & Redis).
3. Wire your system by initializing `kernel.New(cfg)`, registering your modules via `k.Register(my_module.New())`, and running `k.Execute()`.

## Architecture Decisions

The architectural philosophy explicitly decouples the core infrastructure orchestration (Kernel) from domain-specific business features (Modules). This constraint forces:

- **Zero internal drift:** Services only communicate via `sdk.ReaderRegistry`, Events, or Hooks instead of directly mapping internal memory.
- **Easy decomposability:** By having isolated schema declarations (e.g. `module_billing`), transitioning a module inside the monolith into an independent microservice later is trivial.
