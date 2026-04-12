package sdk

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// OperationTracker manages long-running async operations (imports, exports, bulk updates).
// Modules use this to create operations, update progress, and mark them as complete.
type OperationTracker interface {
	// Create starts a new tracked operation and returns its ID.
	Create(ctx context.Context, op OperationInput) (uuid.UUID, error)

	// UpdateProgress updates the progress of a running operation.
	UpdateProgress(ctx context.Context, id uuid.UUID, progress, total int) error

	// Complete marks an operation as completed with an optional result.
	Complete(ctx context.Context, id uuid.UUID, result *string) error

	// Fail marks an operation as failed with an error message.
	Fail(ctx context.Context, id uuid.UUID, errMsg string) error

	// Get retrieves an operation by ID.
	Get(ctx context.Context, id uuid.UUID) (*OperationInfo, error)
}

// OperationInput defines the parameters for creating a new operation.
type OperationInput struct {
	ModuleID string
	TenantID uuid.UUID
	UserID   uuid.UUID
	Type     string // e.g., "import", "export", "bulk_update"
}

// OperationInfo represents the state of a tracked operation.
type OperationInfo struct {
	ID          uuid.UUID
	Status      string // "pending", "running", "completed", "failed", "cancelled"
	Progress    int
	TotalItems  int
	Result      *string
	Error       *string
	CreatedAt   time.Time
	CompletedAt *time.Time
}
