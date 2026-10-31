package twitcheventsub

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestHelix points helixBaseURL at handler and returns a client with a
// pre-seeded app token (so no token mint is attempted). Not parallel-safe:
// helixBaseURL is package state.
func newTestHelix(t *testing.T, handler http.HandlerFunc) *clientImpl {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	old := helixBaseURL
	helixBaseURL = srv.URL
	t.Cleanup(func() { helixBaseURL = old })
	return &clientImpl{
		cfg:         Config{ClientID: "cid"},
		httpClient:  srv.Client(),
		token:       "apptok",
		tokenExpiry: time.Now().Add(time.Hour),
	}
}

func TestGetStream(t *testing.T) {
	c := newTestHelix(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/streams" || r.URL.Query().Get("user_id") != "9" {
			t.Errorf("unexpected request %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer apptok" || r.Header.Get("Client-Id") != "cid" {
			t.Errorf("missing auth headers")
		}
		w.Write([]byte(`{"data":[{"type":"live","started_at":"2026-10-31T10:00:00Z"}]}`))
	})
	s, err := c.GetStream(context.Background(), "9")
	if err != nil || !s.Live || !s.StartedAt.Equal(time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("GetStream = %+v, %v", s, err)
	}

	off, err := parseStreamInfo([]byte(`{"data":[]}`))
	if err != nil || off.Live {
		t.Fatalf("offline = %+v, %v", off, err)
	}
}

func TestGetChannelInfoAndUser(t *testing.T) {
	c := newTestHelix(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/channels":
			w.Write([]byte(`{"data":[{"game_name":"Dwarf Fortress","title":"digging"}]}`))
		case "/users":
			if r.URL.Query().Get("login") == "bob" {
				w.Write([]byte(`{"data":[{"id":"7","login":"bob","display_name":"Bob","created_at":"2020-01-02T03:04:05Z"}]}`))
				return
			}
			w.Write([]byte(`{"data":[]}`))
		default:
			http.NotFound(w, r)
		}
	})
	ci, err := c.GetChannelInfo(context.Background(), "9")
	if err != nil || ci.GameName != "Dwarf Fortress" || ci.Title != "digging" {
		t.Fatalf("GetChannelInfo = %+v, %v", ci, err)
	}
	u, found, err := c.GetUserByLogin(context.Background(), "bob")
	if err != nil || !found || u.ID != "7" || u.CreatedAt.Year() != 2020 {
		t.Fatalf("GetUserByLogin = %+v, %v, %v", u, found, err)
	}
	_, found, err = c.GetUserByLogin(context.Background(), "ghost")
	if err != nil || found {
		t.Fatalf("missing user: found=%v err=%v", found, err)
	}
}

func TestGetChannelFollow(t *testing.T) {
	status, body := http.StatusOK, ""
	c := newTestHelix(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/channels/followers" || r.Header.Get("Authorization") != "Bearer usertok" {
			t.Errorf("unexpected request %s auth=%q", r.URL, r.Header.Get("Authorization"))
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	})
	ctx := context.Background()

	body = `{"total":1,"data":[{"user_id":"7","followed_at":"2025-06-18T12:00:00Z"}]}`
	f, err := c.GetChannelFollow(ctx, "usertok", "9", "7")
	if err != nil || !f.Following || f.FollowedAt.Year() != 2025 {
		t.Fatalf("following = %+v, %v", f, err)
	}

	body = `{"total":0,"data":[]}`
	f, err = c.GetChannelFollow(ctx, "usertok", "9", "7")
	if err != nil || f.Following {
		t.Fatalf("not following = %+v, %v", f, err)
	}

	// Not a moderator: Twitch returns only the total.
	body = `{"total":12}`
	if _, err = c.GetChannelFollow(ctx, "usertok", "9", "7"); !errors.Is(err, ErrFollowsUnavailable) {
		t.Fatalf("total-only response: err = %v", err)
	}

	// Missing scope.
	status, body = http.StatusUnauthorized, `{"error":"Unauthorized","message":"Missing scope: moderator:read:followers"}`
	if _, err = c.GetChannelFollow(ctx, "usertok", "9", "7"); !errors.Is(err, ErrFollowsUnavailable) {
		t.Fatalf("401: err = %v", err)
	}

	status = http.StatusInternalServerError
	if _, err = c.GetChannelFollow(ctx, "usertok", "9", "7"); err == nil || errors.Is(err, ErrFollowsUnavailable) {
		t.Fatalf("500 should be a plain error, got %v", err)
	}

	if _, err = c.GetChannelFollow(ctx, "", "9", "7"); !errors.Is(err, ErrFollowsUnavailable) {
		t.Fatalf("no token: err = %v", err)
	}
}

func TestHelixLookupsHonorContext(t *testing.T) {
	c := newTestHelix(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.GetStream(ctx, "9"); err == nil {
		t.Fatal("expected a timeout error")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("lookup ignored the context deadline")
	}
}
