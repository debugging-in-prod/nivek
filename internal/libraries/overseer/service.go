package overseer

import (
	"fmt"
	"os/exec"
)

// NivekOverseerService submits parsed actions to DFHack.
// Lives executor-side (machine running DF + DFHack), not Pi-side.
type NivekOverseerService interface {
	SubmitManufacture(action Action) error
}

type nivekOverseerServiceImpl struct {
	dfhackRunPath string
}

func NewService(dfhackRunPath string) NivekOverseerService {
	return &nivekOverseerServiceImpl{dfhackRunPath: dfhackRunPath}
}

// itemToJobType maps chat-facing item nouns to DFHack `job_type` enum names
// used by the `workorder` script. v0 covers two items; expand as the slice grows.
var itemToJobType = map[string]string{
	"table": "ConstructTable",
	"bed":   "ConstructBed",
}

func (s *nivekOverseerServiceImpl) SubmitManufacture(action Action) error {
	if action.Kind != ActionKindManufacture {
		return fmt.Errorf("unsupported action kind: %s", action.Kind)
	}
	jobType, ok := itemToJobType[action.Item]
	if !ok {
		return fmt.Errorf("no DFHack job_type mapping for item: %s", action.Item)
	}
	qty := action.Quantity
	if qty <= 0 {
		qty = 1
	}
	out, err := exec.Command(s.dfhackRunPath, "workorder", jobType, fmt.Sprintf("%d", qty)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("dfhack-run failed: %w: %s", err, string(out))
	}
	return nil
}
