package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewStateRandom(t *testing.T) {
	a, err := newState()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := newState()
	if a == "" || a == b || len(a) < 40 {
		t.Fatalf("weak/duplicate state: %q %q", a, b)
	}
}

func TestCallbackHandlerState(t *testing.T) {
	const state = "expected-state"
	cases := []struct {
		name     string
		query    string
		wantCode int
		wantGot  string
		wantErr  bool
	}{
		{"valid", "?code=abc&state=expected-state", http.StatusOK, "abc", false},
		{"missing state", "?code=abc", http.StatusBadRequest, "", false},
		{"wrong state", "?code=abc&state=nope", http.StatusBadRequest, "", false},
		{"error wrong state", "?error=access_denied&state=nope", http.StatusBadRequest, "", false},
		{"error valid state", "?error=access_denied&state=expected-state", http.StatusBadRequest, "", true},
		{"favicon", "", http.StatusOK, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			codeCh, errCh := make(chan string, 1), make(chan string, 1)
			rec := httptest.NewRecorder()
			callbackHandler(state, codeCh, errCh).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+tc.query, nil))
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantCode)
			}
			select {
			case got := <-codeCh:
				if got != tc.wantGot {
					t.Fatalf("code = %q, want %q", got, tc.wantGot)
				}
			default:
				if tc.wantGot != "" {
					t.Fatal("expected code delivered")
				}
			}
			if gotErr := len(errCh) == 1; gotErr != tc.wantErr {
				t.Fatalf("errCh delivered = %v, want %v", gotErr, tc.wantErr)
			}
		})
	}
}
