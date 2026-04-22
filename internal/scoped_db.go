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
		// Use the underlying ConnPool directly to bypass GORM's callback
		// pipeline. Using tx.Exec() here would re-enter the Raw callback,
		// causing infinite recursion and a stack overflow.
		if tx.Statement != nil && tx.Statement.ConnPool != nil {
			_, err := tx.Statement.ConnPool.ExecContext(tx.Statement.Context, setSQL)
			if err != nil {
				_ = tx.AddError(err)
			}
		}
	}

	_ = scoped.Callback().Create().Before("gorm:create").Register(callbackName+"_create", callback)
	_ = scoped.Callback().Query().Before("gorm:query").Register(callbackName+"_query", callback)
	_ = scoped.Callback().Update().Before("gorm:update").Register(callbackName+"_update", callback)
	_ = scoped.Callback().Delete().Before("gorm:delete").Register(callbackName+"_delete", callback)
	_ = scoped.Callback().Raw().Before("gorm:raw").Register(callbackName+"_raw", callback)

	return scoped
}
