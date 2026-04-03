package sdk

import (
	"time"

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
//	    OrgID     uuid.UUID `json:"org_id" gorm:"type:uuid;not null;index"`
//	    Total     int64     `json:"total"`
//	}
type BaseModel struct {
	ID uint64 `json:"id" gorm:"primaryKey"`
	Timestamped
	SoftDeletable
}
