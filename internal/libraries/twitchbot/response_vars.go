package twitchbot

import (
	"math/rand/v2"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Response variables for custom command templates (issue #158).
//
// A custom command's response_tmpl may contain ${...} variables that are
// expanded once, left to right, when the command fires:
//
//	${user}                  chatter's display name
//	${touser}                first argument with leading @ stripped (falls back to ${user})
//	${1} .. ${9}             command arguments (empty when missing)
//	${channel}               channel display name
//	${random.pick 'a' "b"}   one of the quoted options, picked at random
//	${random.1-100}          random integer in the inclusive range (bounded)
//	${count}                 per-channel use counter for this command
//
// Unknown or malformed variables are left in the output literally. Substituted
// text is written straight to the output and never re-scanned, so a chatter
// passing "${count}" as an argument can't trigger another expansion.

const (
	// maxChatReplyRunes caps every expanded custom-command reply.
	maxChatReplyRunes = 450
	// maxExpandedBytes stops building the output early so a template that
	// repeats a large argument many times can't allocate unbounded memory. It's
	// comfortably above maxChatReplyRunes*4 (the most bytes 450 runes can take).
	maxExpandedBytes = 4096
	// maxVarTokenBytes bounds how far ahead we look for a closing '}'.
	maxVarTokenBytes = 512
	// maxRandomBound bounds both ends of ${random.A-B}.
	maxRandomBound = 1_000_000_000
	// maxTemplateArgs is how many positional args (${1}..${9}) are addressable.
	maxTemplateArgs = 9
)

// responseVars is the per-invocation input to expandResponse.
type responseVars struct {
	User    string   // chatter display name
	Channel string   // channel display name
	Args    []string // command arguments, trigger excluded

	// Count returns the command's new use count. It's called lazily -- at most
	// once per expansion, and only if the template uses ${count}. ok=false
	// (e.g. the DB call failed) leaves ${count} literal. nil behaves like
	// ok=false.
	Count func() (n int, ok bool)

	// IntN returns a random int in [0, n). nil uses math/rand/v2.
	IntN func(n int) int
}

// expandResponse expands the template's variables. It's a pure function of its
// inputs (randomness and the counter come in through vars). The result is not
// yet sanitized for chat -- pass it through sanitizeChatReply before sending.
func expandResponse(tmpl string, vars responseVars) string {
	intN := vars.IntN
	if intN == nil {
		intN = rand.IntN
	}

	countDone := false
	countVal, countOK := 0, false
	count := func() (int, bool) {
		if !countDone {
			countDone = true
			if vars.Count != nil {
				countVal, countOK = vars.Count()
			}
		}
		return countVal, countOK
	}

	var out strings.Builder
	i := 0
	for i < len(tmpl) && out.Len() < maxExpandedBytes {
		start := strings.Index(tmpl[i:], "${")
		if start < 0 {
			out.WriteString(tmpl[i:])
			break
		}
		start += i
		out.WriteString(tmpl[i:start])

		end := findVarEnd(tmpl, start+2)
		if end < 0 {
			// No closing brace: emit the '$' and keep scanning after it so a
			// later well-formed variable still expands.
			out.WriteByte('$')
			i = start + 1
			continue
		}

		if val, ok := resolveVar(tmpl[start+2:end], vars, intN, count); ok {
			out.WriteString(val)
			i = end + 1
			continue
		}
		// Unknown variable: leave it literal. Emit only the '$' and rescan
		// from the '{' so a nested valid variable ("${x ${user}}") still works.
		out.WriteByte('$')
		i = start + 1
	}

	s := out.String()
	if len(s) > maxExpandedBytes {
		// A rune split by this cut is replaced by sanitizeChatReply.
		s = s[:maxExpandedBytes]
	}
	return s
}

// findVarEnd returns the index of the '}' that closes a variable whose body
// starts at from, or -1. Braces inside single/double quotes don't count (so
// ${random.pick '}' 'x'} works). The search is bounded by maxVarTokenBytes.
func findVarEnd(s string, from int) int {
	limit := min(len(s), from+maxVarTokenBytes)
	var quote byte
	for j := from; j < limit; j++ {
		c := s[j]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '}':
			return j
		}
	}
	return -1
}

// resolveVar maps one variable body (the text between "${" and "}") to its
// value. ok=false means "unknown or malformed: leave it literal".
func resolveVar(body string, vars responseVars, intN func(int) int, count func() (int, bool)) (string, bool) {
	switch body {
	case "user":
		return vars.User, true
	case "channel":
		return vars.Channel, true
	case "touser":
		if len(vars.Args) > 0 {
			if t := strings.TrimLeft(vars.Args[0], "@"); t != "" {
				return t, true
			}
		}
		return vars.User, true
	case "count":
		n, ok := count()
		if !ok {
			return "", false
		}
		return strconv.Itoa(n), true
	}

	if len(body) == 1 && body[0] >= '1' && body[0] <= '9' {
		idx := int(body[0] - '1')
		if idx < len(vars.Args) {
			return vars.Args[idx], true
		}
		return "", true
	}

	if rest, ok := strings.CutPrefix(body, "random.pick"); ok {
		opts, ok := parsePickOptions(rest)
		if !ok {
			return "", false
		}
		return opts[intN(len(opts))], true
	}

	if rest, ok := strings.CutPrefix(body, "random."); ok {
		lo, hi, ok := parseRandomRange(rest)
		if !ok {
			return "", false
		}
		return strconv.Itoa(lo + intN(hi-lo+1)), true
	}

	return "", false
}

// parsePickOptions parses the argument list of ${random.pick ...}: one or more
// whitespace-separated, single- or double-quoted strings. Anything unquoted, an
// unterminated quote, or an empty list is malformed.
func parsePickOptions(s string) ([]string, bool) {
	// The option list must be separated from "random.pick" by whitespace.
	if s == "" || !isSpaceByte(s[0]) {
		return nil, false
	}
	var opts []string
	i := 0
	for {
		for i < len(s) && isSpaceByte(s[i]) {
			i++
		}
		if i == len(s) {
			break
		}
		q := s[i]
		if q != '\'' && q != '"' {
			return nil, false
		}
		closeIdx := strings.IndexByte(s[i+1:], q)
		if closeIdx < 0 {
			return nil, false
		}
		opts = append(opts, s[i+1:i+1+closeIdx])
		i += closeIdx + 2
		// Options must be whitespace-separated ('a''b' is malformed).
		if i < len(s) && !isSpaceByte(s[i]) {
			return nil, false
		}
	}
	if len(opts) == 0 {
		return nil, false
	}
	return opts, true
}

// parseRandomRange parses "A-B" (non-negative decimal integers, A <= B, both
// at most maxRandomBound).
func parseRandomRange(s string) (lo, hi int, ok bool) {
	a, b, found := strings.Cut(s, "-")
	if !found {
		return 0, 0, false
	}
	lo, ok1 := parseBoundedUint(a)
	hi, ok2 := parseBoundedUint(b)
	if !ok1 || !ok2 || lo > hi {
		return 0, 0, false
	}
	return lo, hi, true
}

func parseBoundedUint(s string) (int, bool) {
	if s == "" || len(s) > 10 {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil || n > maxRandomBound {
		return 0, false
	}
	return n, true
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t'
}

// commandArgs returns the whitespace-separated words that follow the first
// occurrence of trigger (case-insensitive) in text, capped at maxTemplateArgs.
// Case is preserved. nil when the trigger isn't found or has no arguments.
func commandArgs(text, trigger string) []string {
	words := strings.Fields(text)
	for i, w := range words {
		if strings.EqualFold(w, trigger) {
			args := words[i+1:]
			if len(args) > maxTemplateArgs {
				args = args[:maxTemplateArgs]
			}
			if len(args) == 0 {
				return nil
			}
			return args
		}
	}
	return nil
}

// sanitizeChatReply makes text safe to post as a chat reply: control
// characters (including newlines) become spaces, leading whitespace and any
// leading '/' or '.' are stripped (so the reply can never be read as a Twitch
// chat command such as "/ban" or ".timeout"), and the result is capped at
// maxChatReplyRunes. An empty result means "send nothing".
func sanitizeChatReply(text string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == utf8.RuneError || unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)

	cleaned = strings.TrimLeftFunc(cleaned, func(r rune) bool {
		return r == '/' || r == '.' || unicode.IsSpace(r)
	})

	if utf8.RuneCountInString(cleaned) > maxChatReplyRunes {
		runes := []rune(cleaned)
		cleaned = string(runes[:maxChatReplyRunes])
	}
	return strings.TrimRightFunc(cleaned, unicode.IsSpace)
}
