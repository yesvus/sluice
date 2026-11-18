package acp

import (
	"bytes"
	"io"
	"os"
)

const managedDirectory = "/etc/opencode"

// The fixed Linux path is resolved by OpenCode 2.3.0, including in its
// standalone server subprocess. The operator must mount a profile-specific
// read-only config there in a dedicated runtime; sluice never writes to it.
func verifyManagedConfig(b BackendSpec) error {
	return checkManagedConfig(managedDirectory, b, readOnlyMount)
}

func checkManagedConfig(dir string, b BackendSpec, readOnly func(*os.File) bool) error {
	name, expected, err := renderPinnedConfig(b)
	if err != nil || name != "opencode.json" {
		return errIsolationRequired
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return errIsolationRequired
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return errIsolationRequired
	}
	defer root.Close()
	mount, err := root.Open(".")
	if err != nil {
		return errIsolationRequired
	}
	defer mount.Close()
	opened, err := mount.Stat()
	if err != nil || !os.SameFile(info, opened) || !readOnly(mount) {
		return errIsolationRequired
	}
	// Discovery loads opencode.jsonc as well; forbid an alternate managed file.
	if _, err := root.Lstat("opencode.jsonc"); !os.IsNotExist(err) {
		return errIsolationRequired
	}
	regular, err := root.Stat(name)
	if err != nil || !regular.Mode().IsRegular() || regular.Size() != int64(len(expected)) {
		return errIsolationRequired
	}
	file, err := root.Open(name)
	if err != nil {
		return errIsolationRequired
	}
	defer file.Close()
	actual, err := file.Stat()
	if err != nil || !os.SameFile(regular, actual) || !readOnly(file) {
		return errIsolationRequired
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(len(expected)+1)))
	if err != nil || !bytes.Equal(data, expected) {
		return errIsolationRequired
	}
	return nil
}
