package bot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/stats"
)

type fakeUsageStore struct {
	stats.Store
	rows []stats.UsageCount
	err  error
}

func (f *fakeUsageStore) UpsertUsage(_ context.Context, rows []stats.UsageCount) error {
	f.rows = append(f.rows, rows...)
	return f.err
}

func usageNow() time.Time { return time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC) }

func TestPostCommandUsage(t *testing.T) {
	t.Parallel()
	store := &fakeUsageStore{}
	h := postCommandUsageHandler(store, usageNow, nil)
	body := `{"counts":[
		{"channel_twitch_id":"11","channel_login":"Foo","trigger":"!fish","day":"2026-10-31","uses":2},
		{"channel_twitch_id":"11","channel_login":"foo","trigger":"!FISH","day":"2026-10-31","uses":1},
		{"channel_twitch_id":"x","trigger":"!fish","day":"2026-10-31","uses":1}
	]}`
	rec := serve(h, http.MethodPost, "/api/bot/stats/usage", body)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"accepted":1`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(store.rows) != 1 || store.rows[0].Uses != 3 || store.rows[0].ChannelLogin != "foo" {
		t.Fatalf("rows = %+v", store.rows)
	}
}

func TestPostCommandUsageRejects(t *testing.T) {
	t.Parallel()
	h := postCommandUsageHandler(&fakeUsageStore{}, usageNow, nil)
	if rec := serve(h, http.MethodPost, "/", `{not json`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad json: %d", rec.Code)
	}

	var b strings.Builder
	b.WriteString(`{"counts":[`)
	for i := 0; i <= stats.MaxUsageBatch; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"channel_twitch_id":"%d","trigger":"!a","day":"2026-10-31","uses":1}`, i+1)
	}
	b.WriteString(`]}`)
	if rec := serve(h, http.MethodPost, "/", b.String()); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized batch: %d", rec.Code)
	}

	var logged error
	failing := postCommandUsageHandler(&fakeUsageStore{err: errors.New("db: secret detail")}, usageNow, func(err error) { logged = err })
	rec := serve(failing, http.MethodPost, "/", `{"counts":[{"channel_twitch_id":"1","trigger":"!a","day":"2026-10-31","uses":1}]}`)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret") || logged == nil {
		t.Errorf("store failure: %d %s logged=%v", rec.Code, rec.Body.String(), logged)
	}
}
