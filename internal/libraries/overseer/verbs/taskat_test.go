package verbs

import (
	"strings"
	"testing"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

func TestParseTaskat(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	tests := []struct {
		name     string
		input    []string
		wantID   int
		wantItem string
		wantMat  *string
		wantQty  int
		wantErr  bool
		errMatch string
	}{
		{
			name:   "minimal valid",
			input:  []string{"#2", "wood", "table"},
			wantID: 2, wantItem: "table", wantMat: strPtr("wood"), wantQty: 1,
		},
		{
			name:   "with qty",
			input:  []string{"#2", "5", "wood", "chair"},
			wantID: 2, wantItem: "chair", wantMat: strPtr("wood"), wantQty: 5,
		},
		{
			name:   "wooden alias normalized",
			input:  []string{"#7", "wooden", "table"},
			wantID: 7, wantItem: "table", wantMat: strPtr("wood"), wantQty: 1,
		},
		{name: "missing #", input: []string{"2", "wood", "table"}, wantErr: true, errMatch: "must be prefixed with #"},
		{name: "non-numeric id", input: []string{"#abc", "wood", "table"}, wantErr: true, errMatch: "invalid workshop id"},
		{name: "fixed-recipe item rejected", input: []string{"#2", "wood", "ash"}, wantErr: true, errMatch: "fixed recipe"},
		{name: "missing material+item", input: []string{"#2"}, wantErr: true},
		{name: "missing item", input: []string{"#2", "wood"}, wantErr: true},
		{name: "unknown item", input: []string{"#2", "wood", "banana"}, wantErr: true, errMatch: "unknown item"},
		{name: "unknown material", input: []string{"#2", "blah", "table"}, wantErr: true, errMatch: "unknown material"},
		{name: "zero qty rejected", input: []string{"#2", "0", "wood", "table"}, wantErr: true},
		{name: "multiple material tokens", input: []string{"#2", "wood", "stone", "table"}, wantErr: true, errMatch: "exactly one material"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTaskat(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errMatch != "" && !strings.Contains(err.Error(), tt.errMatch) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errMatch)
				}
				return
			}
			if got.WorkshopID != tt.wantID || got.Item != tt.wantItem || got.Quantity != tt.wantQty {
				t.Errorf("got id=%d item=%q qty=%d, want id=%d item=%q qty=%d",
					got.WorkshopID, got.Item, got.Quantity,
					tt.wantID, tt.wantItem, tt.wantQty)
			}
			if got.Material == nil || *got.Material != *tt.wantMat {
				t.Errorf("material = %v, want %q", got.Material, *tt.wantMat)
			}
		})
	}
}

func TestSubmitTaskat_WoodItem(t *testing.T) {
	ex := &fakeExecutor{}
	mat := "wood"
	err := SubmitTaskat(ex, wire.Action{
		Kind: wire.ActionKindTaskat, WorkshopID: 2, Item: "table",
		Material: &mat, Quantity: 3,
	})
	if err != nil {
		t.Fatalf("SubmitTaskat: %v", err)
	}
	lua := ex.lastLua(t)
	assertContainsAll(t, lua,
		"local bld = df.building.find(2)",
		"df.building_type.Workshop",
		"j.job_type = df.job_type.ConstructTable",
		"j.material_category.wood = true",
		"for i = 1, 3 do",
		"bld.jobs:insert",
		"dfhack.job.linkIntoWorld(j, true)",
	)
}

func TestSubmitTaskat_StoneMaterial(t *testing.T) {
	ex := &fakeExecutor{}
	mat := "stone"
	if err := SubmitTaskat(ex, wire.Action{
		WorkshopID: 5, Item: "door", Material: &mat, Quantity: 1,
	}); err != nil {
		t.Fatalf("SubmitTaskat: %v", err)
	}
	assertContainsAll(t, ex.lastLua(t),
		"j.mat_type = 0",
		"j.mat_index = -1",
	)
}

func TestSubmitTaskat_Errors(t *testing.T) {
	ex := &fakeExecutor{}
	mat := "wood"
	if err := SubmitTaskat(ex, wire.Action{Item: "table", Material: &mat}); err == nil {
		t.Error("expected error for zero workshop_id")
	}
	if err := SubmitTaskat(ex, wire.Action{WorkshopID: 1, Item: "table"}); err == nil {
		t.Error("expected error for nil material")
	}
	iron := "iron"
	if err := SubmitTaskat(ex, wire.Action{
		WorkshopID: 1, Item: "table", Material: &iron, Quantity: 1,
	}); err == nil {
		t.Error("expected error for unsupported material (iron not in taskat v1)")
	}
	bookcase := "wood"
	if err := SubmitTaskat(ex, wire.Action{
		WorkshopID: 1, Item: "bookcase", Material: &bookcase, Quantity: 1,
	}); err == nil {
		t.Error("expected error for item_subtype path (bookcase)")
	}
}
