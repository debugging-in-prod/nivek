package twitchbot

import (
	"fmt"
	"log"
	"maps"
	"strings"
	"time"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/api"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
)

// Global builtin command loading (incident #172).
//
// The trigger -> handler map comes from core-api GET /commands. That call used
// to be a hard boot requirement: if core-api was down when the bot started,
// NewBot panicked and the bot crash-looped. Now:
//
//  1. At boot, retry with bounded exponential backoff (bootCommandBackoff,
//     ~2 min total), logging every attempt.
//  2. If every boot attempt fails, start anyway with an empty builtin map (no
//     builtin replies, so nothing can spam) and keep retrying in a background
//     goroutine until it succeeds, then install the map atomically.
//
// The installed set is immutable once published and swapped through an
// atomic.Pointer, so the dispatch worker pool reads it lock-free and never
// observes a half-built map.

// commandSet is one immutable snapshot of the global builtin commands. Never
// mutate a published set; build a new one and swap it in with setCommands.
type commandSet struct {
	handlers     map[string]commandHandler
	cooldownSecs map[string]int
}

// bootCommandBackoff is the wait before each boot-time retry after the first
// attempt: 1+2+4+8+16+30+30+30 = 121s worst case before the bot starts with an
// empty builtin map. Package vars so tests can shrink them.
var (
	bootCommandBackoff         = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second}
	backgroundCommandRetryBase = 1 * time.Second
	backgroundCommandRetryMax  = 30 * time.Second
	commandRetrySleep          = time.Sleep
)

// lookupCommand returns the handler for a global builtin trigger, if loaded.
func (b *Bot) lookupCommand(trigger string) (commandHandler, bool) {
	set := b.commands.Load()
	if set == nil {
		return nil, false
	}
	h, ok := set.handlers[trigger]
	return h, ok
}

// commandsLoaded reports whether a global command set has been installed.
func (b *Bot) commandsLoaded() bool {
	return b.commands.Load() != nil
}

// setCommands publishes a new global command set. Inputs are copied so the
// caller can't mutate a published snapshot.
func (b *Bot) setCommands(handlers map[string]commandHandler, cooldownSecs map[string]int) {
	set := &commandSet{
		handlers:     make(map[string]commandHandler, len(handlers)),
		cooldownSecs: make(map[string]int, len(cooldownSecs)),
	}
	maps.Copy(set.handlers, handlers)
	maps.Copy(set.cooldownSecs, cooldownSecs)
	b.commands.Store(set)
}

// fetchGlobalCommands makes one attempt at loading the global builtin commands.
// A panic inside the client is converted to an error so a retry loop survives.
func fetchGlobalCommands(coreAPI api.CoreAPIClient) (handlers map[string]commandHandler, cooldowns map[string]int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic fetching commands: %v", r)
		}
	}()
	if coreAPI == nil {
		return nil, nil, fmt.Errorf("no core-api client")
	}
	return getGlobalEnabledCommands(coreAPI)
}

// loadCommandsAtBoot tries to install the global commands, retrying with
// bootCommandBackoff. It returns true once installed; false means every boot
// attempt failed and the caller should start the background retry.
func (b *Bot) loadCommandsAtBoot() bool {
	attempts := len(bootCommandBackoff) + 1
	for i := 0; i < attempts; i++ {
		handlers, cooldowns, err := fetchGlobalCommands(b.coreAPI)
		if err == nil {
			b.setCommands(handlers, cooldowns)
			log.Printf("[COMMANDS] loaded %d global builtin commands (attempt %d/%d)", len(handlers), i+1, attempts)
			return true
		}
		if i == attempts-1 {
			log.Printf("[COMMANDS] ERROR: unable to load commands (attempt %d/%d): %s", i+1, attempts, err.Error())
			break
		}
		wait := bootCommandBackoff[i]
		log.Printf("[COMMANDS] ERROR: unable to load commands (attempt %d/%d): %s; retrying in %s", i+1, attempts, err.Error(), wait)
		commandRetrySleep(wait)
	}
	log.Printf("[COMMANDS] ERROR: starting with NO builtin commands; retrying core-api GET /commands in the background")
	return false
}

// retryCommandsInBackground keeps retrying until the global commands load,
// then installs them and returns. Each attempt is panic-isolated by
// fetchGlobalCommands; the loop itself is also guarded so a bug here can never
// take the process down.
func (b *Bot) retryCommandsInBackground() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[COMMANDS] ERROR: panic in background command loader: %v", r)
		}
	}()
	wait := backgroundCommandRetryBase
	for attempt := 1; ; attempt++ {
		commandRetrySleep(wait)
		handlers, cooldowns, err := fetchGlobalCommands(b.coreAPI)
		if err == nil {
			b.setCommands(handlers, cooldowns)
			log.Printf("[COMMANDS] loaded %d global builtin commands (background attempt %d)", len(handlers), attempt)
			return
		}
		wait = min(wait*2, backgroundCommandRetryMax)
		log.Printf("[COMMANDS] ERROR: background load failed (attempt %d): %s; retrying in %s", attempt, err.Error(), wait)
	}
}

// getGlobalEnabledCommands builds the dispatch map (trigger -> handler) by
// joining the command rows from core-api against builtinRegistry: the DB
// supplies trigger -> handler_key, the registry supplies handler_key -> func.
// Custom commands (no compiled handler) and disabled commands are skipped.
// Invalid rows (unknown/null handler_key, mixed-case trigger) are logged loudly
// and skipped rather than failing the whole load, so one bad seed row can't
// crash or silence the bot.
//
// It also returns each builtin trigger's cooldown_secs for the central
// cooldown guard. Only a failed fetch is an error.
func getGlobalEnabledCommands(coreAPI api.CoreAPIClient) (map[string]commandHandler, map[string]int, error) {
	rows, err := coreAPI.GetGlobalEnabledCommands()
	if err != nil {
		return nil, nil, err
	}
	cmds, rowErrs := buildCommandMaps(rows)
	for _, e := range rowErrs {
		log.Printf("[COMMANDS] ERROR: skipping invalid command row: %s", e.Error())
	}
	return cmds, buildCommandCooldowns(rows), nil
}

// buildCommandCooldowns maps each enabled builtin trigger to its configured
// cooldown_secs. Only positive values are kept; anything else falls back to the
// guard's defaults.
func buildCommandCooldowns(rows []commands.Commands) map[string]int {
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		if row.Kind != "builtin" || !row.Enabled || row.CooldownSecs <= 0 {
			continue
		}
		out[strings.ToLower(row.Trigger)] = row.CooldownSecs
	}
	return out
}

// buildCommandMaps turns command rows into the trigger -> handler dispatch map.
// Each invalid row is reported in rowErrs and left out of the map; valid rows
// are still returned.
func buildCommandMaps(rows []commands.Commands) (cmds map[string]commandHandler, rowErrs []error) {
	cmds = make(map[string]commandHandler, len(rows))
	for _, row := range rows {
		if row.Kind != "builtin" || !row.Enabled {
			continue
		}
		if row.HandlerKey == nil {
			rowErrs = append(rowErrs, fmt.Errorf("builtin command %q has null handler_key", row.Trigger))
			continue
		}
		handler, ok := builtinRegistry[*row.HandlerKey]
		if !ok {
			rowErrs = append(rowErrs, fmt.Errorf("command %q: unknown handler_key %q", row.Trigger, *row.HandlerKey))
			continue
		}
		// A trigger stored with any upper case would never match: the incoming
		// message is lowercased before the dispatch lookup, but this map is
		// keyed on the column verbatim. Report it rather than ship a command
		// that silently never fires.
		if row.Trigger != strings.ToLower(row.Trigger) {
			rowErrs = append(rowErrs, fmt.Errorf("command %q: trigger must be lowercase or it can never match", row.Trigger))
			continue
		}
		cmds[row.Trigger] = handler
	}
	return cmds, rowErrs
}
