package verbs

// Executor is the runtime interface every verb's Submit function uses
// to actually talk to DFHack. It abstracts over how dfhack-run is invoked
// so the per-verb implementations don't depend on the concrete service
// type — which would otherwise force a circular import between
// overseer/ (where the service lives) and overseer/verbs/.
//
// The concrete implementation in package overseer wraps exec.Command on
// the configured dfhack-run path; tests can substitute a fake.
type Executor interface {
	// RunLua runs `dfhack-run lua <script>`. Combined stdout/stderr is
	// included in any returned error so chat-side logging surfaces the
	// real DFHack message (not just "exit code 1").
	RunLua(script string) error
	// RunDFHack runs `dfhack-run <args>`. Used by verbs that call into
	// DFHack subcommands rather than raw lua — e.g. the `workorder`
	// command for manufacture/brew.
	RunDFHack(args ...string) error
}
