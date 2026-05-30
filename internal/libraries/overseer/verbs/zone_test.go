package verbs

import (
	"strings"
	"testing"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

func TestParseZone(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		wantItem string
		wantErr  bool
		errMatch string
	}{
		{name: "office", input: []string{"office", "(1,2,3)", "(5,5,3)"}, wantItem: "office"},
		{name: "bedroom", input: []string{"bedroom", "(1,2,3)"}, wantItem: "bedroom"},
		{name: "dormitory", input: []string{"dormitory", "(1,2,3)", "(5,5,3)"}, wantItem: "dormitory"},
		{name: "missing args", input: []string{}, wantErr: true},
		{name: "unknown type", input: []string{"library", "(1,2,3)"}, wantErr: true, errMatch: "unknown zone type"},
		{name: "missing coords", input: []string{"office"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseZone(tt.input)
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
			if got.Kind != wire.ActionKindZone {
				t.Errorf("kind = %q, want zone", got.Kind)
			}
		})
	}
}

func TestSubmitZone_PerType(t *testing.T) {
	region := &wire.Region{
		Min: wire.Position{X: 10, Y: 20, Z: 30},
		Max: wire.Position{X: 12, Y: 22, Z: 30},
	}
	cases := []struct {
		zoneItem string
		wantEnum string
	}{
		{"office", "df.civzone_type.Office"},
		{"bedroom", "df.civzone_type.Bedroom"},
		{"dormitory", "df.civzone_type.Dormitory"},
	}
	for _, c := range cases {
		t.Run(c.zoneItem, func(t *testing.T) {
			ex := &fakeExecutor{}
			err := SubmitZone(ex, wire.Action{
				Kind:   wire.ActionKindZone,
				Item:   c.zoneItem,
				Region: region,
			})
			if err != nil {
				t.Fatalf("SubmitZone: %v", err)
			}
			assertContainsAll(t, ex.lastLua(t),
				"df.building_type.Civzone",
				"abstract = true",
				c.wantEnum,
				"width = 3, height = 3",
			)
		})
	}
}

func TestSubmitZone_Errors(t *testing.T) {
	ex := &fakeExecutor{}
	if err := SubmitZone(ex, wire.Action{Kind: wire.ActionKindZone, Item: "office"}); err == nil {
		t.Error("expected error for nil region")
	}
	if err := SubmitZone(ex, wire.Action{
		Kind:   wire.ActionKindZone,
		Item:   "unknown",
		Region: &wire.Region{},
	}); err == nil {
		t.Error("expected error for unknown zone item")
	}
}
