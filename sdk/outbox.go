package sdk

import (
	"context"
)

// OutboxWriter provides durable event publishing within the same database transaction.
// Events written via the outbox are guaranteed to be delivered - they are stored
// in the same transaction as the business data, then polled and dispatched by the
// outbox module's poller.
type OutboxWriter interface {
	// WriteEvent writes an event to the transactional outbox within the current transaction.
	// The event will be polled and dispatched to the EventBus by the outbox module's poller.
	WriteEvent(ctx context.Context, subject string, payload any) error
}
