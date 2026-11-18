package registry

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		{"whitespace id", strings.Replace(validEntry, "fixture-a", "\" fixture-a\"", 1), "accounts[0]: id"},
		{"whitespace provider", strings.Replace(validEntry, "synthetic", "\"synthetic \"", 1), "accounts[0]: provider"},
		{"whitespace key", strings.Replace(validEntry, "\"101\"", "\" 101\"", 1), "accounts[0]: onwatch_key"},
		{"whitespace deployment", strings.Replace(validEntry, "[fixture-dep-a]", "[\"fixture-dep-a \"]", 1), "accounts[0]: deployment id"},
		{"empty deployment", strings.Replace(validEntry, "[fixture-dep-a]", "[\"\"]", 1), "accounts[0]: deployment id"},
		{"null deployment", strings.Replace(validEntry, "[fixture-dep-a]", "[null]", 1), "accounts[0]: deployment id"},
		{"no api deployments", strings.Replace(validEntry, "    deployments: [fixture-dep-a]\n", "", 1), "accounts[0]: routable account needs deployments"},
		{"no free deployments", strings.NewReplacer("api_key", "free_tier", "cheap", "free", "[fixture-dep-a]", "[]").Replace(validEntry), "accounts[0]: routable account needs deployments"},
		{"null account", "  - null\n" + validEntry, "accounts[0]: expected account mapping"},
		{"second null account", validEntry + "  - null\n", "accounts[1]: expected account mapping"},
		{"wrong scalar type", validEntry + strings.Replace(second, "fixture-b", "[fixture-b]", 1), "accounts[1]: account field must be a string"},
		{"threshold null", strings.Replace(validEntry, "0.8", "null", 1), "accounts[0]: threshold"},
		{"duplicate field", validEntry + "    id: fixture-b\n", "accounts[0]: duplicate account field"},
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
		{"unknown field", validEntry + "    secret: fixture-not-a-secret\n", "accounts[0]: unknown configuration field"},
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
	for _, data := range []string{"", "accounts: null", "accounts: ["} {
		if r, err := Load(writeConfig(t, data)); r != nil || err == nil {
			t.Fatal("invalid config accepted")
		}
	}
	oversized := "accounts:\n" + strings.Replace(validEntry, "fixture-dep-a", strings.Repeat("x", maxRegistryBytes), 1)
	if r, err := Load(writeConfig(t, oversized)); r != nil || err == nil || !strings.Contains(err.Error(), "size exceeds limit") {
		t.Fatal("oversized valid configuration not rejected by size bound")
	}
	if r, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); r != nil || err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestEmptyAccounts(t *testing.T) {
	r, err := Load(writeConfig(t, "accounts: []"))
	if err != nil || r == nil || len(r.Accounts()) != 0 {
		t.Fatalf("empty accounts should load: %v", err)
	}
}

func TestRejectFIFO(t *testing.T) {
	if _, err := exec.LookPath("mkfifo"); err != nil {
		t.Skip("mkfifo is unavailable")
	}
	path := filepath.Join(t.TempDir(), "pipe")
	if err := exec.Command("mkfifo", path).Run(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := Load(path)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "regular configuration file") {
			t.Fatalf("FIFO not rejected: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Load blocked on FIFO")
	}
}
