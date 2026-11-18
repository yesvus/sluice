package acp

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedConfigContract(t *testing.T) {
	p, err := decodeProfile(strings.NewReader(exampleProfile), "worker")
	if err != nil {
		t.Fatal(err)
	}
	b := p.Backends[1]
	dir := t.TempDir()
	_, expected, err := renderPinnedConfig(b)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "opencode.json")
	if err := os.WriteFile(path, expected, 0600); err != nil {
		t.Fatal(err)
	}
	readOnly := func(*os.File) bool { return true }
	if err := checkManagedConfig(dir, b, readOnly); err != nil {
		t.Fatalf("valid synthetic managed config rejected: %v", err)
	}
	if err := checkManagedConfig(dir, b, func(*os.File) bool { return false }); err == nil {
		t.Fatal("writable mount accepted")
	}
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkManagedConfig(dir, b, readOnly); err == nil {
		t.Fatal("wrong model config accepted")
	}
	if err := os.WriteFile(path, expected, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "opencode.jsonc"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkManagedConfig(dir, b, readOnly); err == nil {
		t.Fatal("alternate managed config accepted")
	}
}

func TestOpenCodeLauncherUnderSyntheticManagedContract(t *testing.T) {
	p, err := decodeProfile(strings.NewReader(exampleProfile), "worker")
	if err != nil {
		t.Fatal(err)
	}
	b := &p.Backends[1]
	b.Home = t.TempDir()
	if err := os.Chmod(b.Home, 0700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(binary, []byte("synthetic executable"), 0700); err != nil {
		t.Fatal(err)
	}
	b.Command = []string{binary, "acp"}
	t.Setenv("OPENCODE_CONFIG_CONTENT", "synthetic-override")
	t.Setenv("OPENCODE_CONFIG_DIR", "/synthetic/untrusted")
	t.Setenv("OPENAI_API_KEY", "synthetic-override")
	dir := t.TempDir()
	name, expected, err := renderPinnedConfig(*b)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), expected, 0600); err != nil {
		t.Fatal(err)
	}
	checked := false
	launch := launcherWithManagedCheck(p, t.TempDir(), func(_ context.Context, executable string, args, env []string, _ string) (io.ReadWriteCloser, error) {
		if !checked || executable != binary || len(args) != 1 || args[0] != "acp" {
			t.Fatal("managed config gate not applied before launch")
		}
		got := strings.Join(env, "\n")
		if strings.Contains(got, "synthetic-override") || strings.Contains(got, "/synthetic/untrusted") || !strings.Contains(got, "XDG_DATA_HOME="+filepath.Join(b.Home, ".local", "share")) {
			t.Fatal("unsafe or wrong OpenCode environment")
		}
		one, two := net.Pipe()
		_ = two.Close()
		return one, nil
	}, func(b BackendSpec) error {
		checked = true
		return checkManagedConfig(dir, b, func(*os.File) bool { return true })
	})
	child, err := launch(context.Background(), p.selection()[1])
	if err != nil {
		t.Fatal(err)
	}
	_ = child.Close()
}
