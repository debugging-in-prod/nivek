package verbs

import (
	"fmt"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// ParseCamera handles `camera <x> <y> <z>` (or any of the coord
// tolerances extractCoordInts accepts). z is in-game elevation; the
// executor converts to raw embark-local z at the DFHack boundary.
func ParseCamera(rest []string) (wire.Action, error) {
	coords, err := extractCoordInts(rest)
	if err != nil {
		return wire.Action{}, err
	}
	if len(coords) != 3 {
		return wire.Action{}, fmt.Errorf("camera needs 3 coordinates, got %d", len(coords))
	}
	return wire.Action{
		Kind:     wire.ActionKindCamera,
		Position: &wire.Position{X: coords[0], Y: coords[1], Z: coords[2]},
	}, nil
}

// SubmitCamera recenters the DF camera on the given tile.
func SubmitCamera(ex Executor, action wire.Action) error {
	if action.Position == nil {
		return fmt.Errorf("camera requires position")
	}
	// Position.Z is elevation (dashboard-native); DFHack APIs want raw
	// embark-local z. Convert in the lua so we don't need a separate
	// region_z fetch.
	script := fmt.Sprintf(`local rawz = %d - (df.global.world.map.region_z - 100); dfhack.gui.revealInDwarfmodeMap({x=%d,y=%d,z=rawz}, true)`,
		action.Position.Z, action.Position.X, action.Position.Y)
	return ex.RunLua(script)
}
