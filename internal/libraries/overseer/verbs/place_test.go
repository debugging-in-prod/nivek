package verbs

import (
	"strings"
	"testing"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

func TestParsePlace(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		wantItem string
		wantPos  *wire.Position
		wantErr  bool
		errMatch string
	}{
		{
			name:     "bare item + coords",
			input:    []string{"table", "5", "10", "30"},
			wantItem: "table",
			wantPos:  &wire.Position{X: 5, Y: 10, Z: 30},
		},
		{
			name:     "plural strip",
			input:    []string{"chairs", "1", "2", "3"},
			wantItem: "chair",
			wantPos:  &wire.Position{X: 1, Y: 2, Z: 3},
		},
		{
			name:     "dashboard paste",
			input:    []string{"door", "(1,", "2,", "3)"},
			wantItem: "door",
			wantPos:  &wire.Position{X: 1, Y: 2, Z: 3},
		},
		{
			name:     "workshop",
			input:    []string{"carpenter", "10", "10", "30"},
			wantItem: "carpenter",
			wantPos:  &wire.Position{X: 10, Y: 10, Z: 30},
		},
		{name: "missing args", input: []string{}, wantErr: true},
		{name: "unknown item", input: []string{"banana", "1", "2", "3"}, wantErr: true, errMatch: "not placeable"},
		{name: "missing coords", input: []string{"table"}, wantErr: true},
		{name: "too few coords", input: []string{"table", "1", "2"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePlace(tt.input)
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
			if got.Position == nil || *got.Position != *tt.wantPos {
				t.Errorf("position = %+v, want %+v", got.Position, tt.wantPos)
			}
		})
	}
}

func TestSubmitPlace_FurnitureUsesBuildingplan(t *testing.T) {
	ex := &fakeExecutor{}
	err := SubmitPlace(ex, wire.Action{
		Kind:     wire.ActionKindPlace,
		Item:     "table",
		Position: &wire.Position{X: 5, Y: 10, Z: 30},
	})
	if err != nil {
		t.Fatalf("SubmitPlace: %v", err)
	}
	lua := ex.lastLua(t)
	assertContainsAll(t, lua,
		"df.building_type.Table",
		"local subtype = -1",
		"local rawz = 30 - (df.global.world.map.region_z - 100)",
		"pos = {x = 5, y = 10, z = rawz}",
		"bp.addPlannedBuilding(bld)",
		"bp.scheduleCycle()",
	)
}

func TestSubmitPlace_WorkshopHasSubtype(t *testing.T) {
	ex := &fakeExecutor{}
	err := SubmitPlace(ex, wire.Action{
		Kind:     wire.ActionKindPlace,
		Item:     "carpenter",
		Position: &wire.Position{X: 10, Y: 10, Z: 30},
	})
	if err != nil {
		t.Fatalf("SubmitPlace: %v", err)
	}
	lua := ex.lastLua(t)
	assertContainsAll(t, lua,
		"df.building_type.Workshop",
		"df.workshop_type.Carpenters",
	)
}

func TestSubmitPlace_FurnaceHasSubtype(t *testing.T) {
	ex := &fakeExecutor{}
	err := SubmitPlace(ex, wire.Action{
		Kind:     wire.ActionKindPlace,
		Item:     "smelter",
		Position: &wire.Position{X: 10, Y: 10, Z: 30},
	})
	if err != nil {
		t.Fatalf("SubmitPlace: %v", err)
	}
	assertContainsAll(t, ex.lastLua(t),
		"df.building_type.Furnace",
		"df.furnace_type.Smelter",
	)
}

func TestSubmitPlace_Errors(t *testing.T) {
	ex := &fakeExecutor{}
	if err := SubmitPlace(ex, wire.Action{Kind: wire.ActionKindPlace, Item: "table"}); err == nil {
		t.Error("expected error for nil position")
	}
	if err := SubmitPlace(ex, wire.Action{
		Kind:     wire.ActionKindPlace,
		Item:     "banana",
		Position: &wire.Position{},
	}); err == nil {
		t.Error("expected error for unknown item")
	}
}
