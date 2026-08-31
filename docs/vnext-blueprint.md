# Kernel vNext Blueprint

## Decision

Kernel vNext should be a Kubernetes-inspired SaaS control plane, not a
Kubernetes clone.

Kubernetes-inspired means:

- Stable resource identity: group, version, kind, namespace/scope, and name.
- Typed resources with `kind`, `version`, `metadata`, `spec`, and `status`
  where useful.
- Conditions for asynchronous or controller-managed state.
- Generated clients for known resources.
- A dynamic client for custom/runtime resources.
- Admission, defaulting, validation, authorization, and entitlement checks before
  writes.
- RBAC, service accounts, and signed service-to-service delegation.
- `generation` and `resourceVersion` for managed resources that need
  optimistic concurrency or controller reconciliation.
- Controller-style services that own reconciliation, status, events, and domain
  side effects.
- Declarative and imperative APIs where each fits the product workflow.

It does not mean:

- Running product domains inside Kubernetes CRDs.
- Exposing raw Kubernetes APIs to product clients.
- Treating the control plane as a generic database.
- Forcing all products or modules into one global shared deployment.

The goal is to keep the productive parts of the current kernel while removing the
single-binary deployment ceiling for strategic SaaS products.

Reference consumers:

- Tawajud / HR OS
- TabeebX / Healthcare OS
- Telry
- Insurance Core
- Maritime SaaS

Use vNext as the long-term target:

- Existing products remain legacy/source systems during migration.
- Current kernel remains a useful staging ground and implementation reference.
- vNext becomes the platform shape for large-scale SaaS products.

## Repository Model

`vnext` is the control-plane repository.

It owns the core primitives:

- Resource identity and metadata model
- Tenants and scopes
- Principals/users
- Memberships
- RBAC
- Service accounts
- Entitlements and admission hooks
- Resource definitions and registry
- Generated core clients
- Dynamic client
- Controller/runtime interfaces

Shared modules, controllers, and CRDs live under `github.com/vnext-contrib`.

Examples:

```text
github.com/vnext-contrib/attendance
github.com/vnext-contrib/hierarchy
github.com/vnext-contrib/appointments
github.com/vnext-contrib/payments
github.com/vnext-contrib/prescriptions
github.com/vnext-contrib/reports
```

Product-specific modules can still live in product organizations when they are
not reusable enough for `vnext-contrib`.

## Product Deployment Model

Each SaaS product gets its own vNext deployment and database.

Examples:

```text
hr-os
healthcare-os
telry
insurance-core
```

There is no shared global control plane across all products in the first version.

Each product deployment contains:

```text
frontend/mobile clients
  -> product BFF
  -> control plane API
  -> domain API servers
  -> Postgres + Redis + outbox/events
```

The BFF is product-facing. The control plane is platform-facing. Domain API
servers own business behavior.

## Teleport Reference Takeaways

Teleport is a useful reference, but not a deployment model to copy directly.
Teleport still ships many capabilities in a single Go product. vNext should
borrow its resource/API discipline while keeping product/domain APIs
independently deployable where needed.

Useful patterns to adopt:

- Keep a small public API/client module separate from the server/kernel module.
  Do not make product BFFs or domain services import kernel internals.
- Define first-party resources in protobuf with a stable resource shape:
  kind/version/metadata/spec/status.
- Treat `spec` as desired state owned by the caller and `status` as runtime
  state owned by services/controllers.
- Store secrets as separate resources instead of hiding them behind
  `with_secrets` flags on normal resources.
- Use standard CRUD/list/status RPCs for every first-party resource.
- Make list APIs paginated from day one.
- Use opaque resource revisions for optimistic concurrency.
- Prefer conditional updates over blind overwrites.
- Validate strictly before writes, but keep reads tolerant so old stored data
  does not break caches or operators.
- Add every resource through a checklist: proto, backend service, public client,
  event/watch support, CLI export/apply/edit support, bootstrap/import support,
  and optional operator/Terraform support.
- Keep server-side admission/defaulting/conversion as the source of truth.
  Operators and generated clients should stay thin.
- For breaking storage/resource changes, use new versions/key ranges and phased
  read/write migration instead of in-place destructive migration.
- Track configuration origin explicitly, for example defaults, config file,
  dynamic, migration, subscription, or admin override.

Patterns to avoid copying blindly:

- Do not make every feature part of one binary just because Teleport does.
- Do not expose all resources only through one giant service.
- Do not let old resource shapes force us into long-term legacy behavior before
  vNext has shipped.

## Boundaries

### Control Plane Owns

- Projects/product identity
- Tenants and tenant hierarchy
- Product scopes such as branches, teams, partners, corporate clients, sites, or
  departments
- Principals/users
- Memberships
- RBAC roles and role bindings
- Effective feature entitlements
- Seat limit enforcement
- Module/resource registry
- Resource definitions
- Admission validation
- Audit log
- Service accounts
- Signed delegation verification
- Cross-service request context

### Subscription Module Owns

- Plan catalog
- Pricing
- Subscriptions
- Trials
- Billing periods
- Invoices and payments
- Enterprise contract rules
- Commercial source of purchased limits

Subscription is a producer of entitlement intent. The control plane owns the
effective entitlement state used for runtime enforcement.

Example:

```text
subscription module:
  hospital-plan includes attendance, reports, payroll, 50 active seats

control plane:
  tenant hospital-a can use attendance, reports, payroll
  tenant hospital-a active_member limit is 50
```

### Domain API Servers Own

- Typed domain resources and tables
- Domain validation
- Domain workflows
- Domain migrations
- Reconciliation logic for their own resources
- Domain events
- Read models/projections

Examples:

```text
attendance API server
shifts API server
appointments API server
catalog API server
prescriptions API server
payments API server
threads API server
requests/leaves API server
payroll API server
finance API server
reports API server
hierarchy API server
```

### Product BFF Owns

- Mobile/web-friendly routes
- Aggregation
- Response shaping
- Permission-aware navigation models
- Product workflows that span services
- Backward-compatible response envelopes

The BFF should not own business state. It should consume the control plane and
domain API servers through generated clients.

## Tenants and Product Scopes

The control plane should model tenants and scopes generically.

```text
platform tenant
  -> organization tenant
       -> scope tenant
```

The exact meaning of a child scope is product-specific.

Examples:

```text
HR OS:
  organization tenant = company / contract / billing boundary
  scope tenant        = branch / work location

Healthcare OS:
  organization tenant = TabeebX platform org, partner pharmacy, corporate client
  scope tenant        = team, partner branch, corporate department, clinic/site
```

This supports:

- A user as admin in one organization.
- A user as manager in one branch, team, partner, or corporate scope.
- A user as employee/member/patient in another organization.
- Hospital contracts with many physical sites.
- Healthcare partner and corporate contracts without inventing separate access
  models.

Domain data should not use an ambiguous single `tenant_id` when scope matters.
Prefer explicit fields:

```text
org_tenant_id
scope_tenant_id
```

or domain-specific names:

```text
company_tenant_id / branch_tenant_id
partner_tenant_id / team_tenant_id
corporate_tenant_id / department_tenant_id
```

Seat, quota, and plan limits are enforced at the owning organization/contract
tenant level unless the entitlement explicitly says otherwise.

## Reference Product Mappings

### Tawajud / HR OS

Control-plane mapping:

```text
company       -> organization tenant
branch        -> scope tenant
user          -> principal
employee      -> tenant member + employee profile
manager/admin -> role binding
plan seats    -> entitlement limit on organization tenant
```

Domain API servers:

```text
attendance
shifts
requests/leaves
payroll
finance
reports
hierarchy
```

### TabeebX / Healthcare OS

Control-plane mapping:

```text
patient              -> principal + patient profile
medical staff        -> principal + staff profile
partner pharmacy     -> organization tenant or partner tenant
team                 -> scope tenant under partner/corporate org
corporate client     -> organization tenant
corporate employee   -> tenant member + beneficiary profile
support agent        -> role binding
investor             -> role binding with read-only analytics scope
corporate plan quota -> entitlement limit
```

Domain API servers:

```text
catalog
appointments
scheduling
prescriptions
payments
threads/chat
notifications
partners
corporate plans
analytics
```

## Domain Organization Models

Do not bake product-specific hierarchy into IAM.

IAM/control plane should know tenants, scopes, users, memberships, roles, and
role bindings. Product hierarchy belongs in product/domain API servers.

For HR OS, the hierarchy API server owns:

- Org units
- Departments
- Teams
- Positions
- Employee assignments
- Reporting lines
- Approval chains

For Healthcare OS, domain API servers own:

- Medical specialties/majors
- Doctor pools
- Staff availability
- Partner pharmacy referral model
- Corporate employee benefit structure
- Clinical workflow state

HR example:

```text
hospital org tenant
  -> branch tenant: Baghdad hospital

hierarchy domain:
  Baghdad hospital
    -> Nursing department
    -> ER unit
    -> Night shift team
    -> Head nurse position
```

Healthcare example:

```text
TabeebX platform org
  -> partner tenant: Pharmacy A
  -> corporate tenant: Company B

appointments/scheduling domain:
  Cardiology pool
    -> Doctor A
    -> Doctor B
    -> Doctor C
```

RBAC can later reference these domain scopes, but IAM should not become the
domain hierarchy model itself.

## API Surfaces

Use two API styles.

Product API:

```text
/api/*
```

Used by mobile/web clients. Returns the product/BFF response envelope. Existing
products can keep their current envelope during migration.

Platform API:

```text
/apis/*
```

Used by BFFs, controllers, operators, and dynamic clients. Returns
Kubernetes-like resources/lists/status responses without the product envelope.

Known product workflows should use typed BFF endpoints. Unknown/runtime
resources may use a guarded catch-all route only after authentication,
authorization, resource definition lookup, validation, and audit.

## Client Model

Do not keep relying on:

```go
sdk.Client[any]
```

vNext should use a Kubernetes-like client split:

```go
client.CoreV1().Tenants().Get(ctx, name, opts)
client.IAMV1().Members(namespace).List(ctx, opts)
client.AttendanceV1().Records(scope).Get(ctx, name, opts)
client.Dynamic().Resource(gvr).Namespace(ns).Get(ctx, name, opts)
```

Use generated typed clients for first-party domains:

```text
control-plane client
attendance client
shifts client
appointments client
catalog client
payments client
prescriptions client
payroll client
hierarchy client
finance client
```

Use the dynamic client only for runtime/custom resources.

## Resource Definitions

Use protobuf plus Buf for first-party resource contracts where behavior and
compatibility matter.

Generate from proto:

- Go types
- Go typed clients
- OpenAPI
- JSON Schema
- validation/admission stubs
- resource definition manifests

Use YAML/JSON Schema directly for runtime/customer-defined resources.

Rule:

```text
First-party/core resources -> proto + buf
Runtime/custom resources   -> ResourceDefinition + JSON Schema
```

## Data Ownership

Control plane owns control-plane tables.

Domain API servers own their typed domain tables.

Generic `resources` table is for:

- Configuration
- Desired state
- Controller-managed state
- Dynamic/custom objects
- Low/medium-volume resources

Typed domain tables are for:

- Attendance records
- Attendance events
- Shifts
- Appointments
- Booked slots
- Payments
- Prescriptions
- Medical staff availability
- Payroll runs
- Finance ledger entries
- Notifications/logs
- High-volume reporting data

Do not put high-volume operational data into generic JSON resources unless it is
only a short-lived migration bridge.

## Migration Ownership

Each owner owns its migrations:

- Control plane migrates control-plane schema.
- Attendance API server migrates attendance schema.
- Shifts API server migrates shifts schema.
- Appointments API server migrates appointment/scheduling schema.
- Catalog API server migrates category/service schema.
- Payments API server migrates payment schema.
- Hierarchy API server migrates hierarchy schema.
- Subscription module migrates billing/subscription schema.

The platform deployment can orchestrate migration execution, but ownership stays
with the service/module that owns the tables.

Runtime resource definitions are versioned API objects. Installing a new
ResourceDefinition is a control-plane operation. If it needs projections or
custom tables, the owning controller/domain service must ship and apply those
migrations.

## Auth and Request Context

BFF verifies the external user token using the product auth provider.

BFF calls downstream services using a product service account plus signed
delegation context.

Downstream services trust only:

- mTLS/service-account identity, and
- verified signed delegation context

Normalized request context:

```go
type RequestContext struct {
    ProjectID        string
    OrgTenantID      string
    ScopeTenantID    string
    ScopeKind        string
    PrincipalID      string
    MembershipID     string
    AuthProviderID   string
    AuthSubject      string
    Roles            []string
    Permissions      []string
    EnabledFeatures  []string
}
```

Domain services should be testable with this context directly. They should not
require a real JWT in business tests.

## Runtime Entitlement Checks

All feature checks should go through the control plane admission/entitlement
surface.

Examples:

```text
Can tenant X use attendance?
Can tenant X create another active member?
Can tenant X access reports?
Can tenant X create branch number 51?
Can corporate employee Y book another online consultation this month?
Can partner team Z view commission reports?
```

The control plane should maintain an effective entitlement snapshot so domain
services are not coupled to billing availability.

## Response Envelope Rule

Keep current product envelope for product APIs:

```text
/api/*
```

Use Kubernetes-like objects for platform APIs:

```text
/apis/*
```

This keeps mobile/web compatibility while allowing clean platform/client-go
semantics.

## Migration Shapes

### Tawajud / HR OS

Start from the current Tawajud Go API because it already separates many domains
better than Laravel.

Migration order:

1. Define control-plane primitives: tenants, scopes, users, members, roles, role
   bindings, feature grants, effective entitlements.
2. Normalize Tawajud company/branch into organization tenant/scope tenant.
3. Move fixed `owner`, `branch_manager`, `employee` logic into RBAC.
4. Extract attendance and shifts as the first domain API boundary.
5. Extract requests/leaves because attendance corrections already cross that
   boundary.
6. Add hierarchy API server for hospital-grade organization structure.
7. Move payroll/finance/reports after attendance and hierarchy stabilize.

### TabeebX / Healthcare OS

Start from the current TabeebX API because it already identifies the key product
modules: auth, users, teams, RBAC, catalog, appointments, payments, threads, and
onboarding.

Migration order:

1. Define control-plane primitives for users, roles, teams/partner scopes,
   corporate tenants, and effective entitlements.
2. Normalize platform roles and team-scoped roles into role bindings.
3. Extract catalog/service definitions as a catalog domain API.
4. Extract appointments and scheduling as the first healthcare transactional
   domain API.
5. Add payments as its own domain API, because appointment booking depends on
   payment state.
6. Add prescriptions as a typed clinical domain, not a generic JSON resource.
7. Add partners/corporate plans after appointments and payments have stable
   contracts.
8. Add analytics/reporting projections once transactional domains emit stable
   events.

Current product APIs can be used as implementation references, but in-process
or same-package coupling should become generated network clients in vNext.

## First MVP Slice

Build only enough vNext to prove the architecture:

1. Control plane core schema.
2. Tenant/scope model.
3. Membership and RBAC.
4. Feature grants and quota/seat limit enforcement.
5. Signed request context.
6. Generated client for control plane.
7. One product domain API server using control-plane authz/admission.
8. One product BFF endpoint that aggregates current user, available scopes, and
   one transactional domain summary.

HR success case:

```text
Hospital tenant has 50 active seats and attendance enabled.
User logs in through BFF.
BFF resolves company + branch options.
BFF calls attendance API using signed context.
Attendance API checks control-plane entitlements/RBAC.
Mobile receives the same product-friendly home response shape.
```

Healthcare success case:

```text
Corporate tenant has online consultation quota enabled.
Patient logs in through BFF.
BFF resolves user role, corporate/partner context, and service catalog access.
BFF calls appointments API using signed context.
Appointments API checks control-plane entitlements/RBAC.
Appointments API reserves a slot and emits booking/payment events.
Mobile receives the same product-friendly appointment response shape.
```

## Open Decisions

- Whether vNext MVP runs control plane and first domain APIs as separate
  processes immediately, or starts as one repo with separate binaries.
- Whether domain API servers share one Postgres database with schemas or each
  has its own database from day one.
- Whether first-party APIs are REST+OpenAPI first, gRPC first, or dual generated
  from protobuf.
- Exact resource naming conventions for `/apis/{group}/{version}`.
- How much of current kernel module lifecycle is kept versus replaced by
  ResourceDefinitions and generated clients.
