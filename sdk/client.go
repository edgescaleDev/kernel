package sdk

// Client returns a type-safe cross-module client for the given module.
// Unlike Reader (which implies read-only), Client is the full operational
// interface a module exposes: queries, commands, and lifecycle operations.
//
// Client and Reader share the same underlying registry. A module that calls
// RegisterClient is discoverable via both Client[T] and Reader[T]. The
// distinction is semantic: use Client when you need the full API contract
// (reads + writes), use Reader when you only need read access.
//
// Usage:
//
//	apptClient, err := sdk.Client[appointments.AppointmentsClient](&m.ctx, "appointments")
//	if err != nil { /* module not registered */ }
//	appt, err := apptClient.Book(ctx, tenantID, input)
//
// Rules:
//   - Resolve lazily in handlers, never in Init().
//   - Client interfaces are defined and owned by the providing module.
//   - Both Reader and Client resolve from the same registry.
func Client[T any](ctx *Context, moduleID string) (T, error) {
	return GetReader[T](ctx.readers, moduleID)
}

// RegisterClient stores a client implementation for cross-module access.
// This is the preferred method over RegisterReader for modules that expose
// both read and write operations.
//
// The client is stored in the shared reader registry, so it is also
// discoverable via sdk.Reader[T] for backward compatibility.
func (ctx *Context) RegisterClient(client any) {
	ctx.readers.Register(ctx.ServiceID, client)
}
