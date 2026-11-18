package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validEntry = `  - id: fixture-a
    provider: synthetic
    onwatch_key: "101"
    kind: api_key
    tier: cheap
    deployments: [fixture-dep-a]
    threshold: 0.8
`

func writeConfig(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "accounts.yaml")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadValidation(t *testing.T) {
	second := strings.NewReplacer("fixture-a", "fixture-b", "101", "102", "fixture-dep-a", "fixture-dep-b").Replace(validEntry)
	for _, tc := range []struct {
		name string
		yaml string
		want string
	}{
		{"empty id", strings.Replace(validEntry, "fixture-a", "\"\"", 1), "accounts[0]: id"},
		{"empty provider", strings.Replace(validEntry, "synthetic", "\"\"", 1), "accounts[0]: provider"},
		{"unknown kind", strings.Replace(validEntry, "api_key", "invalid", 1), "accounts[0]: unknown kind"},
		{"unknown tier", strings.Replace(validEntry, "cheap", "invalid", 1), "accounts[0]: unknown tier"},
		{"subscription routes", strings.Replace(validEntry, "api_key", "subscription_observed", 1), "accounts[0]: subscription_observed"},
		{"cross tier", strings.Replace(validEntry, "api_key", "free_tier", 1), "accounts[0]: free_tier"},
		{"duplicate id", validEntry + strings.Replace(second, "fixture-b", "fixture-a", 1), "accounts[1]: duplicate id"},
		{"duplicate provider key", validEntry + strings.Replace(second, "102", "101", 1), "accounts[1]: duplicate (provider, onwatch_key)"},
		{"duplicate deployment", validEntry + strings.Replace(second, "fixture-dep-b", "fixture-dep-a", 1), "accounts[1]: duplicate deployment"},
		{"duplicate same account deployment", strings.Replace(validEntry, "[fixture-dep-a]", "[fixture-dep-a, fixture-dep-a]", 1), "accounts[0]: duplicate deployment"},
		{"threshold zero", strings.Replace(validEntry, "0.8", "0", 1), "accounts[0]: threshold"},
		{"threshold negative", strings.Replace(validEntry, "0.8", "-0.1", 1), "accounts[0]: threshold"},
		{"threshold above one", strings.Replace(validEntry, "0.8", "1.01", 1), "accounts[0]: threshold"},
		{"threshold nan", strings.Replace(validEntry, "0.8", ".nan", 1), "accounts[0]: threshold"},
		{"threshold inf", strings.Replace(validEntry, "0.8", ".inf", 1), "accounts[0]: threshold"},
		{"unknown field", validEntry + "    secret: fixture-not-a-secret\n", "unknown configuration field"},
		{"multi document", validEntry + "---\naccounts: []\n", "exactly one YAML document"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Load(writeConfig(t, "accounts:\n"+tc.yaml))
			if r != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q; registry=%v err=%v", tc.want, r, err)
			}
			if strings.Contains(err.Error(), "fixture-not-a-secret") {
				t.Fatal("raw YAML value leaked")
			}
		})
	}
}

func TestLoadExample(t *testing.T) {
	r, err := Load("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Accounts()) != 3 {
		t.Fatal("example account set differs")
	}
	a, ok := r.Lookup("codex", "101")
	if !ok || a.Kind != KindSubscriptionObserved || len(a.Deployments) != 0 || a.Threshold != 0 {
		t.Fatal("display-only account or omitted threshold not preserved")
	}
	a, ok = r.Lookup("openrouter", "")
	if !ok || a.Kind != KindFreeTier || a.Tier != TierFree || a.Threshold != 0.9 {
		t.Fatal("free fixture invalid")
	}
}

func TestThresholdOne(t *testing.T) {
	r, err := Load(writeConfig(t, "accounts:\n"+strings.Replace(validEntry, "0.8", "1", 1)))
	if err != nil {
		t.Fatal(err)
	}
	a, ok := r.Lookup("synthetic", "101")
	if !ok || a.Threshold != 1 {
		t.Fatal("threshold 1 not preserved")
	}
}

func TestBadConfig(t *testing.T) {
	for _, data := range []string{"", "accounts: null", "accounts: [", "accounts: "+strings.Repeat(" ", maxRegistryBytes)} {
		if r, err := Load(writeConfig(t, data)); r != nil || err == nil {
			t.Fatal("invalid config accepted")
		}
	}
	if r, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); r != nil || err == nil {
		t.Fatal("missing file accepted")
	}
}
