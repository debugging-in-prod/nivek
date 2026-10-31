//go:build integration

// Integration test for incident #172: an additive schema change applied while
// core-api is running breaks queries on already-open pooled connections (pgx
// "bind message has N result formats but query has M columns", SQLSTATE 08P01)
// because upper/db + pgx cache prepared statements per connection and the
// services select whole rows via struct mapping.
//
// Until queries are made schema-change tolerant (tracked separately on #172),
// the mitigation is operational: restart core-api after every prod-apply SQL
// (docs/deploy-runbook.md). This test pins both halves of that:
//   - on the stale pool, the only acceptable failure is 08P01 (logged, so the
//     job output shows whether the hazard still exists); anything else fails;
//   - after the pool is restarted, the same query must succeed with no 08P01.
//
// Runs only in the CI "db-migrations" job, against a throwaway Postgres that
// has database/init.sql applied. Connection settings come from the same
// POSTGRES_* env vars core-api reads, through the same code path
// (nivek.GetStartupConnectionsForPostgres -> conman.NewConnection).
//
//	go test -tags integration ./internal/libraries/commands/ -run Integration
package commands

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/conman"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/upper/db/v4"
)

func is08P01(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 08P01")
}

func TestIntegrationAddColumnWhilePoolOpen(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	opts := *nivek.GetStartupConnectionsForPostgres()[conman.DefaultConnection]
	// A single connection guarantees the post-ALTER query lands on the very
	// connection that prepared (and cached) the statement before the ALTER,
	// which is the exact condition that produced the intermittent 08P01.
	opts.MaxConnections = 1
	opts.MaxIdleConnections = 1

	mgr := conman.NewPostgresConnectionManager(logger)
	t.Cleanup(func() { _ = mgr.Close() })

	open := func(name string) db.Session {
		t.Helper()
		sess, err := mgr.NewConnection(name, opts)
		if err != nil {
			t.Fatalf("open %s connection: %v", name, err)
		}
		return sess
	}

	// A separate session plays the operator running prod-apply SQL from psql
	// while core-api keeps its pool open.
	ddl := open("ddl")
	svc := &commandsImpl{commandsTable: open(conman.DefaultConnection).Collection(TableCommands)}

	// Prime the pooled connection's statement cache, as a long-running
	// core-api would have done before anyone touched the schema.
	for i := 0; i < 3; i++ {
		if _, err := svc.GetGlobalEnabledCommands(); err != nil {
			t.Fatalf("query before ALTER (attempt %d): %v", i+1, err)
		}
	}

	col := fmt.Sprintf("it_added_%d", time.Now().UnixNano())
	if _, err := ddl.SQL().Exec(fmt.Sprintf(
		"ALTER TABLE nivek.%s ADD COLUMN IF NOT EXISTS %s INTEGER NOT NULL DEFAULT 0", TableCommands, col,
	)); err != nil {
		t.Fatalf("ALTER TABLE add column: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ddl.SQL().Exec(fmt.Sprintf("ALTER TABLE nivek.%s DROP COLUMN IF EXISTS %s", TableCommands, col))
	})

	// Stale pool: 08P01 is the known hazard; any other error is a regression.
	_, err := svc.GetGlobalEnabledCommands()
	switch {
	case is08P01(err):
		t.Logf("stale pool after ALTER hit 08P01 (known #172 hazard; restart core-api after schema changes): %v", err)
	case err != nil:
		t.Fatalf("query on stale pool after ALTER: %v", err)
	default:
		t.Logf("stale pool after ALTER succeeded: queries are schema-change tolerant now; the runbook restart may be revisitable")
	}

	// Runbook mitigation: drop the pool and reconnect, which is what
	// "docker compose restart core-api" does.
	if err := mgr.CloseConnection(conman.DefaultConnection); err != nil {
		t.Fatalf("close app pool: %v", err)
	}
	svc = &commandsImpl{commandsTable: open(conman.DefaultConnection).Collection(TableCommands)}

	for i := 0; i < 3; i++ {
		_, err := svc.GetGlobalEnabledCommands()
		if is08P01(err) {
			t.Fatalf("query after pool restart (attempt %d) still hit 08P01: %v", i+1, err)
		}
		if err != nil {
			t.Fatalf("query after pool restart (attempt %d): %v", i+1, err)
		}
	}
}
