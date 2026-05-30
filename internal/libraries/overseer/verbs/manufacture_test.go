package verbs

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

func TestParseManufacture(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	tests := []struct {
		name     string
		input    []string
		wantItem string
		wantMat  *string
		wantQty  int
		wantErr  bool
		errMatch string
	}{
		{
			name:     "wood table default qty",
			input:    []string{"wood", "table"},
			wantItem: "table", wantMat: strPtr("wood"), wantQty: 1,
		},
		{
			name:     "qty + material + item",
			input:    []string{"5", "stone", "block"},
			wantItem: "block", wantMat: strPtr("stone"), wantQty: 5,
		},
		{
			name:     "plural strip",
			input:    []string{"wood", "tables"},
			wantItem: "table", wantMat: strPtr("wood"), wantQty: 1,
		},
		{
			name:     "wooden alias normalized to wood",
			input:    []string{"wooden", "table"},
			wantItem: "table", wantMat: strPtr("wood"), wantQty: 1,
		},
		{
			name:     "ash needs no material",
			input:    []string{"ash"},
			wantItem: "ash", wantMat: nil, wantQty: 1,
		},
		{
			name:     "ash silently ignores material if supplied",
			input:    []string{"wood", "ash"},
			wantItem: "ash", wantMat: nil, wantQty: 1,
		},
		{
			name:     "bed restricted to wood",
			input:    []string{"wood", "bed"},
			wantItem: "bed", wantMat: strPtr("wood"), wantQty: 1,
		},
		{name: "bed stone rejected", input: []string{"stone", "bed"}, wantErr: true, errMatch: "not allowed"},
		{name: "missing item", input: []string{}, wantErr: true},
		{name: "qty only", input: []string{"5"}, wantErr: true},
		{name: "unknown item", input: []string{"wood", "banana"}, wantErr: true, errMatch: "unknown item"},
		{name: "missing material on non-ash", input: []string{"table"}, wantErr: true, errMatch: "missing material"},
		{name: "unknown material", input: []string{"banana", "table"}, wantErr: true, errMatch: "unknown material"},
		{name: "negative qty", input: []string{"-3", "wood", "table"}, wantErr: true},
		{name: "extra tokens", input: []string{"foo", "wood", "table"}, wantErr: true, errMatch: "extra tokens"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseManufacture(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errMatch != "" && !strings.Contains(err.Error(), tt.errMatch) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errMatch)
				}
				return
			}
			if got.Item != tt.wantItem || got.Quantity != tt.wantQty {
				t.Errorf("got item=%q qty=%d, want item=%q qty=%d",
					got.Item, got.Quantity, tt.wantItem, tt.wantQty)
			}
			switch {
			case got.Material == nil && tt.wantMat != nil:
				t.Errorf("material = nil, want %q", *tt.wantMat)
			case got.Material != nil && tt.wantMat == nil:
				t.Errorf("material = %q, want nil", *got.Material)
			case got.Material != nil && tt.wantMat != nil && *got.Material != *tt.wantMat:
				t.Errorf("material = %q, want %q", *got.Material, *tt.wantMat)
			}
		})
	}
}

func TestSubmitManufacture_BasicJob(t *testing.T) {
	ex := &fakeExecutor{}
	mat := "wood"
	err := SubmitManufacture(ex, wire.Action{
		Kind:     wire.ActionKindManufacture,
		Item:     "table",
		Material: &mat,
		Quantity: 3,
	})
	if err != nil {
		t.Fatalf("SubmitManufacture: %v", err)
	}
	if len(ex.DFHackCalls) != 1 {
		t.Fatalf("expected 1 dfhack call, got %d", len(ex.DFHackCalls))
	}
	if ex.DFHackCalls[0][0] != "workorder" {
		t.Errorf("call[0] = %q, want workorder", ex.DFHackCalls[0][0])
	}
	var req workorderRequest
	if err := json.Unmarshal([]byte(ex.DFHackCalls[0][1]), &req); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if req.Job != "ConstructTable" {
		t.Errorf("Job = %q, want ConstructTable", req.Job)
	}
	if len(req.MaterialCategory) != 1 || req.MaterialCategory[0] != "wood" {
		t.Errorf("MaterialCategory = %v, want [wood]", req.MaterialCategory)
	}
	if req.AmountTotal != 3 {
		t.Errorf("AmountTotal = %d, want 3", req.AmountTotal)
	}
	if req.ItemSubtype != "" {
		t.Errorf("ItemSubtype = %q, want empty", req.ItemSubtype)
	}
}

func TestSubmitManufacture_StoneMaterial(t *testing.T) {
	ex := &fakeExecutor{}
	mat := "stone"
	if err := SubmitManufacture(ex, wire.Action{
		Item: "door", Material: &mat, Quantity: 1,
	}); err != nil {
		t.Fatalf("SubmitManufacture: %v", err)
	}
	var req workorderRequest
	if err := json.Unmarshal([]byte(ex.DFHackCalls[0][1]), &req); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if req.Material != "INORGANIC" {
		t.Errorf("Material = %q, want INORGANIC", req.Material)
	}
	if req.MaterialCategory != nil {
		t.Errorf("MaterialCategory = %v, want nil", req.MaterialCategory)
	}
}

func TestSubmitManufacture_IronMaterial(t *testing.T) {
	ex := &fakeExecutor{}
	mat := "iron"
	if err := SubmitManufacture(ex, wire.Action{Item: "door", Material: &mat, Quantity: 1}); err != nil {
		t.Fatalf("SubmitManufacture: %v", err)
	}
	var req workorderRequest
	if err := json.Unmarshal([]byte(ex.DFHackCalls[0][1]), &req); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if req.Material != "INORGANIC:IRON" {
		t.Errorf("Material = %q, want INORGANIC:IRON", req.Material)
	}
}

func TestSubmitManufacture_SubtypeJob(t *testing.T) {
	ex := &fakeExecutor{}
	mat := "wood"
	if err := SubmitManufacture(ex, wire.Action{
		Item: "bookcase", Material: &mat, Quantity: 1,
	}); err != nil {
		t.Fatalf("SubmitManufacture: %v", err)
	}
	var req workorderRequest
	if err := json.Unmarshal([]byte(ex.DFHackCalls[0][1]), &req); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if req.Job != "MakeTool" {
		t.Errorf("Job = %q, want MakeTool", req.Job)
	}
	if req.ItemSubtype != "ITEM_TOOL_BOOKCASE" {
		t.Errorf("ItemSubtype = %q, want ITEM_TOOL_BOOKCASE", req.ItemSubtype)
	}
}

func TestSubmitManufacture_FixedRecipeNoMaterial(t *testing.T) {
	ex := &fakeExecutor{}
	if err := SubmitManufacture(ex, wire.Action{Item: "ash", Quantity: 2}); err != nil {
		t.Fatalf("SubmitManufacture: %v", err)
	}
	var req workorderRequest
	if err := json.Unmarshal([]byte(ex.DFHackCalls[0][1]), &req); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if req.Job != "MakeAsh" {
		t.Errorf("Job = %q, want MakeAsh", req.Job)
	}
	if req.Material != "" || req.MaterialCategory != nil {
		t.Errorf("material populated for fixed-recipe job: req=%+v", req)
	}
}

func TestSubmitManufacture_UnknownItem(t *testing.T) {
	ex := &fakeExecutor{}
	mat := "wood"
	if err := SubmitManufacture(ex, wire.Action{Item: "banana", Material: &mat}); err == nil {
		t.Error("expected error for unknown item")
	}
}
