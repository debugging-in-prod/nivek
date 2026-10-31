package conman

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/upper/db/v4"
	"github.com/upper/db/v4/adapter/postgresql"
)

// Pool lifetime defaults applied to every upper/db Postgres session. They are
// defense in depth on top of the describe_exec query mode: even if some future
// code path ends up caching per-connection state, no connection lives longer
// than postgresConnMaxLifetime, so a stale cache heals itself.
const (
	postgresConnMaxLifetime = 30 * time.Minute
	postgresConnMaxIdleTime = 5 * time.Minute
)

// postgresQueryExecMode is the pgx query execution mode used for every
// upper/db session.
//
// upper/db's ConnectionURL.String() unconditionally forces
// default_query_exec_mode=cache_describe. That mode caches each query's
// result-column description per pooled connection and assumes it never
// changes, so after an `ALTER TABLE ... ADD COLUMN` every `SELECT *` on an
// already-warm connection fails with
// "bind message has N result formats but query has M columns (SQLSTATE 08P01)"
// (incident #172). describe_exec asks the server for the statement description
// on every execution (unnamed statement, no cache), which pgx documents as
// "safe even when the database schema is modified concurrently". The cost is
// one extra round trip per parameterized query; results are still decoded in
// the binary format exactly as before, so scanned values do not change.
const postgresQueryExecMode = pgx.QueryExecModeDescribeExec

// postgresPgxConfig turns upper/db connection options into a pgx ConnConfig
// with a schema-change-safe query execution mode.
func postgresPgxConfig(connURL postgresql.ConnectionURL) (*pgx.ConnConfig, error) {
	cfg, err := pgx.ParseConfig(connURL.String())
	if err != nil {
		return nil, fmt.Errorf("parse postgres connection config: %w", err)
	}
	cfg.DefaultQueryExecMode = postgresQueryExecMode
	// No statement/description caches are used in describe_exec mode; make
	// that explicit so nothing stale can be retained per connection.
	cfg.StatementCacheCapacity = 0
	cfg.DescriptionCacheCapacity = 0
	return cfg, nil
}

// openPostgresSession opens a *sql.DB through pgx's stdlib driver with the
// safe exec mode and wraps it in an upper/db session. upper/db's own
// prepared-statement cache stays at its default (disabled).
func openPostgresSession(connURL postgresql.ConnectionURL) (db.Session, error) {
	cfg, err := postgresPgxConfig(connURL)
	if err != nil {
		return nil, err
	}

	sqlDB := stdlib.OpenDB(*cfg)
	sqlDB.SetConnMaxLifetime(postgresConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(postgresConnMaxIdleTime)

	// postgresql.New pings the database, matching postgresql.Open's behavior.
	session, err := postgresql.New(sqlDB)
	if err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return session, nil
}
