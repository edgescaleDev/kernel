package sdk

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TaskExecutor abstracts background task execution behind a pluggable interface.
// OS consumers choose their execution engine (Temporal, inline goroutines, Asynq, etc.).
//
// This interface covers three use cases:
//   - Import/Export: CSV parsing, row-by-row processing, progress tracking
//   - Scheduled Jobs: Recurring task scheduling (wraps Execute with cron)
//   - Long-Running Operations: Any async operation returned as 202 Accepted
type TaskExecutor interface {
	// Execute starts a background task and returns an operation ID for tracking.
	Execute(ctx context.Context, task TaskDefinition) (operationID string, err error)

	// Cancel attempts to cancel a running task.
	Cancel(ctx context.Context, operationID string) error
}

// TaskDefinition describes a background task to be executed.
type TaskDefinition struct {
	// ID is a unique identifier for this task instance.
	ID string

	// Name identifies the task type (e.g., "listing_import", "invoice_export").
	Name string

	// ServiceID identifies which service owns this task.
	ServiceID string

	// OrgID is the tenant context for this task.
	OrgID uuid.UUID

	// Handler is the function to execute. It receives a ProgressReporter
	// for reporting execution progress back to the kernel.
	Handler func(ctx context.Context, progress ProgressReporter) error

	// Retries is the number of times to retry on failure. 0 means no retry.
	Retries int

	// Timeout is the maximum duration for this task.
	Timeout time.Duration
}

// ProgressReporter allows tasks to report execution progress.
// Progress is stored and made available via the operations API.
type ProgressReporter interface {
	// Report updates the task progress.
	// percent should be 0-100, message is a human-readable status.
	Report(ctx context.Context, percent int, message string) error
}
