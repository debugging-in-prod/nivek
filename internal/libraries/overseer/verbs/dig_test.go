package verbs

import (
	"strings"
	"testing"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// TestParseRegionVerb_Forms covers the shared coord-shape contract for
// mine/channel/digramp/cuttree/stockpile/zone. We test through ParseMine
// since they all go through parseRegionVerb.
func TestParseRegionVerb_Forms(t *testing.T) {
	tests := []struct {
		name      string
		input     []string
		wantMin   wire.Position
		wantMax   wire.Position
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "single tile (3 ints)",
			input:   []string{"(97,", "87,", "35)"},
			wantMin: wire.Position{X: 97, Y: 87, Z: 35},
			wantMax: wire.Position{X: 97, Y: 87, Z: 35},
		},
		{
			name:    "two corner legacy (5 ints)",
			input:   []string{"0,0,5", "3,4"},
			wantMin: wire.Position{X: 0, Y: 0, Z: 5},
			wantMax: wire.Position{X: 3, Y: 4, Z: 5},
		},
		{
			name:    "dashboard paste (6 ints; second z ignored)",
			input:   []string{"(97,", "87,", "35)", "(99,", "90,", "35)"},
			wantMin: wire.Position{X: 97, Y: 87, Z: 35},
			wantMax: wire.Position{X: 99, Y: 90, Z: 35},
		},
		{
			name:    "second-corner-z silently ignored even if differs",
			input:   []string{"1,1,5", "3,3,99"},
			wantMin: wire.Position{X: 1, Y: 1, Z: 5},
			wantMax: wire.Position{X: 3, Y: 3, Z: 5},
		},
		{name: "4 ints rejected", input: []string{"1", "2", "3", "4"}, wantErr: true, errSubstr: "3, 5, or 6"},
		{name: "non-numeric rejected", input: []string{"a", "b", "c"}, wantErr: true, errSubstr: "invalid coordinate"},
		{
			name:      "area-cap exceeded",
			input:     []string{"0,0,1", "10,10"},
			wantErr:   true,
			errSubstr: "exceeds 100-tile cap",
		},
		{
			name:    "max area accepted (10x10 = 100)",
			input:   []string{"0,0,1", "9,9"},
			wantMin: wire.Position{X: 0, Y: 0, Z: 1},
			wantMax: wire.Position{X: 9, Y: 9, Z: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMine(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if got.Region == nil {
				t.Fatal("expected non-nil Region")
			}
			if got.Region.Min != tt.wantMin || got.Region.Max != tt.wantMax {
				t.Errorf("region = (%+v -> %+v), want (%+v -> %+v)",
					got.Region.Min, got.Region.Max, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestParseDigVerbs_KindStamps(t *testing.T) {
	// Each dig verb should preserve the same Region but stamp its own Kind.
	in := []string{"1,2,5", "3,4"}
	cases := []struct {
		parse func([]string) (wire.Action, error)
		want  wire.ActionKind
	}{
		{ParseMine, wire.ActionKindMine},
		{ParseChannel, wire.ActionKindChannel},
		{ParseDigRamp, wire.ActionKindDigRamp},
	}
	for _, c := range cases {
		got, err := c.parse(in)
		if err != nil {
			t.Errorf("%s parse: %v", c.want, err)
			continue
		}
		if got.Kind != c.want {
			t.Errorf("kind = %q, want %q", got.Kind, c.want)
		}
	}
}

func TestSubmitDigDesignation_EnumPerVerb(t *testing.T) {
	region := &wire.Region{
		Min: wire.Position{X: 10, Y: 20, Z: 35},
		Max: wire.Position{X: 12, Y: 22, Z: 35},
	}
	cases := []struct {
		name     string
		submit   func(Executor, wire.Action) error
		kind     wire.ActionKind
		wantEnum string
	}{
		{"mine -> Default", SubmitMine, wire.ActionKindMine, "df.tile_dig_designation.Default"},
		{"channel -> Channel", SubmitChannel, wire.ActionKindChannel, "df.tile_dig_designation.Channel"},
		{"digramp -> Ramp", SubmitDigRamp, wire.ActionKindDigRamp, "df.tile_dig_designation.Ramp"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ex := &fakeExecutor{}
			if err := c.submit(ex, wire.Action{Kind: c.kind, Region: region}); err != nil {
				t.Fatalf("submit: %v", err)
			}
			lua := ex.lastLua(t)
			assertContainsAll(t, lua,
				c.wantEnum,
				"local rawz = 35 - (df.global.world.map.region_z - 100)",
				"for x = 10, 12 do",
				"for y = 20, 22 do",
				"block.flags.designated = true",
			)
		})
	}
}

func TestSubmitDigDesignation_Errors(t *testing.T) {
	t.Run("nil region", func(t *testing.T) {
		ex := &fakeExecutor{}
		if err := SubmitMine(ex, wire.Action{Kind: wire.ActionKindMine}); err == nil {
			t.Error("expected error for nil region")
		}
	})
	t.Run("multi-Z rejected", func(t *testing.T) {
		ex := &fakeExecutor{}
		err := SubmitMine(ex, wire.Action{
			Kind: wire.ActionKindMine,
			Region: &wire.Region{
				Min: wire.Position{Z: 1},
				Max: wire.Position{Z: 2},
			},
		})
		if err == nil {
			t.Error("expected error for multi-Z")
		}
	})
}

func TestSubmitDigDesignation_SwapsCorners(t *testing.T) {
	// Min/Max can come in either order; the lua loop should always go
	// low-to-high (otherwise `for x = high, low` would iterate zero times).
	ex := &fakeExecutor{}
	err := SubmitMine(ex, wire.Action{
		Kind: wire.ActionKindMine,
		Region: &wire.Region{
			Min: wire.Position{X: 9, Y: 9, Z: 5},
			Max: wire.Position{X: 5, Y: 5, Z: 5},
		},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	assertContainsAll(t, ex.lastLua(t), "for x = 5, 9 do", "for y = 5, 9 do")
}
