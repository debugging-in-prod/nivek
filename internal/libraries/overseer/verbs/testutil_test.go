package verbs

import (
	"strings"
	"testing"
)

// fakeExecutor is a verbs.Executor that records every RunLua / RunDFHack
// call instead of shelling out. Tests construct one, hand it to the
// SubmitXxx function, then inspect what was captured.
type fakeExecutor struct {
	LuaCalls    []string
	DFHackCalls [][]string

	// LuaErr and DFHackErr, when non-nil, are returned by the next call
	// of the matching method. Lets tests exercise the error-propagation
	// path without a real DFHack failure.
	LuaErr    error
	DFHackErr error
}

func (f *fakeExecutor) RunLua(script string) error {
	f.LuaCalls = append(f.LuaCalls, script)
	return f.LuaErr
}

func (f *fakeExecutor) RunDFHack(args ...string) error {
	// copy so callers can't mutate our record via the slice header
	cp := make([]string, len(args))
	copy(cp, args)
	f.DFHackCalls = append(f.DFHackCalls, cp)
	return f.DFHackErr
}

// lastLua returns the script of the most recent RunLua call, or fails
// the test if nothing was captured.
func (f *fakeExecutor) lastLua(t *testing.T) string {
	t.Helper()
	if len(f.LuaCalls) == 0 {
		t.Fatalf("expected at least one RunLua call, got none")
	}
	return f.LuaCalls[len(f.LuaCalls)-1]
}

// assertContains fails the test if haystack does not contain needle.
// Used to spot-check lua snippets without locking tests to byte-exact
// strings — the substantive bits (position, enum names, etc.) get
// checked; whitespace and comment changes don't.
func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected output to contain %q\n--- output ---\n%s", needle, haystack)
	}
}

// assertContainsAll runs assertContains over a list of substrings.
func assertContainsAll(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		assertContains(t, haystack, n)
	}
}
