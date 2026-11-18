package registry

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxRegistryBytes = 1 << 20

type configAccount struct {
	ID          string   `yaml:"id"`
	Provider    string   `yaml:"provider"`
	OnwatchKey  string   `yaml:"onwatch_key"`
	Kind        Kind     `yaml:"kind"`
	Tier        Tier     `yaml:"tier"`
	Deployments []string `yaml:"deployments"`
	Threshold   *float64 `yaml:"threshold"`
}

// Load reads non-secret account configuration. Invalid input never produces
// a partially populated registry.
func Load(path string) (*Registry, error) {
	// The path is operator-controlled; reject special files before opening.
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("registry: expected a regular configuration file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("registry: cannot open configuration")
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("registry: expected a regular configuration file")
	}
	return decode(f)
}

func decode(r io.Reader) (*Registry, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxRegistryBytes+1))
	if err != nil || len(data) > maxRegistryBytes {
		return nil, errors.New("registry: cannot read configuration or size exceeds limit")
	}
	var doc yaml.Node
	d := yaml.NewDecoder(bytes.NewReader(data))
	if err := d.Decode(&doc); err != nil {
		// Parser errors may echo accidentally supplied credentials.
		return nil, errors.New("registry: invalid YAML")
	}
	var extra yaml.Node
	if d.Decode(&extra) != io.EOF {
		return nil, errors.New("registry: expected exactly one YAML document")
	}
	if len(doc.Content) != 1 {
		return nil, errors.New("registry: missing accounts list")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode || len(root.Content) != 2 ||
		root.Content[0].Kind != yaml.ScalarNode || root.Content[0].Value != "accounts" ||
		root.Content[1].Kind != yaml.SequenceNode {
		return nil, errors.New("registry: expected a single accounts list")
	}
	entries := root.Content[1].Content
	type sourceKey struct {
		provider, key string
	}
	ids := map[string]bool{}
	keys := map[sourceKey]bool{}
	deployments := map[string]bool{}
	out := make([]Account, 0, len(entries))
	for i, entry := range entries {
		invalid := func(reason string) (*Registry, error) {
			return nil, fmt.Errorf("registry: accounts[%d]: %s", i, reason)
		}
		a, reason := decodeAccount(entry)
		if reason != "" {
			return invalid(reason)
		}
		if a.ID == "" || strings.TrimSpace(a.ID) != a.ID {
			return invalid("id must be nonempty without surrounding whitespace")
		}
		if a.Provider == "" || strings.TrimSpace(a.Provider) != a.Provider {
			return invalid("provider must be nonempty without surrounding whitespace")
		}
		if ids[a.ID] {
			return invalid("duplicate id")
		}
		if strings.TrimSpace(a.OnwatchKey) != a.OnwatchKey {
			return invalid("onwatch_key has surrounding whitespace")
		}
		k := sourceKey{a.Provider, a.OnwatchKey}
		if keys[k] {
			return invalid("duplicate (provider, onwatch_key)")
		}
		switch a.Kind {
		case KindAPIKey, KindFreeTier, KindSubscriptionObserved:
		default:
			return invalid("unknown kind")
		}
		switch a.Tier {
		case TierFree, TierCheap, TierFrontier:
		default:
			return invalid("unknown tier")
		}
		if a.Kind == KindSubscriptionObserved && len(a.Deployments) != 0 {
			return invalid("subscription_observed must not have deployments")
		}
		if a.Kind != KindSubscriptionObserved && len(a.Deployments) == 0 {
			return invalid("routable account needs deployments")
		}
		if a.Kind == KindFreeTier && a.Tier != TierFree {
			return invalid("free_tier account must have free tier")
		}
		threshold := 0.0
		if a.Threshold != nil {
			threshold = *a.Threshold
			if threshold <= 0 || threshold > 1 || math.IsNaN(threshold) || math.IsInf(threshold, 0) {
				return invalid("threshold must be in (0,1]")
			}
		}
		for _, deployment := range a.Deployments {
			if deployment == "" || strings.TrimSpace(deployment) != deployment {
				return invalid("deployment id must be nonempty without surrounding whitespace")
			}
			if deployments[deployment] {
				return invalid("duplicate deployment")
			}
			deployments[deployment] = true
		}
		ids[a.ID], keys[k] = true, true
		out = append(out, Account{
			ID: a.ID, Provider: a.Provider, OnwatchKey: a.OnwatchKey,
			Kind: a.Kind, Tier: a.Tier, Deployments: a.Deployments, Threshold: threshold,
		})
	}
	return New(out), nil
}

func decodeAccount(n *yaml.Node) (configAccount, string) {
	var a configAccount
	if n.Kind != yaml.MappingNode {
		return a, "expected account mapping"
	}
	seen := map[string]bool{}
	for i := 0; i < len(n.Content); i += 2 {
		key, value := n.Content[i], n.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return a, "invalid account field"
		}
		if seen[key.Value] {
			return a, "duplicate account field"
		}
		seen[key.Value] = true
		switch key.Value {
		case "id", "provider", "onwatch_key", "kind", "tier":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
				return a, "account field must be a string"
			}
		case "threshold":
			if value.Kind != yaml.ScalarNode || (value.Tag != "!!int" && value.Tag != "!!float") {
				return a, "threshold must be in (0,1]"
			}
		case "deployments":
			if value.Kind != yaml.SequenceNode {
				return a, "deployments must be a list"
			}
			for _, dep := range value.Content {
				if dep.Kind != yaml.ScalarNode || dep.Tag != "!!str" {
					return a, "deployment id must be a string"
				}
			}
		default:
			return a, "unknown configuration field"
		}
	}
	if err := n.Decode(&a); err != nil {
		return a, "invalid account field value"
	}
	return a, ""
}
