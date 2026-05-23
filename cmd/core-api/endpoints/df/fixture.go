package df

import (
	"time"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer"
)

// buildFixtureSnapshot returns a hand-crafted snapshot covering three Z
// levels so the dashboard has something interesting to navigate before any
// real DFHack data is wired in. All three share the same 50×50 footprint
// anchored at world origin (100, 100).
//
//   - Z=149 (below):  underground tunnel + small carved room with a coffin
//   - Z=150 (surface): the main fortress — two carved rooms separated by a
//     wall with a door, water pool in the corner, stair down, ramp up,
//     trees outside the perimeter, table+chair+bed+door+coffin
//   - Z=151 (above):  open sky with the staircase landing, a tiny watch
//     post (cabinet + chair), tree canopies poking up from Z=150
func buildFixtureSnapshot() overseer.MapSnapshot {
	const (
		width  = 50
		height = 50
		baseZ  = 149 // lowest Z in this snapshot
	)

	levels := []overseer.ZLevel{
		buildFixtureLevelBelow(width, height, 149),
		buildFixtureLevelSurface(width, height, 150),
		buildFixtureLevelAbove(width, height, 151),
	}

	return overseer.MapSnapshot{
		CapturedAt: time.Now().UTC(),
		Origin:     overseer.Position{X: 100, Y: 100, Z: baseZ},
		Width:      width,
		Height:     height,
		Levels:     levels,
	}
}

// buildFixtureLevelBelow is a tunnel + small chamber underground.
func buildFixtureLevelBelow(width, height, z int) overseer.ZLevel {
	tiles := make([]overseer.TileType, width*height)
	// Underground: most of the map is unmined wall.
	for i := range tiles {
		tiles[i] = overseer.TileWall
	}
	set := tileSetter(tiles, width, height)

	// A small carved chamber.
	for x := 15; x <= 20; x++ {
		for y := 15; y <= 20; y++ {
			set(x, y, overseer.TileFloor)
		}
	}
	// Tunnel connecting it east-ish.
	for x := 20; x <= 32; x++ {
		set(x, 17, overseer.TileFloor)
		set(x, 18, overseer.TileFloor)
	}
	// Stair up to the surface at (8, 8) — matches Z=150's stair column.
	set(8, 8, overseer.TileStair)

	furniture := []overseer.FurniturePlace{
		{Type: "coffin", Material: "stone", X: 100 + 17, Y: 100 + 17},
	}

	return overseer.ZLevel{Z: z, Tiles: tiles, Furniture: furniture}
}

// buildFixtureLevelSurface is the existing single-Z fixture content (now
// just one of three levels), with the same rooms and pool.
func buildFixtureLevelSurface(width, height, z int) overseer.ZLevel {
	tiles := make([]overseer.TileType, width*height)
	for i := range tiles {
		tiles[i] = overseer.TileFloor
	}
	set := tileSetter(tiles, width, height)

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
			continue
		}
		set(20, y, overseer.TileWall)
	}
	// Pool of water in the bottom-right of the interior.
	for x := 28; x <= 33; x++ {
		for y := 28; y <= 33; y++ {
			set(x, y, overseer.TileWater)
		}
	}
	// Stair down to Z=149 in the NW room corner.
	set(8, 8, overseer.TileStair)
	// Ramp up to Z=151 in the NE corner.
	set(32, 8, overseer.TileRamp)
	// Trees outside the perimeter.
	set(40, 40, overseer.TileTree)
	set(42, 38, overseer.TileTree)

	furniture := []overseer.FurniturePlace{
		{Type: "table", Material: "stone", X: 100 + 12, Y: 100 + 15},
		{Type: "chair", Material: "stone", X: 100 + 13, Y: 100 + 15},
		{Type: "bed", Material: "wood", X: 100 + 28, Y: 100 + 12},
		{Type: "door", Material: "wood", X: 100 + 20, Y: 100 + 20},
		{Type: "coffin", Material: "stone", X: 100 + 32, Y: 100 + 32},
	}

	return overseer.ZLevel{Z: z, Tiles: tiles, Furniture: furniture}
}

// buildFixtureLevelAbove is open sky with a small watch post.
func buildFixtureLevelAbove(width, height, z int) overseer.ZLevel {
	tiles := make([]overseer.TileType, width*height)
	// Open sky represented as TileFloor (default rendering color).
	for i := range tiles {
		tiles[i] = overseer.TileFloor
	}
	set := tileSetter(tiles, width, height)

	// Tree canopies poking up from Z=150's tree tiles.
	set(40, 40, overseer.TileTree)
	set(42, 38, overseer.TileTree)
	// Stair landing matching Z=149/150 stair column.
	set(8, 8, overseer.TileStair)
	// A tiny watch post — a 3x3 wall ring with a chair+cabinet inside.
	for x := 31; x <= 33; x++ {
		set(x, 7, overseer.TileWall)
		set(x, 9, overseer.TileWall)
	}
	set(31, 8, overseer.TileWall)
	set(33, 8, overseer.TileWall)

	furniture := []overseer.FurniturePlace{
		{Type: "chair", Material: "wood", X: 100 + 32, Y: 100 + 8},
		{Type: "cabinet", Material: "wood", X: 100 + 32, Y: 100 + 9},
	}

	return overseer.ZLevel{Z: z, Tiles: tiles, Furniture: furniture}
}

// tileSetter returns a bounds-checked setter for a single Z level's tile
// slice. Used to keep level builders readable.
func tileSetter(tiles []overseer.TileType, width, height int) func(x, y int, t overseer.TileType) {
	return func(x, y int, t overseer.TileType) {
		if x < 0 || x >= width || y < 0 || y >= height {
			return
		}
		tiles[y*width+x] = t
	}
}
