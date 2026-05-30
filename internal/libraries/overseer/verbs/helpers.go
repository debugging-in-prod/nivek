package verbs

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// regionVerbMaxArea is the per-command cap on the rectangular tile area
// for region-based dig verbs (mine, channel, digramp, cuttree, stockpile,
// zone). Keeps individual commands bounded.
const regionVerbMaxArea = 100

// extractCoordInts pulls all integer tokens out of a coord-list. Tolerates
// the formats chatters actually type or copy off the dashboard:
//   - bare:               `137 115 150`
//   - commas:             `137,115,150` / `137, 115, 150`
//   - dashboard parens:   `(137, 115, 150)` / `(137,115,150) (142,118,150)`
//
// Strips `(`, `)`, and `,`, then splits on whitespace and Atoi's each token.
// Returns the integers in the order the chatter wrote them; callers decide
// how many they expect and what each slot means.
func extractCoordInts(rest []string) ([]int, error) {
	joined := strings.Join(rest, " ")
	for _, ch := range []string{"(", ")", ","} {
		joined = strings.ReplaceAll(joined, ch, " ")
	}
	parts := strings.Fields(joined)
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid coordinate %q", p)
		}
		out[i] = n
	}
	return out, nil
}

// parseRegionVerb parses the coord-list shared by all rectangular dig
// verbs. Accepts:
//   - 3 ints — single tile (x,y,z); 1×1 region
//   - 5 ints — legacy two-corner form: x1,y1,z + x2,y2
//   - 6 ints — dashboard copy-paste: (x1,y1,z) + (x2,y2,z); the second z is
//     silently discarded to preserve the single-Z invariant
//
// Anything else rejects. verb is the chat-facing verb name, used only in
// error messages.
func parseRegionVerb(verb string, rest []string) (*wire.Region, error) {
	coords, err := extractCoordInts(rest)
	if err != nil {
		return nil, err
	}
	var x1, y1, z, x2, y2 int
	switch len(coords) {
	case 3:
		x1, y1, z = coords[0], coords[1], coords[2]
		x2, y2 = x1, y1
	case 5, 6:
		x1, y1, z, x2, y2 = coords[0], coords[1], coords[2], coords[3], coords[4]
	default:
		return nil, fmt.Errorf("%s needs (x,y,z) or (x1,y1,z) (x2,y2[,z]) — 3, 5, or 6 numbers total, got %d", verb, len(coords))
	}

	dx := abs(x2-x1) + 1
	dy := abs(y2-y1) + 1
	area := dx * dy
	if area > regionVerbMaxArea {
		return nil, fmt.Errorf("%s area %dx%d=%d tiles exceeds %d-tile cap", verb, dx, dy, area, regionVerbMaxArea)
	}

	return &wire.Region{
		Min: wire.Position{X: x1, Y: y1, Z: z},
		Max: wire.Position{X: x2, Y: y2, Z: z},
	}, nil
}

// abs returns |n|. Used by parseRegionVerb to compute rectangle
// dimensions regardless of which corner the chatter listed first.
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// stripFillerWords removes the chat-tolerance tokens (a, an, the, some,
// me, us, please, drink, from) before grammar matching.
func stripFillerWords(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if _, ok := fillerWords[t]; ok {
			continue
		}
		out = append(out, t)
	}
	return out
}

// StripFillerWords is the exported variant called by the public
// ParseCommand dispatcher in package overseer.
func StripFillerWords(tokens []string) []string {
	return stripFillerWords(tokens)
}

// normalizeMaterial folds adjectival material aliases (wooden → wood)
// to the canonical token.
func normalizeMaterial(token string) string {
	if alias, ok := materialAliases[token]; ok {
		return alias
	}
	return token
}
