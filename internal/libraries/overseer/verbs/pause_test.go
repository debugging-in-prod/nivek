package verbs

import (
	"testing"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

func TestParsePause(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    wire.ActionKind
		wantErr bool
	}{
		{name: "bare pause", input: []string{}, want: wire.ActionKindPause},
		{name: "extra tokens reject", input: []string{"now"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePause(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && got.Kind != tt.want {
				t.Errorf("kind = %q, want %q", got.Kind, tt.want)
			}
		})
	}
}

func TestParseUnpause(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		wantErr bool
	}{
		{name: "bare unpause", input: []string{}},
		{name: "extra tokens reject", input: []string{"now"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseUnpause(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && got.Kind != wire.ActionKindUnpause {
				t.Errorf("kind = %q, want %q", got.Kind, wire.ActionKindUnpause)
			}
		})
	}
}

func TestSubmitPauseAndUnpause(t *testing.T) {
	ex := &fakeExecutor{}
	if err := SubmitPause(ex, wire.Action{}); err != nil {
		t.Fatalf("SubmitPause: %v", err)
	}
	if err := SubmitUnpause(ex, wire.Action{}); err != nil {
		t.Fatalf("SubmitUnpause: %v", err)
	}
	if len(ex.LuaCalls) != 2 {
		t.Fatalf("expected 2 lua calls, got %d", len(ex.LuaCalls))
	}
	assertContains(t, ex.LuaCalls[0], "df.global.pause_state=true")
	assertContains(t, ex.LuaCalls[1], "df.global.pause_state=false")
}
