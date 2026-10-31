package bot

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/quote"
)

type fakeQuoteService struct {
	quotes  map[int]quote.Quote
	err     error
	lastAdd string
}

func (f *fakeQuoteService) Random(string) (quote.Quote, bool, error) {
	for _, q := range f.quotes {
		return q, true, f.err
	}
	return quote.Quote{}, false, f.err
}

func (f *fakeQuoteService) Get(_ string, n int) (quote.Quote, bool, error) {
	q, ok := f.quotes[n]
	return q, ok, f.err
}

func (f *fakeQuoteService) Add(ch, text, by string, _ *string) (quote.Quote, error) {
	if f.err != nil {
		return quote.Quote{}, f.err
	}
	f.lastAdd = text
	return quote.Quote{ChannelTwitchId: ch, QuoteNumber: 3, Text: text, AddedBy: by}, nil
}

func (f *fakeQuoteService) Delete(_ string, n int) (bool, error) {
	_, ok := f.quotes[n]
	return ok, f.err
}

func serve(h echo.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	}
	rec := httptest.NewRecorder()
	_ = h(e.NewContext(req, rec))
	return rec
}

func TestGetQuoteHandler(t *testing.T) {
	t.Parallel()
	svc := &fakeQuoteService{quotes: map[int]quote.Quote{2: {QuoteNumber: 2, Text: "hi"}}}
	h := getQuoteHandler(svc)

	cases := []struct {
		target    string
		status    int
		wantFound bool
	}{
		{"/?channel_twitch_id=9&number=2", http.StatusOK, true},
		{"/?channel_twitch_id=9&number=5", http.StatusOK, false},
		{"/?channel_twitch_id=9", http.StatusOK, true},
		{"/?channel_twitch_id=9&number=abc", http.StatusBadRequest, false},
		{"/?channel_twitch_id=9&number=0", http.StatusBadRequest, false},
		{"/", http.StatusBadRequest, false},
	}
	for _, tc := range cases {
		rec := serve(h, http.MethodGet, tc.target, "")
		if rec.Code != tc.status {
			t.Errorf("%s: status %d, want %d", tc.target, rec.Code, tc.status)
			continue
		}
		if tc.status != http.StatusOK {
			continue
		}
		var resp quoteResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("%s: decode: %v", tc.target, err)
		}
		if resp.Found != tc.wantFound {
			t.Errorf("%s: found=%v, want %v", tc.target, resp.Found, tc.wantFound)
		}
	}

	svc.err = errors.New("db down")
	if rec := serve(h, http.MethodGet, "/?channel_twitch_id=9", ""); rec.Code != http.StatusInternalServerError {
		t.Errorf("db error: status %d, want 500", rec.Code)
	} else if strings.Contains(rec.Body.String(), "db down") {
		t.Error("internal error leaked to the response")
	}
}

func TestPostQuoteAddHandler(t *testing.T) {
	t.Parallel()
	svc := &fakeQuoteService{}
	h := postQuoteAddHandler(svc)

	rec := serve(h, http.MethodPost, "/", `{"channel_twitch_id":"9","text":"a  quote","added_by":"mod"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var resp quoteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Quote == nil || resp.Quote.QuoteNumber != 3 {
		t.Fatalf("resp = %+v (%v)", resp, err)
	}

	for _, body := range []string{
		`{"text":"x"}`,
		`{"channel_twitch_id":"9","text":"   "}`,
		`{"channel_twitch_id":"9","text":"` + strings.Repeat("x", quote.MaxTextLen+1) + `"}`,
		`not json`,
	} {
		if rec := serve(h, http.MethodPost, "/", body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %.40q: status %d, want 400", body, rec.Code)
		}
	}
}

func TestPostQuoteDeleteHandler(t *testing.T) {
	t.Parallel()
	svc := &fakeQuoteService{quotes: map[int]quote.Quote{1: {QuoteNumber: 1}}}
	h := postQuoteDeleteHandler(svc)

	for body, want := range map[string]string{
		`{"channel_twitch_id":"9","number":1}`: `"found":true`,
		`{"channel_twitch_id":"9","number":4}`: `"found":false`,
	} {
		rec := serve(h, http.MethodPost, "/", body)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body)
		}
	}
	if rec := serve(h, http.MethodPost, "/", `{"channel_twitch_id":"9","number":0}`); rec.Code != http.StatusBadRequest {
		t.Errorf("number 0: status %d, want 400", rec.Code)
	}
}
