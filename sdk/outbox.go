package sdk

import (
	"context"
	"encoding/json"
	"time"
)

// OutboxWriter provides durable event publishing within the same database transaction.
// Events written via the outbox are guaranteed to be delivered — they are stored
// in the same transaction as the business data, then polled and dispatched by the kernel.
type OutboxWriter interface {
	// WriteEvent writes an event to the transactional outbox within the current transaction.
	// The event will be polled and dispatched to the EventBus by the kernel's outbox poller.
	WriteEvent(ctx context.Context, subject string, payload any) error
}

// OutboxEvent represents a single event stored in the transactional outbox.
// This is the database representation used by the outbox poller.
type OutboxEvent struct {
	// ID is the unique identifier for this outbox entry.
	ID uint64 `gorm:"primaryKey;autoIncrement"`

	// Subject is the event topic (e.g., "orders.created").
	Subject string `gorm:"not null"`

	// Payload is the serialized event data.
	Payload json.RawMessage `gorm:"type:jsonb;not null"`

	// ServiceID identifies which service produced this event.
	ServiceID string `gorm:"not null"`

	// OrgID is the tenant context (nullable for system events).
	OrgID *string `gorm:"type:uuid"`

	// Status tracks delivery state: "pending", "delivered", "failed".
	Status string `gorm:"not null;default:'pending'"`

	// Attempts tracks how many times delivery has been attempted.
	Attempts int `gorm:"not null;default:0"`

	// CreatedAt is when the event was written to the outbox.
	CreatedAt time.Time `gorm:"autoCreateTime;not null"`
}

// TableName returns the table name for GORM.
func (OutboxEvent) TableName() string {
	return "public.event_outbox"
}
