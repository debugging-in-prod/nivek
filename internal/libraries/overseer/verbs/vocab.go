package verbs

// Shared vocabularies used by multiple verbs. Per-verb-only vocabs (e.g.
// stockpileCategoryToPreset) live in the verb file they belong to.

// itemVocab is the set of chat-facing item nouns the parser recognizes.
// Plural and case variations are normalized before lookup (the trailing
// "s" is stripped before lookup, so all entries are singular even when
// the DFHack-side job_type is plural — e.g. `block` -> `ConstructBlocks`).
var itemVocab = map[string]struct{}{
	"table":         {},
	"bed":           {},
	"door":          {},
	"chair":         {},
	"throne":        {}, // synonym for chair; both -> ConstructThrone
	"coffin":        {},
	"block":         {},
	"cabinet":       {},
	"chest":         {},
	"statue":        {},
	"floodgate":     {},
	"bucket":        {},
	"barrel":        {},
	"ash":           {},
	"charcoal":      {},
	"armorstand":    {},
	"grate":         {},
	"hatchcover":    {},
	"millstone":     {},
	"quern":         {},
	"slab":          {},
	"weaponrack":    {},
	"altar":         {},
	"bookcase":      {},
	"pedestal":      {},
	"cage":          {},
	"animaltrap":    {},
	"bin":           {},
	"crutch":        {},
	"splint":        {},
	"pipesection":   {},
	"minecart":      {},
	"stepladder":    {},
	"wheelbarrow":   {},
	"shield":        {},
	"buckler":       {},
	"trainingaxe":   {},
	"trainingspear": {},
	"trainingsword": {},
	"corkscrew":     {},
	"menacingspike": {},
	"spikedball":    {},
}

// itemMaterialNotApplicable lists items where the DFHack job_type doesn't
// take a material spec — the recipe has fixed inputs/outputs (e.g.
// MakeAsh burns wood and produces ash bars regardless of wood type).
// For these items the parser allows the material to be omitted, and
// silently ignores any material the chatter supplies; the service-side
// skips populating the workorder JSON's material field.
var itemMaterialNotApplicable = map[string]struct{}{
	"ash":      {},
	"charcoal": {},
}

// itemMaterialAllowlist restricts which materials are accepted for
// specific items where DF has a strict requirement that would otherwise
// queue an unbuildable order. Items not present here accept any material
// in materialVocab (the loose default — DF rejects the impossibility
// itself).
var itemMaterialAllowlist = map[string]map[string]struct{}{
	"bed":    {"wood": {}},
	"barrel": {"wood": {}},
}

// materialVocab is the set of chat-facing materials accepted in v0.
// "metal" is a meta-token meaning "any available metal, executor picks".
var materialVocab = map[string]struct{}{
	"stone": {}, "wood": {}, "bone": {}, "leather": {}, "cloth": {}, "shell": {},
	"iron": {}, "copper": {}, "bronze": {}, "steel": {}, "silver": {}, "gold": {},
	"metal": {},
}

// materialAliases normalizes adjectival forms to the canonical material
// token.
var materialAliases = map[string]string{
	"wooden": "wood",
}

// fillerWords are chat-tolerance tokens stripped before grammar matching.
// `drink` and `from` are stripped specifically so the verbose brew
// phrasing (`brew drink from fruit`) works in addition to the canonical
// short form (`brew fruit`). They're benign for other verbs — no
// existing grammar slot needs to preserve them as data.
var fillerWords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "some": {},
	"me": {}, "us": {}, "please": {},
	"drink": {}, "from": {},
}
