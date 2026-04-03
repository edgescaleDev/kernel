package kernel

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.edgescale.dev/kernel/sdk"
	"gorm.io/gorm"
)

// auditLogger is the kernel's production implementation of sdk.AuditLogger.
// It writes hash-chained audit entries to the audit_events table.
type auditLogger struct {
	db       *gorm.DB
	moduleID string
}

// AuditEvent maps to the public.audit_events table.
type AuditEvent struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement"`
	Timestamp  time.Time `gorm:"column:timestamp;autoCreateTime"`
	UserID     *uuid.UUID `gorm:"column:user_id;type:uuid"`
	OrgID      *uuid.UUID `gorm:"column:org_id;type:uuid"`
	ModuleID   string    `gorm:"column:module_id;not null"`
	Action     string    `gorm:"column:action;not null"`
	Resource   string    `gorm:"column:resource;not null"`
	ResourceID string    `gorm:"column:resource_id"`
	Changes    JSON      `gorm:"column:changes;type:jsonb"`
	IPAddress  string    `gorm:"column:ip_address"`
	UserAgent  string    `gorm:"column:user_agent"`
	RequestID  string    `gorm:"column:request_id"`
	PrevHash   string    `gorm:"column:prev_hash"`
	Hash       string    `gorm:"column:hash;not null"`
}

func (AuditEvent) TableName() string {
	return "audit_events"
}

// JSON is a helper type for GORM JSONB columns.
type JSON json.RawMessage

func (j JSON) Value() (any, error) {
	if j == nil {
		return nil, nil
	}
	return json.RawMessage(j).MarshalJSON()
}

func (j *JSON) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("json.Scan: expected []byte, got %T", value)
	}
	*j = bytes
	return nil
}

// Log writes a hash-chained audit entry to the database.
func (a *auditLogger) Log(ctx context.Context, entry sdk.AuditEntry) error {
	// Serialize changes to JSON.
	var changesJSON JSON
	if entry.Changes != nil {
		data, err := json.Marshal(entry.Changes)
		if err != nil {
			return fmt.Errorf("marshal changes: %w", err)
		}
		changesJSON = JSON(data)
	}

	// Get the previous hash for chain integrity.
	var prevHash string
	var lastEvent AuditEvent
	if err := a.db.Order("id DESC").Limit(1).Find(&lastEvent).Error; err == nil && lastEvent.ID > 0 {
		prevHash = lastEvent.Hash
	}

	event := AuditEvent{
		UserID:     entry.UserID,
		OrgID:      entry.OrgID,
		ModuleID:   a.moduleID,
		Action:     string(entry.Action),
		Resource:   entry.Resource,
		ResourceID: entry.ResourceID,
		Changes:    changesJSON,
		PrevHash:   prevHash,
	}

	// Compute hash: SHA256 of (action + resource + resource_id + module_id + prev_hash).
	hashInput := fmt.Sprintf("%s|%s|%s|%s|%s",
		event.Action, event.Resource, event.ResourceID, event.ModuleID, prevHash)
	hash := sha256.Sum256([]byte(hashInput))
	event.Hash = fmt.Sprintf("%x", hash)

	return a.db.WithContext(ctx).Create(&event).Error
}

// newAuditLogger creates a kernel-owned AuditLogger for a module.
func newAuditLogger(db *gorm.DB, moduleID string) sdk.AuditLogger {
	if db == nil {
		return &sdk.TestAuditLogger{}
	}
	return &auditLogger{db: db, moduleID: moduleID}
}
