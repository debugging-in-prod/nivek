package verbs

import (
	"strings"
	"testing"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

func TestParseStockpile(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		wantItem string
		wantErr  bool
		errMatch string
	}{
		{
			name:     "single category single-tile",
			input:    []string{"wood", "(1,2,3)"},
			wantItem: "wood",
		},
		{
			name:     "plural strip (animals -> animal)",
			input:    []string{"animals", "(1,2,3)", "(2,3,3)"},
			wantItem: "animal",
		},
		{
			name:     "all category",
			input:    []string{"all", "(1,2,3)"},
			wantItem: "all",
		},
		{name: "missing args", input: []string{}, wantErr: true},
		{name: "unknown category", input: []string{"blah", "(1,2,3)"}, wantErr: true, errMatch: "unknown stockpile category"},
		{name: "missing coords", input: []string{"wood"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseStockpile(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errMatch != "" && !strings.Contains(err.Error(), tt.errMatch) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errMatch)
				}
				return
			}
			if got.Item != tt.wantItem {
				t.Errorf("item = %q, want %q", got.Item, tt.wantItem)
			}
			if got.Kind != wire.ActionKindStockpile {
				t.Errorf("kind = %q, want stockpile", got.Kind)
			}
		})
	}
}

func TestSubmitStockpile_LuaUsesPreset(t *testing.T) {
	ex := &fakeExecutor{}
	err := SubmitStockpile(ex, wire.Action{
		Kind: wire.ActionKindStockpile,
		Item: "wood",
		Region: &wire.Region{
			Min: wire.Position{X: 10, Y: 20, Z: 30},
			Max: wire.Position{X: 12, Y: 22, Z: 30},
		},
	})
	if err != nil {
		t.Fatalf("SubmitStockpile: %v", err)
	}
	lua := ex.lastLua(t)
	assertContainsAll(t, lua,
		"df.building_type.Stockpile",
		"abstract = true",
		"local rawz = 30 - (df.global.world.map.region_z - 100)",
		"pos = {x=10, y=20, z=rawz}",
		"width = 3, height = 3",
		`require("plugins.stockpiles").import_settings("library/cat_wood"`,
	)
}

func TestSubmitStockpile_AllPreset(t *testing.T) {
	ex := &fakeExecutor{}
	err := SubmitStockpile(ex, wire.Action{
		Kind: wire.ActionKindStockpile,
		Item: "all",
		Region: &wire.Region{
			Min: wire.Position{X: 1, Y: 1, Z: 5},
			Max: wire.Position{X: 1, Y: 1, Z: 5},
		},
	})
	if err != nil {
		t.Fatalf("SubmitStockpile: %v", err)
	}
	assertContains(t, ex.lastLua(t), `"library/all"`)
}

func TestSubmitStockpile_Errors(t *testing.T) {
	ex := &fakeExecutor{}
	if err := SubmitStockpile(ex, wire.Action{Kind: wire.ActionKindStockpile, Item: "wood"}); err == nil {
		t.Error("expected error for nil region")
	}
}
