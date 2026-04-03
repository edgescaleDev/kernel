package sdk

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// EventBus provides in-process, topic-based event publishing and subscription.
// Events are fire-and-forget from the publisher's perspective.
// For durable delivery, use the OutboxWriter which writes events transactionally.
type EventBus interface {
	// Publish sends an event to all subscribers of the given subject.
	Publish(ctx context.Context, subject string, payload any) error

	// Subscribe registers a handler for events matching the given subject.
	// The subscriberID is used for logging and debugging.
	Subscribe(subscriberID string, subject string, handler EventHandler) error
}

// EventHandler is a function that processes an incoming event.
type EventHandler func(ctx context.Context, envelope EventEnvelope) error

// EventEnvelope wraps an event payload with metadata for tracing,
// identity propagation, and debugging across async boundaries.
type EventEnvelope struct {
	// Subject is the event topic (e.g., "orders.created").
	Subject string `json:"subject"`

	// Payload is the serialized event data.
	Payload json.RawMessage `json:"payload"`

	// OrgID is the tenant that originated this event.
	OrgID uuid.UUID `json:"org_id,omitempty"`

	// UserID is the user who triggered the action that produced this event.
	UserID uuid.UUID `json:"user_id,omitempty"`

	// TraceID propagates distributed tracing across async boundaries.
	TraceID string `json:"trace_id,omitempty"`

	// SpanID propagates the parent span for distributed tracing.
	SpanID string `json:"span_id,omitempty"`

	// RequestID links this event back to the originating HTTP request.
	RequestID string `json:"request_id,omitempty"`

	// Timestamp is when the event was produced.
	Timestamp time.Time `json:"timestamp"`

	// ServiceID identifies which service produced this event.
	ServiceID string `json:"service_id"`
}
