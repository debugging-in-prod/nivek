package stats

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

// fakeResolver records each DisplayNames call. names maps login -> display
// name; err (optional) is returned alongside whatever names matched; block
// makes it wait for ctx to end.
type fakeResolver struct {
	mu    sync.Mutex
	names map[string]string
	err   error
	block bool
	calls [][]string
}

func (f *fakeResolver) DisplayNames(ctx context.Context, logins []string) (map[string]string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, slices.Clone(logins))
	block, err := f.block, f.err
	out := map[string]string{}
	for _, l := range logins {
		if n, ok := f.names[l]; ok {
			out[l] = n
		}
	}
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return out, err
}

func (f *fakeResolver) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func TestNameCacheResolvesAndCaches(t *testing.T) {
	t.Parallel()
	r := &fakeResolver{names: map[string]string{"bob": "Bob", "kim": "キム"}}
	now := fixedNow
	c := newNameCache(r, func() time.Time { return now })

	logins := []string{"bob", "kim", "ghost", "bob", "Not A Login!", ""}
	got, err := c.resolve(context.Background(), logins)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"bob": "Bob", "kim": "キム", "ghost": "ghost", "Not A Login!": "Not A Login!", "": ""}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// One upstream call, deduped, with invalid logins never sent.
	if len(r.calls) != 1 || !slices.Equal(r.calls[0], []string{"bob", "kim", "ghost"}) {
		t.Fatalf("calls = %v", r.calls)
	}

	// Within the miss TTL everything (hits and the miss) comes from cache.
	now = now.Add(nameMissTTL - time.Second)
	if got, _ := c.resolve(context.Background(), logins); got["bob"] != "Bob" || got["ghost"] != "ghost" || r.callCount() != 1 {
		t.Fatalf("cached resolve = %v, calls = %d", got, r.callCount())
	}

	// After the miss TTL only the miss is retried; hits last nameCacheTTL.
	now = now.Add(time.Second)
	r.mu.Lock()
	r.names["ghost"] = "Ghost"
	r.mu.Unlock()
	if got, _ := c.resolve(context.Background(), logins); got["ghost"] != "Ghost" || got["bob"] != "Bob" {
		t.Fatalf("after miss TTL = %v", got)
	}
	if len(r.calls) != 2 || !slices.Equal(r.calls[1], []string{"ghost"}) {
		t.Fatalf("calls = %v", r.calls)
	}

	// After the hit TTL, names are looked up again.
	now = fixedNow.Add(nameCacheTTL)
	_, _ = c.resolve(context.Background(), []string{"bob"})
	if len(r.calls) != 3 || !slices.Equal(r.calls[2], []string{"bob"}) {
		t.Fatalf("calls = %v", r.calls)
	}
}

func TestNameCacheFallsBackOnError(t *testing.T) {
	t.Parallel()
	// Partial success: "bob" resolved, the rest of the lookup failed.
	r := &fakeResolver{names: map[string]string{"bob": "Bob"}, err: errors.New("helix 503")}
	c := newNameCache(r, func() time.Time { return fixedNow })

	got, err := c.resolve(context.Background(), []string{"bob", "amy"})
	if err == nil {
		t.Fatal("want the upstream error reported")
	}
	if got["bob"] != "Bob" || got["amy"] != "amy" {
		t.Fatalf("got %v", got)
	}
	// The failure isn't cached as a miss: "amy" is retried next time, "bob"
	// (a real answer) is not.
	r.mu.Lock()
	r.err = nil
	r.names["amy"] = "Amy"
	r.mu.Unlock()
	got, err = c.resolve(context.Background(), []string{"bob", "amy"})
	if err != nil || got["amy"] != "Amy" || got["bob"] != "Bob" {
		t.Fatalf("retry = %v, %v", got, err)
	}
	if len(r.calls) != 2 || !slices.Equal(r.calls[1], []string{"amy"}) {
		t.Fatalf("calls = %v", r.calls)
	}
}

func TestNameCacheRejectsBadUpstreamNames(t *testing.T) {
	t.Parallel()
	long := string(make([]byte, maxDisplayNameLen+1))
	r := &fakeResolver{names: map[string]string{"blank": "  ", "long": long}}
	c := newNameCache(r, func() time.Time { return fixedNow })
	got, err := c.resolve(context.Background(), []string{"blank", "long"})
	if err != nil || got["blank"] != "blank" || got["long"] != "long" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestNameCacheTimeout(t *testing.T) {
	t.Parallel()
	r := &fakeResolver{block: true}
	c := newNameCache(r, func() time.Time { return fixedNow })
	c.timeout = 50 * time.Millisecond

	// A cancelled caller doesn't cut the lookup short (it's shared), but the
	// lookup's own timeout always ends it.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	got, err := c.resolve(ctx, []string{"bob"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("resolve took %s", elapsed)
	}
	if got["bob"] != "bob" {
		t.Fatalf("got %v", got)
	}
}

func TestNameCacheBounded(t *testing.T) {
	t.Parallel()
	r := &fakeResolver{names: map[string]string{}}
	now := fixedNow
	c := newNameCache(r, func() time.Time { return now })
	for i := range 3 * nameCacheMax {
		l := fmt.Sprintf("user%d", i)
		r.names[l] = "U"
		if i%TopFishersLimit == 0 {
			now = now.Add(time.Minute)
		}
		_, _ = c.resolve(context.Background(), []string{l})
		if n := len(c.entries); n > nameCacheMax {
			t.Fatalf("cache grew to %d entries", n)
		}
	}
}

func TestNameCacheNilResolver(t *testing.T) {
	t.Parallel()
	c := newNameCache(nil, func() time.Time { return fixedNow })
	got, err := c.resolve(context.Background(), []string{"bob"})
	if err != nil || got["bob"] != "bob" {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestPublicServiceResolvesFisherNames(t *testing.T) {
	t.Parallel()
	store := &fakeStore{fishers: []Fisher{{DisplayName: "bob", Score: 9}, {DisplayName: "ghost", Score: 1}}}
	r := &fakeResolver{names: map[string]string{"bob": "B0b"}}
	var nameErrs []error
	svc := NewPublicService(store, r, func(err error) { nameErrs = append(nameErrs, err) })
	now := fixedNow
	svc.now = func() time.Time { return now }

	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.TopFishers[0].DisplayName != "B0b" || got.TopFishers[1].DisplayName != "ghost" || got.TopFishers[0].Score != 9 {
		t.Fatalf("fishers = %+v", got.TopFishers)
	}
	if store.fishers[0].DisplayName != "bob" {
		t.Error("store's slice must not be mutated")
	}
	if len(nameErrs) != 0 {
		t.Errorf("nameErrs = %v", nameErrs)
	}

	// Upstream down on the next refresh: payload still served, cached names
	// still used, failure reported but not fatal.
	r.mu.Lock()
	r.err = errors.New("helix down")
	r.mu.Unlock()
	store.mu.Lock()
	store.fishers = append(store.fishers, Fisher{DisplayName: "newbie", Score: 1})
	store.mu.Unlock()
	now = now.Add(CacheTTL)
	got, err = svc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.TopFishers[0].DisplayName != "B0b" || got.TopFishers[2].DisplayName != "newbie" {
		t.Fatalf("fishers = %+v", got.TopFishers)
	}
	if len(nameErrs) != 1 {
		t.Errorf("nameErrs = %v, want 1", nameErrs)
	}
}
