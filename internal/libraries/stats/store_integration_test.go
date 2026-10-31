//go:build integration

// Runs the stats SQL against a real Postgres with database/init.sql (and the
// prod-apply files) applied: the CI "db-migrations" job. Uses the same POSTGRES_*
// env vars and connection path as core-api.
//
//	go test -tags integration ./internal/libraries/stats/ -run Integration
package stats

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/conman"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

func TestIntegrationStatsQueries(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	opts := *nivek.GetStartupConnectionsForPostgres()[conman.DefaultConnection]
	mgr := conman.NewPostgresConnectionManager(logger)
	t.Cleanup(func() { _ = mgr.Close() })
	sess, err := mgr.NewConnection(conman.DefaultConnection, opts)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := sess.SQL().ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}

	// Unique ids/logins so the test is independent of other rows.
	tag := fmt.Sprint(time.Now().UnixNano() % 1_000_000_000)
	in, out, legacy := "91"+tag, "92"+tag, "93"+tag
	inLogin, outLogin := "it_in_"+tag, "it_out_"+tag
	exec(`INSERT INTO nivek.users (twitch_id, twitch_login, twitch_display_name, bot_opt_in) VALUES ($1, $2, $3, TRUE), ($4, $5, $6, FALSE)`,
		in, inLogin, "It_In_"+tag, out, outLogin, "It_Out_"+tag)
	t.Cleanup(func() {
		_, _ = sess.SQL().ExecContext(ctx, `DELETE FROM nivek.users WHERE twitch_id IN ($1, $2)`, in, out)
		_, _ = sess.SQL().ExecContext(ctx, `DELETE FROM nivek.command_usage WHERE channel_twitch_id IN ($1, $2, $3)`, in, out, legacy)
		_, _ = sess.SQL().ExecContext(ctx, `DELETE FROM nivek.fish_score WHERE chattername LIKE $1`, "it_fisher_"+tag+"%")
	})

	store := NewStore(sess)
	now := time.Now().UTC()
	today := now.Format(DayLayout)
	old := now.AddDate(-1, 0, -3).Format(DayLayout)

	// Upsert accumulates, incl. two flushes and a repeated key in a batch
	// (IngestUsage merges it).
	for range 2 {
		n, err := IngestUsage(ctx, store, []UsageCount{
			{ChannelTwitchID: in, ChannelLogin: inLogin, Trigger: "!fish", Day: today, Uses: 900_000},
			{ChannelTwitchID: in, ChannelLogin: inLogin, Trigger: "!fish", Day: today, Uses: 5},
			{ChannelTwitchID: out, ChannelLogin: outLogin, Trigger: "!fish", Day: today, Uses: 1_000_000},
		}, now)
		if err != nil || n != 2 {
			t.Fatalf("ingest n=%d err=%v", n, err)
		}
	}
	// Over a year old: still counted, top_channels is all time (issue #184).
	// Inserted directly, since Normalize rejects days that old.
	exec(`INSERT INTO nivek.command_usage (channel_twitch_id, channel_login, trigger, day, uses) VALUES ($1, $2, '!old', $3::date, 999999999)`, in, inLogin, old)
	// A home channel with no users row is still eligible (uses its stored login).
	exec(`INSERT INTO nivek.command_usage (channel_twitch_id, channel_login, trigger, day, uses) VALUES ($1, 'timallenfanclubofficial', '!it', $2::date, 1)`, legacy, today)

	var uses int
	row, err := sess.SQL().QueryRowContext(ctx, `SELECT uses FROM nivek.command_usage WHERE channel_twitch_id = $1 AND trigger = '!fish' AND day = $2::date`, in, today)
	if err != nil {
		t.Fatal(err)
	}
	if err := row.Scan(&uses); err != nil || uses != 1_800_010 {
		t.Fatalf("uses = %d err=%v, want 1800010", uses, err)
	}

	// Overflow clamps instead of failing the batch.
	exec(`UPDATE nivek.command_usage SET uses = 2147483000 WHERE channel_twitch_id = $1 AND trigger = '!fish'`, in)
	if _, err := IngestUsage(ctx, store, []UsageCount{{ChannelTwitchID: in, ChannelLogin: inLogin, Trigger: "!fish", Day: today, Uses: 1_000_000}}, now); err != nil {
		t.Fatalf("overflow upsert: %v", err)
	}

	channels, err := store.TopChannels(ctx, 1000)
	if err != nil {
		t.Fatal(err)
	}
	var sawIn, sawHome bool
	for _, c := range channels {
		switch c.DisplayName {
		case "It_In_" + tag:
			sawIn = true
			if c.Uses != 2147483647+999999999 {
				t.Errorf("opted-in uses = %d (all-time sum must include the old row)", c.Uses)
			}
		case "It_Out_" + tag, outLogin:
			t.Errorf("opted-out channel listed: %+v", c)
		case "timallenfanclubofficial", "Timallenfanclubofficial":
			sawHome = true
		}
	}
	if !sawIn || !sawHome {
		t.Errorf("top channels missing rows (in=%v home=%v): %+v", sawIn, sawHome, channels)
	}
	if lim, err := store.TopChannels(ctx, 1); err != nil || len(lim) > 1 {
		t.Errorf("limit not applied: %v %v", lim, err)
	}

	before, err := store.ActiveChats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE nivek.users SET bot_opt_in = TRUE WHERE twitch_id = $1`, out)
	after, err := store.ActiveChats(ctx)
	if err != nil || after != before+1 {
		t.Errorf("active chats %d -> %d (err %v), want +1", before, after, err)
	}
	if before < len(HomeChannelLogins) {
		t.Errorf("active chats %d < home channels", before)
	}

	exec(`INSERT INTO nivek.fish_score (channelname, chattername, score, fish) VALUES
		('c1', $1, 2000000000, '[{"value":1,"name":"a","scarcity":1},{"value":1,"name":"b","scarcity":1}]'),
		('c2', $1, 2000000000, '[{"value":1,"name":"c","scarcity":1}]'),
		('c1', $2, 0, '[]')`, "it_fisher_"+tag+"_a", "it_fisher_"+tag+"_zero")
	fishers, err := store.TopFishers(ctx, TopFishersLimit)
	if err != nil {
		t.Fatal(err)
	}
	if len(fishers) == 0 || len(fishers) > TopFishersLimit {
		t.Fatalf("fishers = %+v", fishers)
	}
	top := fishers[0]
	if top.DisplayName != "it_fisher_"+tag+"_a" || top.Score != 4_000_000_000 || top.Catches != 3 {
		t.Errorf("top fisher = %+v (sum across channels must not overflow)", top)
	}
	for _, f := range fishers {
		if f.Score <= 0 {
			t.Errorf("zero-score fisher listed: %+v", f)
		}
	}

	// Full cached path.
	got, err := NewPublicService(store, nil, nil).Get(ctx)
	if err != nil || got.ActiveChats != after || len(got.TopChannels) > TopChannelsLimit {
		t.Fatalf("public stats = %+v err=%v", got, err)
	}
}
