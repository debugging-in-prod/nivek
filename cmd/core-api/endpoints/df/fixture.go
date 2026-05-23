package df

import (
	"time"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer"
)

// buildFixtureSnapshot returns a hand-crafted snapshot so the dashboard has
// something interesting to render before any real DFHack data is wired in.
// A 50x50 area at Z=150 with: an outer wall, two carved rooms (one with a
// table+chair, one with a bed), a connecting corridor, a door, a pool of
// water, and a tree outside the walls.
//
// Origin (100, 100, 150) is arbitrary — picked to look like a real fortress
// coord rather than (0,0,0) so the HUD readout shows realistic numbers.
func buildFixtureSnapshot() overseer.MapSnapshot {
	const (
		width  = 50
		height = 50
		zLevel = 150
	)

	tiles := make([]overseer.TileType, width*height)
	// Default everything to floor; we'll overwrite specific cells below.
	for i := range tiles {
		tiles[i] = overseer.TileFloor
	}

	set := func(x, y int, t overseer.TileType) {
		if x < 0 || x >= width || y < 0 || y >= height {
			return
		}
		tiles[y*width+x] = t
	}

	// Outer perimeter wall around a 30x30 interior (rows/cols 5..35).
	for x := 5; x <= 35; x++ {
		set(x, 5, overseer.TileWall)
		set(x, 35, overseer.TileWall)
	}
	for y := 5; y <= 35; y++ {
		set(5, y, overseer.TileWall)
		set(35, y, overseer.TileWall)
	}

	// Dividing wall down the middle, with a door gap at y=20.
	for y := 5; y <= 35; y++ {
		if y == 20 {
			continue // door gap
		}
		set(20, y, overseer.TileWall)
	}

	// Pool of water in the bottom-right corner of the interior.
	for x := 28; x <= 33; x++ {
		for y := 28; y <= 33; y++ {
			set(x, y, overseer.TileWater)
		}
	}

	// A staircase in the top-left room corner.
	set(8, 8, overseer.TileStair)

	// A ramp connecting Z levels in the top-right area.
	set(32, 8, overseer.TileRamp)

	// A tree outside the perimeter walls.
	set(40, 40, overseer.TileTree)
	set(42, 38, overseer.TileTree)

	// Furniture: chair + table in the west room, bed in the east room, door
	// in the dividing wall, coffin in the east room corner.
	furniture := []overseer.FurniturePlace{
		{Type: "table", Material: "stone", X: 100 + 12, Y: 100 + 15},
		{Type: "chair", Material: "stone", X: 100 + 13, Y: 100 + 15},
		{Type: "bed", Material: "wood", X: 100 + 28, Y: 100 + 12},
		{Type: "door", Material: "wood", X: 100 + 20, Y: 100 + 20},
		{Type: "coffin", Material: "stone", X: 100 + 32, Y: 100 + 32},
	}

	return overseer.MapSnapshot{
		CapturedAt: time.Now().UTC(),
		Origin:     overseer.Position{X: 100, Y: 100, Z: zLevel},
		Width:      width,
		Height:     height,
		Z:          zLevel,
		Tiles:      tiles,
		Furniture:  furniture,
	}
}
