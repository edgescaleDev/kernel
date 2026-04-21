package kernel

import (
	"context"
	"fmt"
	"hash/crc32"
	"time"

	"go.edgescale.dev/kernel/sdk"
	"gorm.io/gorm"
)

// DBLockProvider implements sdk.LockProvider using PostgreSQL advisory locks.
// Use this when Redis is not available or for simpler single-database deployments.
// Note: advisory locks are session-scoped — the release function must be called
// from the same database connection that acquired the lock.
type DBLockProvider struct {
	db *gorm.DB
}

// NewDBLockProvider creates a PostgreSQL advisory lock provider.
func NewDBLockProvider(db *gorm.DB) *DBLockProvider {
	return &DBLockProvider{db: db}
}

// Acquire attempts to get a PostgreSQL advisory lock using pg_try_advisory_lock.
// The key is hashed to a 32-bit integer for the advisory lock ID.
// The ttl parameter is ignored — advisory locks are released explicitly
// or when the database session ends.
func (p *DBLockProvider) Acquire(ctx context.Context, key string, _ time.Duration) (func(), bool, error) {
	lockID := int64(crc32.ChecksumIEEE([]byte(key)))

	var acquired bool
	if err := p.db.WithContext(ctx).Raw("SELECT pg_try_advisory_lock(?)", lockID).Scan(&acquired).Error; err != nil {
		return nil, false, fmt.Errorf("pg advisory lock acquire: %w", err)
	}
	if !acquired {
		return nil, false, nil
	}

	release := func() {
		p.db.Exec("SELECT pg_advisory_unlock(?)", lockID)
	}

	return release, true, nil
}

// Compile-time check that DBLockProvider implements sdk.LockProvider.
var _ sdk.LockProvider = (*DBLockProvider)(nil)
