package verbs

import (
	"testing"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

func TestParseCamera(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    *wire.Position
		wantErr bool
	}{
		{
			name:  "bare numbers",
			input: []string{"137", "115", "150"},
			want:  &wire.Position{X: 137, Y: 115, Z: 150},
		},
		{
			name:  "comma form",
			input: []string{"137,115,150"},
			want:  &wire.Position{X: 137, Y: 115, Z: 150},
		},
		{
			name:  "dashboard parens",
			input: []string{"(137,", "115,", "150)"},
			want:  &wire.Position{X: 137, Y: 115, Z: 150},
		},
		{name: "too few coords", input: []string{"1", "2"}, wantErr: true},
		{name: "too many coords", input: []string{"1", "2", "3", "4"}, wantErr: true},
		{name: "non-numeric", input: []string{"one", "two", "three"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCamera(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Kind != wire.ActionKindCamera {
				t.Errorf("kind = %q, want camera", got.Kind)
			}
			if got.Position == nil || *got.Position != *tt.want {
				t.Errorf("position = %+v, want %+v", got.Position, tt.want)
			}
		})
	}
}

func TestSubmitCamera(t *testing.T) {
	t.Run("emits raw-z conversion + revealInDwarfmodeMap", func(t *testing.T) {
		ex := &fakeExecutor{}
		err := SubmitCamera(ex, wire.Action{
			Kind:     wire.ActionKindCamera,
			Position: &wire.Position{X: 50, Y: 60, Z: 150},
		})
		if err != nil {
			t.Fatalf("SubmitCamera: %v", err)
		}
		lua := ex.lastLua(t)
		// Elevation 150 -> raw-z conversion uses region_z - 100.
		assertContainsAll(t, lua,
			"local rawz = 150 - (df.global.world.map.region_z - 100)",
			"dfhack.gui.revealInDwarfmodeMap({x=50,y=60,z=rawz}, true)",
		)
	})
	t.Run("nil position errors", func(t *testing.T) {
		ex := &fakeExecutor{}
		if err := SubmitCamera(ex, wire.Action{Kind: wire.ActionKindCamera}); err == nil {
			t.Error("expected error for nil position")
		}
	})
}
