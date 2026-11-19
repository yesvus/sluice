package acp

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const trustedPath = "/opt/agent-tools:/usr/local/bin:/usr/bin:/bin"

type processStarter func(context.Context, string, []string, []string, string) (io.ReadWriteCloser, error)

// Serve runs a single approved profile over an ACP stream. OpenCode requires
// an operator-provisioned private runtime and verified read-only managed config.
func Serve(ctx context.Context, client io.ReadWriteCloser, name string, p Profile, workdir string, audit io.Writer) error {
	if !safeID.MatchString(name) || p.validate() != nil || !filepath.IsAbs(workdir) || audit == nil {
		return errProfile
	}
	return runRelay(ctx, client, name, p.selection(), p.LimitPatterns, newLauncher(p, workdir, startProcess), audit)
}

func newLauncher(p Profile, workdir string, start processStarter) dialBackend {
	return launcherWithManagedCheck(p, workdir, start, verifyManagedConfig)
}

func launcherWithManagedCheck(p Profile, workdir string, start processStarter, managedCheck func(BackendSpec) error) dialBackend {
	return func(ctx context.Context, selected Backend) (io.ReadWriteCloser, error) {
		for _, b := range p.Backends {
			if b.ID != selected.ID || b.Model != selected.Model {
				continue
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if filepath.Base(b.Command[0]) == "opencode" {
				if managedCheck(b) != nil || validatePrivateHome(b.Home) != nil {
					return nil, errIsolationRequired
				}
				executable, err := resolveExecutable(b.Command[0])
				if err != nil {
					return nil, err
				}
				return start(ctx, executable, b.Command[1:], opencodeEnvironment(b.Home), workdir)
			}
			executable, err := resolveExecutable(b.Command[0])
			if err != nil {
				return nil, err
			}
			if err := prepareCodex(b); err != nil {
				return nil, err
			}
			return start(ctx, executable, b.Command[1:], codexEnvironment(b.Home), workdir)
		}
		return nil, errBackendLaunch
	}
}

func resolveExecutable(name string) (string, error) {
	candidates := []string{name}
	if !filepath.IsAbs(name) {
		candidates = nil
		for _, dir := range strings.Split(trustedPath, ":") {
			candidates = append(candidates, filepath.Join(dir, name))
		}
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
			return candidate, nil
		}
	}
	return "", errBackendLaunch
}

func validatePrivateHome(home string) error {
	info, err := os.Lstat(home)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errPinnedConfigWrite
	}
	return nil
}

func prepareCodex(b BackendSpec) error {
	// The operator provisions the dedicated CLI home; no login store is opened.
	info, err := os.Lstat(b.Home)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errPinnedConfigWrite
	}
	root, err := os.OpenRoot(b.Home)
	if err != nil {
		return errPinnedConfigWrite
	}
	defer root.Close()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) || opened.Mode().Perm()&0077 != 0 {
		return errPinnedConfigWrite
	}
	return writePinnedConfig(root, b)
}

func codexEnvironment(home string) []string {
	// Do not inherit provider tokens, CODEX_CONFIG, MODEL_PROVIDER, shell
	// startup overrides or the parent PATH. File-based CLI login is provisioned
	// by the operator; other auth/environment combinations need separate proof.
	return []string{"PATH=" + trustedPath, "HOME=" + home, "CODEX_HOME=" + home, "LANG=C.UTF-8"}
}

func opencodeEnvironment(home string) []string {
	return []string{
		"PATH=" + trustedPath, "HOME=" + home, "LANG=C.UTF-8",
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
	}
}

func launchReason(err error) error {
	if errors.Is(err, errIsolationRequired) {
		return errIsolationRequired
	}
	return errBackendLaunch
}
