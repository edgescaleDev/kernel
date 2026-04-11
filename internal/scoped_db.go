package internal

import (
	"fmt"

	"gorm.io/gorm"
)

// ScopedDB returns a GORM DB instance that sets the search_path to the given
// schema at the start of every SQL operation. Unlike a one-shot SET, this
// approach is pool-safe: each query/transaction gets its own SET LOCAL,
// which automatically reverts when the connection returns to the pool.
func ScopedDB(db *gorm.DB, schema string) *gorm.DB {
	setSQL := fmt.Sprintf("SET LOCAL search_path TO %s, public", schema)
	callbackName := fmt.Sprintf("kernel:set_search_path_%s", schema)

	scoped := db.Session(&gorm.Session{NewDB: true})

	callback := func(tx *gorm.DB) {
		// SET LOCAL is transaction-scoped; it reverts on COMMIT/ROLLBACK,
		// so the pooled connection is never left with a stale search_path.
		tx.Exec(setSQL)
	}

	_ = scoped.Callback().Create().Before("gorm:create").Register(callbackName+"_create", callback)
	_ = scoped.Callback().Query().Before("gorm:query").Register(callbackName+"_query", callback)
	_ = scoped.Callback().Update().Before("gorm:update").Register(callbackName+"_update", callback)
	_ = scoped.Callback().Delete().Before("gorm:delete").Register(callbackName+"_delete", callback)
	_ = scoped.Callback().Raw().Before("gorm:raw").Register(callbackName+"_raw", callback)

	return scoped
}
