package acp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

func renderPinnedConfig(b BackendSpec) (string, []byte, error) {
	if !safeModel.MatchString(b.Model) || len(b.Command) == 0 {
		return "", nil, errProfile
	}
	switch filepath.Base(b.Command[0]) {
	case "codex-acp":
		if b.Effort != "high" {
			return "", nil, errProfile
		}
		return "config.toml", []byte(fmt.Sprintf("model = %q\nmodel_reasoning_effort = %q\n", b.Model, b.Effort)), nil
	case "opencode":
		if b.Config != "managed" {
			return "", nil, errProfile
		}
		// v2.3 migrates small_model to the title agent. Specify both as
		// legacy keys in the final managed document, never a shared global.
		data, err := json.MarshalIndent(map[string]string{"model": b.Model, "small_model": b.Model}, "", "  ")
		return "opencode.json", append(data, '\n'), err
	default:
		return "", nil, errProfile
	}
}
