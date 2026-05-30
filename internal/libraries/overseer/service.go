package overseer

import (
	"fmt"
	"os/exec"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/verbs"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// NivekOverseerService submits parsed actions to DFHack. Lives
// executor-side (machine running DF + DFHack), not Pi-side.
type NivekOverseerService interface {
	Submit(action wire.Action) error
}

// nivekOverseerServiceImpl is the production implementation. Each
// verb's Submit logic lives in package verbs; this type exists to hold
// the dfhack-run path and to satisfy the verbs.Executor interface for
// the per-verb submitters.
type nivekOverseerServiceImpl struct {
	dfhackRunPath string
}

func NewService(dfhackRunPath string) NivekOverseerService {
	return &nivekOverseerServiceImpl{dfhackRunPath: dfhackRunPath}
}

// RunLua satisfies verbs.Executor by shelling to `dfhack-run lua` with
// the script. Combined stdout/stderr is included in error messages so
// chat-side logging surfaces the real DFHack failure.
func (s *nivekOverseerServiceImpl) RunLua(script string) error {
	out, err := exec.Command(s.dfhackRunPath, "lua", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("dfhack-run lua failed: %w: %s", err, string(out))
	}
	return nil
}

// RunDFHack satisfies verbs.Executor by shelling to `dfhack-run <args>`.
// Used by verbs that drive a DFHack subcommand (workorder for
// manufacture/brew) rather than raw lua.
func (s *nivekOverseerServiceImpl) RunDFHack(args ...string) error {
	out, err := exec.Command(s.dfhackRunPath, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("dfhack-run failed: %w: %s", err, string(out))
	}
	return nil
}

// Submit dispatches the action to the right per-verb submitter in
// package verbs. New verbs land here as one new case plus a new file in
// verbs/.
func (s *nivekOverseerServiceImpl) Submit(action wire.Action) error {
	switch action.Kind {
	case wire.ActionKindManufacture:
		return verbs.SubmitManufacture(s, action)
	case wire.ActionKindPause:
		return verbs.SubmitPause(s, action)
	case wire.ActionKindUnpause:
		return verbs.SubmitUnpause(s, action)
	case wire.ActionKindCamera:
		return verbs.SubmitCamera(s, action)
	case wire.ActionKindPlace:
		return verbs.SubmitPlace(s, action)
	case wire.ActionKindBrew:
		return verbs.SubmitBrew(s, action)
	case wire.ActionKindMine:
		return verbs.SubmitMine(s, action)
	case wire.ActionKindChannel:
		return verbs.SubmitChannel(s, action)
	case wire.ActionKindDigRamp:
		return verbs.SubmitDigRamp(s, action)
	case wire.ActionKindCutTree:
		return verbs.SubmitCutTree(s, action)
	case wire.ActionKindStockpile:
		return verbs.SubmitStockpile(s, action)
	case wire.ActionKindZone:
		return verbs.SubmitZone(s, action)
	case wire.ActionKindAppoint:
		return verbs.SubmitAppoint(s, action)
	case wire.ActionKindTaskat:
		return verbs.SubmitTaskat(s, action)
	default:
		return fmt.Errorf("unsupported action kind: %s", action.Kind)
	}
}
