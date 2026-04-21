package sdk

import (
	"context"
	"sync"
	"time"
)

// CronDef declares a periodic job in the module's Manifest.
// This is pure metadata — used for discovery, admin UI, and schedule management.
// Handlers are wired separately via CronModule.RegisterCrons().
type CronDef struct {
	// ID uniquely identifies this cron within the module.
	// The kernel prefixes it with the module ID: "shifts.reminders".
	ID string `json:"id"`

	// Schedule is a cron expression. Three formats are supported:
	//   - Standard 5-field:  "* * * * *" (min hour dom month dow)
	//   - Extended 6-field:  "0 * * * * *" (sec min hour dom month dow)
	//   - @every shorthand:  "@every 15m", "@every 1h30m"
	Schedule string `json:"schedule"`

	// Timezone is the IANA timezone for schedule evaluation.
	// Examples: "America/New_York", "Asia/Baghdad", "UTC".
	// Defaults to UTC if empty.
	Timezone string `json:"timezone,omitempty"`

	// Description is a human-readable, translatable label for admin UI.
	Description TranslatableField `json:"description"`

	// Timeout is the maximum execution time before the job is killed.
	// Defaults to 5 minutes if not set.
	Timeout time.Duration `json:"timeout,omitempty"`

	// Retry defines how failures are handled.
	// Defaults to no retry if not set.
	Retry *CronRetryPolicy `json:"retry,omitempty"`
}

// CronRetryPolicy configures retry behavior for failed cron executions.
type CronRetryPolicy struct {
	// MaxAttempts is the maximum number of retry attempts. 0 = no retry.
	MaxAttempts int `json:"max_attempts"`

	// Backoff is the delay between retries.
	Backoff time.Duration `json:"backoff"`
}

// CronHandler is the function signature for cron job execution.
// Returning an error marks the execution as failed (triggers retry if configured).
// Context carries deadline from CronDef.Timeout and distributed trace ID.
type CronHandler func(ctx context.Context) error

// CronModule is an optional capability interface for modules that need
// periodic background work. The kernel's job runner manages scheduling,
// distributed locking, retries, and observability.
//
// Cron metadata (schedule, timezone, timeout) is declared in Manifest().Crons.
// Handlers are wired here. The kernel validates that every CronDef has a
// corresponding handler and vice versa — mismatches cause a startup panic.
type CronModule interface {
	RegisterCrons(registry *CronRegistry)
}

// CronRegistry collects handler functions during module lifecycle.
// Modules call Handle() to associate a handler with a CronDef.ID
// declared in their Manifest.
type CronRegistry struct {
	mu       sync.RWMutex
	handlers map[string]CronHandler
}

// NewCronRegistry creates a new empty CronRegistry.
func NewCronRegistry() *CronRegistry {
	return &CronRegistry{
		handlers: make(map[string]CronHandler),
	}
}

// Handle registers a handler for a cron declared in Manifest.Crons.
// The cronID must match a CronDef.ID from the module's Manifest.
func (r *CronRegistry) Handle(cronID string, handler CronHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[cronID] = handler
}

// Get returns the handler for the given cron ID.
// Returns nil and false if no handler is registered.
func (r *CronRegistry) Get(cronID string) (CronHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[cronID]
	return h, ok
}

// Handlers returns a copy of the handler map.
// Used by the kernel to build the scheduler.
func (r *CronRegistry) Handlers() map[string]CronHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	copied := make(map[string]CronHandler, len(r.handlers))
	for k, v := range r.handlers {
		copied[k] = v
	}
	return copied
}
