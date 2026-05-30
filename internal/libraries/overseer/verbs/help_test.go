package verbs

import (
	"testing"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

func TestParseHelp(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		wantErr bool
	}{
		{name: "bare help", input: []string{}},
		{name: "extra tokens reject", input: []string{"me"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseHelp(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && got.Kind != wire.ActionKindHelp {
				t.Errorf("kind = %q, want %q", got.Kind, wire.ActionKindHelp)
			}
		})
	}
}
