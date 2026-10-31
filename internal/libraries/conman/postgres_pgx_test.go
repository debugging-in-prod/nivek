package conman

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/upper/db/v4"
	"github.com/upper/db/v4/adapter/postgresql"
)

func TestPostgresPgxConfig_UsesSchemaSafeExecMode(t *testing.T) {
	connURL := postgresql.ConnectionURL{
		User:     "nivek",
		Password: "s3cret",
		Database: "nivek",
		Host:     "db.example:6543",
		Options: map[string]string{
			"sslmode":          "disable",
			"application_name": "core-api",
		},
	}

	// Sanity check the reason this file exists: upper/db forces
	// cache_describe into the DSN, which is the mode that broke in #172.
	if !strings.Contains(connURL.String(), "default_query_exec_mode=cache_describe") {
		t.Fatalf("expected upper/db DSN to force cache_describe; got %q", connURL.String())
	}

	cfg, err := postgresPgxConfig(connURL)
	if err != nil {
		t.Fatalf("postgresPgxConfig: %v", err)
	}

	if cfg.DefaultQueryExecMode != pgx.QueryExecModeDescribeExec {
		t.Errorf("DefaultQueryExecMode = %v, want describe_exec", cfg.DefaultQueryExecMode)
	}
	if cfg.StatementCacheCapacity != 0 || cfg.DescriptionCacheCapacity != 0 {
		t.Errorf("caches should be disabled; statement=%d description=%d",
			cfg.StatementCacheCapacity, cfg.DescriptionCacheCapacity)
	}

	// Connection parameters must still come through untouched.
	if cfg.Host != "db.example" || cfg.Port != 6543 {
		t.Errorf("host/port = %s:%d", cfg.Host, cfg.Port)
	}
	if cfg.User != "nivek" || cfg.Password != "s3cret" || cfg.Database != "nivek" {
		t.Errorf("user/password/db = %q/%q/%q", cfg.User, cfg.Password, cfg.Database)
	}
	if cfg.TLSConfig != nil {
		t.Errorf("sslmode=disable should yield no TLS config")
	}
	if got := cfg.RuntimeParams["application_name"]; got != "core-api" {
		t.Errorf("application_name = %q", got)
	}
	if _, ok := cfg.RuntimeParams["default_query_exec_mode"]; ok {
		t.Errorf("default_query_exec_mode must not be sent to the server as a runtime param")
	}
}

func TestPostgresPgxConfig_InvalidDSN(t *testing.T) {
	connURL := postgresql.ConnectionURL{
		Host:    "localhost:5432",
		Options: map[string]string{"sslmode": "bogus"},
	}
	if _, err := postgresPgxConfig(connURL); err == nil {
		t.Fatal("expected error for invalid sslmode")
	}
}

func TestPostgresPoolLifetimeDefaults(t *testing.T) {
	if postgresConnMaxLifetime <= 0 || postgresConnMaxLifetime > time.Hour {
		t.Errorf("postgresConnMaxLifetime = %v; want (0, 1h]", postgresConnMaxLifetime)
	}
	if postgresConnMaxIdleTime <= 0 || postgresConnMaxIdleTime > postgresConnMaxLifetime {
		t.Errorf("postgresConnMaxIdleTime = %v", postgresConnMaxIdleTime)
	}
}

// TestOpenPostgresSession_SurvivesAddColumn reproduces incident #172 against a
// real Postgres. Opt-in: set NIVEK_TEST_POSTGRES_URL, e.g.
// postgres://nivek:nivek@localhost:55432/nivek?sslmode=disable
func TestOpenPostgresSession_SurvivesAddColumn(t *testing.T) {
	rawURL := os.Getenv("NIVEK_TEST_POSTGRES_URL")
	if rawURL == "" {
		t.Skip("NIVEK_TEST_POSTGRES_URL not set; skipping Postgres integration test")
	}
	connURL, err := postgresql.ParseURL(rawURL)
	if err != nil {
		t.Fatalf("parse NIVEK_TEST_POSTGRES_URL: %v", err)
	}

	run := func(t *testing.T, session db.Session) error {
		t.Helper()
		sqlDB := session.Driver().(*sql.DB)
		// One connection so the warm query and the post-ALTER query are
		// guaranteed to share per-connection caches.
		sqlDB.SetMaxOpenConns(1)

		table := fmt.Sprintf("conman_stmt_cache_test_%d", time.Now().UnixNano())

		if _, err := session.SQL().Exec(fmt.Sprintf(`CREATE TABLE %s (id SERIAL PRIMARY KEY, name TEXT NOT NULL)`, table)); err != nil {
			t.Fatalf("create table: %v", err)
		}
		defer func() { _, _ = session.SQL().Exec(fmt.Sprintf(`DROP TABLE IF EXISTS %s`, table)) }()

		if _, err := session.Collection(table).Insert(map[string]any{"name": "a"}); err != nil {
			t.Fatalf("insert: %v", err)
		}

		var before []map[string]any
		if err := session.Collection(table).Find(db.Cond{"id >": 0}).All(&before); err != nil {
			t.Fatalf("warm select: %v", err)
		}
		if len(before) != 1 || len(before[0]) != 2 {
			t.Fatalf("warm select rows = %v", before)
		}

		if _, err := session.SQL().Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS extra TEXT NOT NULL DEFAULT 'x'`, table)); err != nil {
			t.Fatalf("alter: %v", err)
		}

		var after []map[string]any
		if err := session.Collection(table).Find(db.Cond{"id >": 0}).All(&after); err != nil {
			return err
		}
		if len(after) != 1 || len(after[0]) != 3 {
			t.Fatalf("post-alter select rows = %v; want 3 columns", after)
		}
		return nil
	}

	t.Run("describe_exec survives ADD COLUMN", func(t *testing.T) {
		session, err := openPostgresSession(*connURL)
		if err != nil {
			t.Skipf("postgres not reachable: %v", err)
		}
		defer session.Close()
		if err := run(t, session); err != nil {
			t.Fatalf("post-alter select failed: %v", err)
		}
	})

	t.Run("upper default cache_describe reproduces #172", func(t *testing.T) {
		session, err := postgresql.Open(connURL)
		if err != nil {
			t.Skipf("postgres not reachable: %v", err)
		}
		defer session.Close()
		err = run(t, session)
		if err == nil {
			t.Skip("upper/db default mode did not reproduce the stale-description error (driver behavior changed?)")
		}
		if !strings.Contains(err.Error(), "08P01") {
			t.Fatalf("unexpected error from control: %v", err)
		}
	})
}
