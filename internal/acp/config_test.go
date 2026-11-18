package acp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWritePinnedConfigPrivateDirectories(t *testing.T) {
	p, err := decodeProfile(strings.NewReader(exampleProfile), "worker")
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range p.Backends {
		t.Run(b.ID, func(t *testing.T) {
			dir := t.TempDir()
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			// Fixed config name must be replaced, never followed as a symlink.
			name, want, err := renderPinnedConfig(b)
			if err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(dir, "synthetic-sentinel")
			if err := os.WriteFile(sentinel, []byte("unchanged"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("synthetic-sentinel", filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := writePinnedConfig(root, b); err != nil {
					t.Fatal(err)
				}
			}
			got, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil || string(got) != string(want) {
				t.Fatal("pinned config differs from renderer output")
			}
			info, err := os.Lstat(filepath.Join(dir, name))
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
				t.Fatal("config must be a private regular file")
			}
			got, err = os.ReadFile(sentinel)
			if err != nil || string(got) != "unchanged" {
				t.Fatal("symlink target modified")
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 2 {
				t.Fatal("temporary config not cleaned up")
			}
		})
	}
}
