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
// Note: advisory locks are session-scoped — acquire and release must happen on
// the same database connection. This provider pins a dedicated *sql.Conn for the
// lock's lifetime to guarantee that invariant.
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

	sqlDB, err := p.db.DB()
	if err != nil {
		return nil, false, fmt.Errorf("get sql db: %w", err)
	}

	// Use a dedicated connection so that acquire and release are guaranteed
	// to run on the same session (advisory locks are session-scoped).
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("get dedicated db connection: %w", err)
	}

	var acquired bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&acquired); err != nil {
		conn.Close()
		return nil, false, fmt.Errorf("pg advisory lock acquire: %w", err)
	}
	if !acquired {
		conn.Close()
		return nil, false, nil
	}

	release := func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", lockID)
		_ = conn.Close()
	}

	return release, true, nil
}

// Compile-time check that DBLockProvider implements sdk.LockProvider.
var _ sdk.LockProvider = (*DBLockProvider)(nil)
