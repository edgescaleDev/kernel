package kernel

import (
	"github.com/edgescaleDev/kernel/internal"
	"gorm.io/gorm"
)

// scopedDB returns a GORM DB instance scoped to the given schema.
// Delegates to the internal implementation.
func scopedDB(db *gorm.DB, schema string) *gorm.DB {
	return internal.ScopedDB(db, schema)
}
