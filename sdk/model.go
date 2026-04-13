package sdk

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SoftDeletable provides soft delete fields for GORM models.
// Embed this in any model that should support soft deletion.
//
// Usage:
//
//	type Order struct {
//	    ID uint64
//	    sdk.SoftDeletable
//	}
type SoftDeletable struct {
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

// IsDeleted returns true if the record has been soft-deleted.
func (s SoftDeletable) IsDeleted() bool {
	return s.DeletedAt.Valid
}

// DataErasable is implemented by models that need GDPR-compliant data erasure.
// When an org is deprovisioned or a user requests data deletion, the kernel
// calls ErasePersonalData on all models that implement this interface.
type DataErasable interface {
	// ErasePersonalData anonymizes or removes personal data from this record.
	// Called during GDPR data deletion requests.
	ErasePersonalData() error
}

// Timestamped provides created_at and updated_at fields for GORM models.
// Embed this in any model that should track creation and modification times.
//
// Usage:
//
//	type Order struct {
//	    ID uint64
//	    sdk.Timestamped
//	}
type Timestamped struct {
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// BaseModel combines the most common model fields: ID, timestamps, and soft delete.
//
// Usage:
//
//	type Order struct {
//	    sdk.BaseModel
//	    TenantID  uuid.UUID `json:"tenant_id" gorm:"type:uuid;not null;index"`
//	    Total     int64     `json:"total"`
//	}
type BaseModel struct {
	ID uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Timestamped
	SoftDeletable
}

// JSONB is a cross-database compatible JSON column type.
// Unlike json.RawMessage, it implements sql.Scanner to handle both
// []byte (PostgreSQL) and string (SQLite) driver values.
type JSONB json.RawMessage

// Value implements driver.Valuer — returns raw JSON bytes.
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return []byte(j), nil
}

// Scan implements sql.Scanner — handles both []byte and string.
func (j *JSONB) Scan(src any) error {
	if src == nil {
		*j = nil
		return nil
	}
	switch v := src.(type) {
	case []byte:
		cp := make(JSONB, len(v))
		copy(cp, v)
		*j = cp
	case string:
		*j = JSONB(v)
	default:
		return fmt.Errorf("iam: unsupported JSONB scan type %T", src)
	}
	return nil
}

// MarshalJSON passes through raw bytes.
func (j JSONB) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("null"), nil
	}
	return json.RawMessage(j).MarshalJSON()
}

// UnmarshalJSON stores raw bytes.
func (j *JSONB) UnmarshalJSON(data []byte) error {
	rm := json.RawMessage(data)
	*j = JSONB(rm)
	return nil
}
