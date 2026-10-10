package deepl

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranslateAt_Success(t *testing.T) {
	var gotAuth, gotTarget, gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = r.ParseForm()
		gotTarget = r.Form.Get("target_lang")
		gotText = r.Form.Get("text")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"translations":[{"detected_source_language":"EN","text":"こんにちは"}]}`))
	}))
	defer srv.Close()

	out, err := translateAt(srv.Client(), srv.URL, "key:fx", "hello", "JA", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "こんにちは" {
		t.Fatalf("got %q, want %q", out, "こんにちは")
	}
	if gotAuth != "DeepL-Auth-Key key:fx" {
		t.Errorf("auth header = %q", gotAuth)
	}
	if gotTarget != "JA" || gotText != "hello" {
		t.Errorf("form target=%q text=%q", gotTarget, gotText)
	}
}

func TestTranslateAt_QuotaExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(456)
	}))
	defer srv.Close()

	_, err := translateAt(srv.Client(), srv.URL, "key:fx", "hello", "JA", "")
	if err == nil || !strings.Contains(err.Error(), "quota") {
		t.Fatalf("want quota error, got %v", err)
	}
}

func TestTranslateAt_MissingKey(t *testing.T) {
	if _, err := translateAt(http.DefaultClient, "http://unused", "", "hello", "JA", ""); err == nil {
		t.Fatal("want error for empty api key")
	}
}
