package internal

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// ScopedDB returns a GORM DB instance whose queries target the given
// PostgreSQL schema by schema-qualifying every table name in the SQL.
//
// Previous implementation used SET LOCAL search_path callbacks, but that
// approach is broken for read queries: GORM only wraps writes (Create,
// Update, Delete) in implicit transactions, while reads (Find, First,
// Take) run without a transaction. SET LOCAL outside a transaction has
// no effect, and even session-level SET may execute on a different
// pooled connection than the subsequent SELECT.
//
// The NamingStrategy approach embeds the schema directly into table
// names (e.g. "module_shifts"."shifts"), making it connection-pool-safe
// and independent of search_path state.
//
// Each call creates a fully independent *gorm.DB that shares the
// underlying *sql.DB connection pool but has its own NamingStrategy.
func ScopedDB(db *gorm.DB, schemaName string) *gorm.DB {
	// Obtain the underlying *sql.DB (shared connection pool).
	sqlDB, err := db.DB()
	if err != nil {
		panic(fmt.Sprintf("kernel: scoped_db: cannot get underlying *sql.DB: %v", err))
	}

	// Create a fully independent GORM instance using the same connection
	// pool but with a NamingStrategy that prefixes all table names with
	// the module's schema (e.g. "module_shifts.").
	scoped, err := gorm.Open(
		postgres.New(postgres.Config{Conn: sqlDB}),
		&gorm.Config{
			Logger:                 db.Config.Logger,
			NowFunc:                db.Config.NowFunc,
			SkipDefaultTransaction: db.Config.SkipDefaultTransaction,
			NamingStrategy: schema.NamingStrategy{
				TablePrefix: schemaName + ".",
			},
		},
	)
	if err != nil {
		panic(fmt.Sprintf("kernel: scoped_db: cannot create independent DB for schema %q: %v", schemaName, err))
	}

	return scoped
}
