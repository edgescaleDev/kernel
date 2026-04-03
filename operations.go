package kernel

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Operation tracks long-running async operations (imports, exports, bulk updates).
// Modules create operations via the kernel API, then update progress.
type Operation struct {
	ID         uuid.UUID         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	ModuleID   string            `gorm:"column:module_id;not null;index"`
	OrgID      uuid.UUID         `gorm:"column:org_id;type:uuid;not null;index"`
	UserID     uuid.UUID         `gorm:"column:user_id;type:uuid;not null"`
	Type       string            `gorm:"column:type;not null"`
	Status     OperationStatus   `gorm:"column:status;not null;default:'pending'"`
	Progress   int               `gorm:"column:progress;default:0"`
	TotalItems int               `gorm:"column:total_items;default:0"`
	Result     *string           `gorm:"column:result;type:jsonb"`
	Error      *string           `gorm:"column:error"`
	CreatedAt  time.Time         `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt  time.Time         `gorm:"column:updated_at;autoUpdateTime"`
	CompletedAt *time.Time       `gorm:"column:completed_at"`
}

func (Operation) TableName() string {
	return "operations"
}

// OperationStatus represents the lifecycle state of an operation.
type OperationStatus string

const (
	OperationPending    OperationStatus = "pending"
	OperationRunning    OperationStatus = "running"
	OperationCompleted  OperationStatus = "completed"
	OperationFailed     OperationStatus = "failed"
	OperationCancelled  OperationStatus = "cancelled"
)

// CreateOperation starts a new tracked operation.
func (k *Kernel) CreateOperation(ctx context.Context, op Operation) (*Operation, error) {
	if op.ID == uuid.Nil {
		op.ID = uuid.New()
	}
	op.Status = OperationPending

	if err := k.db.WithContext(ctx).Create(&op).Error; err != nil {
		return nil, fmt.Errorf("create operation: %w", err)
	}
	return &op, nil
}

// UpdateOperationProgress updates the progress of a running operation.
func (k *Kernel) UpdateOperationProgress(ctx context.Context, operationID uuid.UUID, progress, total int) error {
	return k.db.WithContext(ctx).
		Model(&Operation{}).
		Where("id = ?", operationID).
		Updates(map[string]any{
			"status":      OperationRunning,
			"progress":    progress,
			"total_items": total,
		}).Error
}

// CompleteOperation marks an operation as completed with an optional result.
func (k *Kernel) CompleteOperation(ctx context.Context, operationID uuid.UUID, result *string) error {
	now := time.Now()
	return k.db.WithContext(ctx).
		Model(&Operation{}).
		Where("id = ?", operationID).
		Updates(map[string]any{
			"status":       OperationCompleted,
			"progress":     gorm.Expr("total_items"),
			"result":       result,
			"completed_at": &now,
		}).Error
}

// FailOperation marks an operation as failed with an error message.
func (k *Kernel) FailOperation(ctx context.Context, operationID uuid.UUID, errMsg string) error {
	now := time.Now()
	return k.db.WithContext(ctx).
		Model(&Operation{}).
		Where("id = ?", operationID).
		Updates(map[string]any{
			"status":       OperationFailed,
			"error":        errMsg,
			"completed_at": &now,
		}).Error
}

// GetOperation retrieves an operation by ID.
func (k *Kernel) GetOperation(ctx context.Context, operationID uuid.UUID) (*Operation, error) {
	var op Operation
	if err := k.db.WithContext(ctx).First(&op, "id = ?", operationID).Error; err != nil {
		return nil, fmt.Errorf("get operation: %w", err)
	}
	return &op, nil
}
