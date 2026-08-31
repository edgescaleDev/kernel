# Architecture Reference: SaaS Kernel + Control Plane + BFF

See also: [Kernel vNext Blueprint](./vnext-blueprint.md) for the concrete
product-agnostic blueprint and first MVP slices.

## Context

We currently have `https://github.com/edgescaleDev/kernel`.

The existing kernel is a Go-based modular/microkernel-inspired SaaS framework. Current limitation: modules are compiled into the binary, so adding a new module requires a rebuild/redeploy.

We want to evolve it into a reusable SaaS backbone for multiple SaaS products such as:

- HealthCare OS
- HR OS
- Telry
- AI Call Center
- Maritime SaaS
- Insurance Core

Each SaaS project should have a **single deployment per project**, not one global deployment serving all projects.

Example deployments:

```text
healthcare-os
hr-os
telry
ai-callcenter
```

Each project can configure its own auth provider:

```text
HealthCare OS → Keycloak
HR OS         → Firebase phone auth
Telry         → Keycloak / Zitadel / custom OIDC
```

---

# Chosen Direction

Use the existing `edgescaleDev/kernel` as the foundation, but evolve it into a:

```text
Hybrid SaaS Control Plane Kernel
```

Meaning:

```text
Static kernel
+ compiled first-party modules
+ runtime module registry
+ dynamic resource definitions
+ optional external controllers
+ BFFs using a k8s-like client
```

Do **not** replace the kernel with a full Kubernetes clone.

Borrow selected Kubernetes control-plane patterns:

```text
spec/status
conditions
generation / observedGeneration
resourceVersion
reconciliation loops
finalizers
idempotent controllers
status updates
watch/events
```

---

# Deployment Model

Per SaaS project:

```text
Frontend
  ↓
Product BFF
  ↓
K8s-like Platform Client SDK
  ↓
Control Plane API
  ↓
Postgres + Outbox + NATS
  ↓
Controllers / Reconcilers / Workers
```

Example:

```text
Healthcare UI
  → healthcare-bff
    → platform client
      → healthcare-control-plane
        → Postgres/NATS
        → appointment/ehr/ai-callcenter controllers
```

Each project should have its own deployment and preferably its own database.

---

# Major Boundary

## Control Plane owns

```text
resources
modules
schemas
status
conditions
events
outbox
audit
RBAC/policies
tenant isolation
module activation
controller registration
secrets metadata
service accounts
```

## BFF owns

```text
product-specific routes
request/response shaping
frontend aggregation
view models
workflow shortcuts
permission-aware navigation
thin orchestration
```

## Controllers own

```text
reconciliation
long-running workflows
external integrations
AI jobs
telephony jobs
billing sync
status updates
```

The BFF should not become the real backend. It should translate user/admin-facing API calls into control-plane resource operations.

---

# API Rule

Frontend should call product-friendly APIs:

```http
POST /api/appointments
GET /api/patients/:id/timeline
POST /api/ehr-drafts/:id/approve
```

BFF should use a platform client to call the control plane:

```go
client.Resources().Create(...)
client.Resources().Patch(...)
client.Resources().UpdateStatus(...)
client.Modules().IsEnabled(...)
```

The frontend should not directly call the generic control-plane API.

---

# Platform Client SDK

Create a small Kubernetes-like Go client.

Core interface:

```go
type Client interface {
    Resources() ResourceClient
    Modules() ModuleClient
    Watches() WatchClient
    RBAC() RBACClient
}
```

Resource client:

```go
type ResourceClient interface {
    Create(ctx context.Context, obj *UnstructuredResource) error
    Get(ctx context.Context, ref ResourceRef) (*UnstructuredResource, error)
    List(ctx context.Context, opts ListOptions) (*ResourceList, error)
    Patch(ctx context.Context, ref ResourceRef, patch Patch) error
    Delete(ctx context.Context, ref ResourceRef) error
    UpdateStatus(ctx context.Context, ref ResourceRef, status map[string]any) error
}
```

Resource reference:

```go
type ResourceRef struct {
    ProjectID  string
    TenantID   string
    APIGroup   string
    APIVersion string
    Kind       string
    Namespace  string
    Name       string
}
```

Unstructured resource:

```go
type UnstructuredResource struct {
    APIVersion string         `json:"apiVersion"`
    Kind       string         `json:"kind"`
    Metadata   Metadata       `json:"metadata"`
    Spec       map[string]any `json:"spec"`
    Status     map[string]any `json:"status,omitempty"`
}
```

Support both:

```text
typed clients       → for first-party modules
unstructured client → for dynamic/runtime modules
```

---

# Module Types

Formalize three module types.

## 1. Compiled Modules

Go packages registered at boot.

Used for core product modules:

```text
patients
appointments
ehr
employees
attendance
whatsapp
billing
call sessions
```

Requires rebuild/redeploy.

## 2. Declarative Runtime Modules

Installed after deployment using manifests.

Can include:

```text
resource definitions
JSON schemas
permissions
roles
forms
tables
navigation
reports
simple workflows
```

No rebuild required.

Good for simple CRM/data-collection/admin modules.

## 3. Controller-Backed Runtime Modules

Installed after deployment, but custom behavior runs outside the control plane as an external service/controller.

Good for:

```text
WhatsApp integration
LiveKit agents
AI EHR generation
insurance validation
external sync
billing automation
```

---

# Runtime Module Manifest Shape

Example:

```yaml
apiVersion: platform.edgescale.dev/v1
kind: Module
metadata:
  name: dental
  version: 1.0.0

spec:
  type: controller-backed

  resources:
    - kind: Dentist
      plural: dentists
      schemaRef: ./resources/dentist.schema.json

    - kind: DentalAppointment
      plural: dentalappointments
      schemaRef: ./resources/dentalappointment.schema.json

  permissions:
    - dentists:read
    - dentists:create
    - dentalappointments:read
    - dentalappointments:create
    - dentalappointments:update

  ui:
    navigation:
      - label: Dental
        path: /modules/dental
        permission: dentalappointments:read

  controllers:
    - name: dental-controller
      watches:
        - DentalAppointment
      endpoint: https://dental-controller.svc.cluster.local
```

---

# Resource Model

Use Kubernetes-like resources for dynamic/config/controller-managed state.

Example:

```yaml
apiVersion: healthcare.edgescale.dev/v1
kind: Appointment
metadata:
  project: healthcare-os
  tenant: clinic_123
  name: appt_789

spec:
  patientId: pat_123
  doctorId: doc_456
  startsAt: "2026-06-26T10:00:00+03:00"
  type: online-consultation

status:
  phase: Scheduled
  observedGeneration: 1
  conditions:
    - type: DoctorAvailable
      status: "True"
    - type: NotificationSent
      status: "True"
```

Important rule:

```text
Users/BFF modify spec.
Controllers modify status.
```

---

# Database Strategy

Per SaaS project:

```text
one database
```

Inside each database:

```text
control_plane schema
+ module-owned schemas/tables
```

Example:

```text
healthcare_os_db
  control_plane.*
  healthcare.*
  voice.*
  billing.*

hr_os_db
  control_plane.*
  hr.*
  billing.*

telry_db
  control_plane.*
  telry.*
  voice.*
  billing.*
```

---

# Control Plane Tables

Minimum MVP tables:

```text
projects
tenants
modules
module_instances
resource_definitions
resources
outbox_events
audit_logs
principals
memberships
roles
role_bindings
```

Later add:

```text
resource_events
service_accounts
secrets_metadata
external_identities
api_keys
controller_registrations
```

---

# Generic Resources Table

Use this as the dynamic/control-plane backbone:

```sql
create table control_plane.resources (
    id uuid primary key,

    project_id text not null,
    tenant_id text,

    api_group text not null,
    api_version text not null,
    kind text not null,
    namespace text,
    name text not null,

    spec jsonb not null default '{}',
    status jsonb not null default '{}',
    metadata jsonb not null default '{}',

    generation bigint not null default 1,
    observed_generation bigint,
    resource_version bigint not null default 1,

    deleted_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),

    unique (
      project_id,
      tenant_id,
      api_group,
      api_version,
      kind,
      namespace,
      name
    )
);
```

Use resources table for:

```text
configuration
integration setup
controller-managed desired state
dynamic module data
workflow state
low/medium-volume objects
```

Examples:

```text
WhatsAppChannel
VoiceAgent
ClinicModuleConfig
IntegrationConfig
AppointmentWorkflow
BillingPlan
EdgeNode
```

Use typed module tables for high-volume/query-heavy domain data:

```text
patients
appointments
messages
call_sessions
attendance_records
employees
invoices
usage_records
```

Rule:

```text
Configuration/stateful infrastructure → Resource
Core transactional product data       → typed tables
Dynamic/customer-defined data         → Resource first, projection later
```

---

# Write vs Read Rule

Writes should go through the control plane.

```text
BFF → platform client → control plane
```

Reads can use either:

```text
control plane API
or
read-optimized module projections/tables
```

Example:

```text
Create appointment:
  BFF → control plane

List calendar appointments:
  BFF → healthcare.appointments projection/table

Show module config:
  BFF → control_plane.resources/module_instances
```

Do not let BFF write directly to source-of-truth tables if that bypasses:

```text
admission validation
RBAC
audit
events
generation/resourceVersion
module lifecycle rules
```

---

# Auth Model

Each project can have its own auth provider, but the control plane should receive normalized identity.

Project auth examples:

```text
HealthCare OS → Keycloak
HR OS         → Firebase phone auth
Telry         → Zitadel/Keycloak/custom OIDC
```

Recommended BFF-to-control-plane model:

```text
BFF verifies user auth.
BFF calls control plane with project-scoped service account.
BFF includes signed user delegation context.
```

Do not trust arbitrary headers.

Use a signed delegation envelope:

```json
{
  "projectId": "healthcare-os",
  "tenantId": "clinic_123",
  "principalId": "usr_123",
  "roles": ["doctor"],
  "permissions": ["appointments:create"],
  "authProvider": "keycloak-healthcare",
  "expiresAt": "2026-06-26T10:05:00+03:00"
}
```

Control plane audit should record both:

```text
service account
delegated end user
```

---

# Project Abstraction

Even with one deployment per project, keep `Project` as a first-class concept.

Hierarchy:

```text
Platform
  └── Project / SaaS Product
        └── Tenant / Customer Organization
              └── Module
                    └── Resource
```

Example:

```text
healthcare-os
  └── clinic_123
        └── patients, appointments, ehr, ai-callcenter

hr-os
  └── company_456
        └── employees, attendance, leave
```

Every request context should include:

```go
type RequestContext struct {
    ProjectID   string
    TenantID    string
    PrincipalID string

    AuthProviderID string
    AuthSubject    string

    Roles       []string
    Permissions []string

    SecurityProfile string
    ModulesEnabled  []string
}
```

---

# Event Envelope

Every event should include project/module/tenant context.

```json
{
  "eventId": "evt_123",
  "projectId": "healthcare-os",
  "tenantId": "clinic_123",
  "moduleId": "appointments",
  "eventType": "appointment.scheduled",
  "resourceId": "res_123",
  "occurredAt": "2026-06-26T10:00:00+03:00",
  "data": {}
}
```

Use transactional outbox for reliability:

```text
DB transaction writes resource + outbox event.
Outbox publisher sends to NATS.
Controllers consume and reconcile.
```

---

# Controller Pattern

Controllers use the same platform client as BFF.

```go
type Reconciler interface {
    Reconcile(ctx context.Context, ref platform.ResourceRef) error
}
```

Example:

```go
func (r *AppointmentReconciler) Reconcile(ctx context.Context, ref platform.ResourceRef) error {
    appt, err := r.client.Resources().Get(ctx, ref)
    if err != nil {
        return err
    }

    // perform business logic:
    // - check doctor availability
    // - reserve slot
    // - send notification
    // - create EHR draft if needed

    return r.client.Resources().UpdateStatus(ctx, ref, map[string]any{
        "phase": "Scheduled",
        "conditions": []Condition{
            {Type: "DoctorAvailable", Status: "True"},
            {Type: "NotificationSent", Status: "True"},
        },
    })
}
```

---

# What To Build Next

Suggested milestone:

```text
Kernel vNext: Control-plane-ready SaaS Kernel
```

Scope:

```text
1. Add ProjectConfig.
2. Add normalized RequestContext.
3. Add AuthProvider / Delegation model.
4. Add ResourceDefinition table/API.
5. Add generic Resource API.
6. Add spec/status/conditions primitives.
7. Add generation/resourceVersion semantics.
8. Add ModuleDefinition + runtime module registry.
9. Add module enable/disable per tenant.
10. Add transactional outbox event envelope.
11. Add platform client SDK.
12. Add controller SDK/reconciler interface.
13. Add BFF example using the SDK.
14. Add one example dynamic declarative module.
15. Add one example controller-backed module.
```

---

# Final Decision

Use the current `edgescaleDev/kernel` as the base.

Do not build a full Kubernetes clone.

Evolve the kernel into:

```text
Per-project SaaS control plane
+ BFF
+ k8s-like platform client
+ dynamic module registry
+ optional external controllers
```

Core product modules can stay compiled.

Runtime extensibility should be added through:

```text
declarative modules
dynamic resource definitions
external controllers
```

Avoid runtime-loaded Go plugins inside the API server.
