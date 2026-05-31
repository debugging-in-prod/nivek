package verbs

import (
	"testing"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

func TestParseCutTree(t *testing.T) {
	got, err := ParseCutTree([]string{"(1,2,5)", "(3,4,5)"})
	if err != nil {
		t.Fatalf("ParseCutTree: %v", err)
	}
	if got.Kind != wire.ActionKindCutTree {
		t.Errorf("kind = %q, want cuttree", got.Kind)
	}
	if got.Region == nil ||
		got.Region.Min != (wire.Position{X: 1, Y: 2, Z: 5}) ||
		got.Region.Max != (wire.Position{X: 3, Y: 4, Z: 5}) {
		t.Errorf("region = %+v, want (1,2,5)-(3,4,5)", got.Region)
	}
}

func TestSubmitCutTree_LuaIteratesPlantVectors(t *testing.T) {
	ex := &fakeExecutor{}
	err := SubmitCutTree(ex, wire.Action{
		Kind: wire.ActionKindCutTree,
		Region: &wire.Region{
			Min: wire.Position{X: 92, Y: 92, Z: 35},
			Max: wire.Position{X: 102, Y: 99, Z: 35},
		},
	})
	if err != nil {
		t.Fatalf("SubmitCutTree: %v", err)
	}
	lua := ex.lastLua(t)
	assertContainsAll(t, lua,
		"df.global.world.plants.tree_dry",
		"df.global.world.plants.tree_wet",
		"local rawz = 35 - (df.global.world.map.region_z - 100)",
		"local minX, maxX, minY, maxY = 92, 102, 92, 99",
		"df.tile_dig_designation.Default",
		`if count == 0 then error("no trees in region") end`,
	)
}

func TestSubmitCutTree_Errors(t *testing.T) {
	ex := &fakeExecutor{}
	if err := SubmitCutTree(ex, wire.Action{Kind: wire.ActionKindCutTree}); err == nil {
		t.Error("expected error for nil region")
	}
}
