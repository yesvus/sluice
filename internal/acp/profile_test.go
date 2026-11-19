package acp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const exampleProfile = `profiles:
  worker:
    backends:
      - id: synthetic-codex
        command: [codex-acp]
        home: /private/worker-codex
        model: example-a
        effort: high
        account_id: synthetic-first
      - id: synthetic-opencode
        command: [/opt/bin/opencode, acp]
        home: /private/worker-opencode
        config: managed
        model: example/example-b
        account_id: synthetic-second
    limit_patterns: [usage limit]
    limits:
      max_turns: 0
      max_minutes: 0
`

func TestDecodeProfile(t *testing.T) {
	p, err := decodeProfile(strings.NewReader(exampleProfile), "worker")
	if err != nil || len(p.Backends) != 2 || len(p.selection()) != 2 || p.selection()[1].Model != "example/example-b" {
		t.Fatalf("valid profile rejected: %v", err)
	}
	for _, tc := range []struct{ name, from, to string }{
		{"duplicate ID", "id: synthetic-opencode", "id: synthetic-codex"},
		{"duplicate account", "account_id: synthetic-second", "account_id: synthetic-first"},
		{"shell command", "command: [codex-acp]", "command: [sh, -c, example]"},
		{"relative home", "home: /private/worker-codex", "home: ../other"},
		{"overlapping homes", "home: /private/worker-opencode", "home: /private/worker-codex/subdir"},
		{"missing OpenCode home", "home: /private/worker-opencode", "home: \"\""},
		{"global config", "config: managed", "config: global"},
		{"extra arg", "command: [/opt/bin/opencode, acp]", "command: [/opt/bin/opencode, acp, --extra]"},
		{"unknown key", "effort: high", "effort: high\n        secret: synthetic"},
		{"active limit", "max_turns: 0", "max_turns: 3"},
		{"short quota pattern", "usage limit", "x"},
		{"model injection", "model: example-a", "model: 'example-a\"'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeProfile(strings.NewReader(strings.Replace(exampleProfile, tc.from, tc.to, 1)), "worker"); err == nil {
				t.Fatal("invalid profile accepted")
			}
		})
	}
	for _, input := range []string{exampleProfile + "\n---\nprofiles: {}\n", strings.Repeat(" ", maxConfigBytes+1), "profiles: {}"} {
		if _, err := decodeProfile(strings.NewReader(input), "worker"); err == nil {
			t.Fatal("invalid document accepted")
		}
	}
}

func TestRenderPinnedConfig(t *testing.T) {
	p, err := decodeProfile(strings.NewReader(exampleProfile), "worker")
	if err != nil {
		t.Fatal(err)
	}
	name, data, err := renderPinnedConfig(p.Backends[0])
	if err != nil || name != "config.toml" || string(data) != "model = \"example-a\"\nmodel_reasoning_effort = \"high\"\n" {
		t.Fatalf("unexpected Codex config: %s", data)
	}
	name, data, err = renderPinnedConfig(p.Backends[1])
	var settings map[string]string
	if err != nil || name != "opencode.json" || json.Unmarshal(data, &settings) != nil || len(settings) != 2 || settings["model"] != "example/example-b" || settings["small_model"] != "example/example-b" {
		t.Fatalf("unexpected OpenCode config: %s", data)
	}
}

func TestLoadProfileTrustedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.yaml")
	if err := os.WriteFile(path, []byte(exampleProfile), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProfile(path, "worker"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{path, filepath.Dir(path), "profiles.yaml"} {
		if _, err := LoadProfile(candidate, "worker"); err == nil {
			t.Fatal("untrusted profile file accepted")
		}
	}
}
