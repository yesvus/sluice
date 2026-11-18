package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestACPOptions(t *testing.T) {
	t.Setenv("SLUICE_CONFIG", "/synthetic/env.yaml")
	opts, err := parseACPOptions([]string{"--profile", "worker"}, io.Discard)
	if err != nil || opts.profile != "worker" || opts.config != "/synthetic/env.yaml" {
		t.Fatalf("unexpected default: %+v, %v", opts, err)
	}
	opts, err = parseACPOptions([]string{"--profile", "worker", "--config", "/synthetic/flag.yaml"}, io.Discard)
	if err != nil || opts.config != "/synthetic/flag.yaml" {
		t.Fatalf("flag did not override env: %+v, %v", opts, err)
	}
	for _, args := range [][]string{nil, {"--profile", "worker", "extra"}, {"--profile", "worker", "--config", ""}, {"--profile", "worker", "--config", "config.local.yaml"}, {"--profile", "worker", "--config", "/synthetic/../profile.yaml"}} {
		if _, err := parseACPOptions(args, io.Discard); err == nil {
			t.Fatal("invalid arguments accepted")
		}
	}
}

func TestACPConfigOutsideWorkspace(t *testing.T) {
	t.Setenv("SLUICE_CONFIG", "")
	opts, err := parseACPOptions([]string{"--profile", "worker"}, io.Discard)
	if err != nil || opts.config != defaultACPConfigPath {
		t.Fatal("ACP default must be operator path")
	}
	workspace, outside := t.TempDir(), t.TempDir()
	external := filepath.Join(outside, "profiles.yaml")
	internal := filepath.Join(workspace, "profiles.yaml")
	for _, path := range []string{external, internal} {
		if err := os.WriteFile(path, []byte("synthetic"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(outside, "link.yaml")
	if err := os.Symlink(internal, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{internal, link} {
		if _, err := resolveACPConfig(path, workspace); err == nil {
			t.Fatal("workspace-controlled config accepted")
		}
	}
	if path, err := resolveACPConfig(external, workspace); err != nil || path == "" {
		t.Fatalf("operator config rejected: %v", err)
	}
}
