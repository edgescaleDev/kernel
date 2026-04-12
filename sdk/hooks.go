package sdk

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

// HookPoint identifies a specific interception point in a service's lifecycle.
// Hook points follow the convention: "{lifecycle}.{service}.{action}"
// Example: "before.orders.create", "after.orders.create"
type HookPoint string

// TenantProvisionedEvent is the payload for the "after.kernel.tenant.provisioned" hook.
type TenantProvisionedEvent struct {
	TenantID    uuid.UUID
	ActivatedBy uuid.UUID
}

// HookHandler is a function that intercepts a hook point.
// Returning an AbortError from a Before hook stops the operation.
// After hooks cannot abort - they run for side effects only.
type HookHandler func(ctx context.Context, payload any) error

// HookRegistry manages sync interceptors for service operations.
// Services register hooks during RegisterHooks(), and the kernel fires them
// at the appropriate lifecycle points.
type HookRegistry struct {
	mu     sync.RWMutex
	before map[HookPoint][]HookHandler
	after  map[HookPoint][]HookHandler
}

// NewHookRegistry creates a new empty HookRegistry.
func NewHookRegistry() *HookRegistry {
	return &HookRegistry{
		before: make(map[HookPoint][]HookHandler),
		after:  make(map[HookPoint][]HookHandler),
	}
}

// Before registers a handler to run before the specified hook point.
// If the handler returns an AbortError, the operation is cancelled.
func (r *HookRegistry) Before(point HookPoint, handler HookHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.before[point] = append(r.before[point], handler)
}

// After registers a handler to run after the specified hook point.
// After hooks cannot abort the operation.
func (r *HookRegistry) After(point HookPoint, handler HookHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.after[point] = append(r.after[point], handler)
}

// FireBefore executes all Before handlers for the given hook point.
// Returns an AbortError if any handler aborts, or the first non-abort error.
func (r *HookRegistry) FireBefore(ctx context.Context, point HookPoint, payload any) error {
	r.mu.RLock()
	handlers := r.before[point]
	r.mu.RUnlock()

	for _, h := range handlers {
		if err := h(ctx, payload); err != nil {
			return fmt.Errorf("before hook %s: %w", point, err)
		}
	}
	return nil
}

// FireAfter executes all After handlers for the given hook point.
// Errors are collected but do not abort the operation.
// Returns the first error encountered, if any.
func (r *HookRegistry) FireAfter(ctx context.Context, point HookPoint, payload any) error {
	r.mu.RLock()
	handlers := r.after[point]
	r.mu.RUnlock()

	var firstErr error
	for _, h := range handlers {
		if err := h(ctx, payload); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("after hook %s: %w", point, err)
		}
	}
	return firstErr
}

// BeforeHookPoint constructs a Before hook point for the given service and action.
func BeforeHookPoint(service, action string) HookPoint {
	return HookPoint(fmt.Sprintf("before.%s.%s", service, action))
}

// AfterHookPoint constructs an After hook point for the given service and action.
func AfterHookPoint(service, action string) HookPoint {
	return HookPoint(fmt.Sprintf("after.%s.%s", service, action))
}
