package acp

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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

var errPinnedConfigWrite = errors.New("cannot prepare pinned backend configuration")

// writePinnedConfig atomically replaces only the owned config file in a
// caller-provisioned private directory. It never reads existing files.
// The caller must establish exclusive config ownership and an isolated
// managed configuration mount before passing a root; this is not isolation.
func writePinnedConfig(root *os.Root, b BackendSpec) error {
	if root == nil {
		return errPinnedConfigWrite
	}
	name, data, err := renderPinnedConfig(b)
	if err != nil {
		return err
	}
	temp := ".sluice-config-" + rand.Text()
	file, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errPinnedConfigWrite
	}
	defer root.Remove(temp)
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errPinnedConfigWrite
	}
	if err := root.Rename(temp, name); err != nil {
		return errPinnedConfigWrite
	}
	return nil
}
