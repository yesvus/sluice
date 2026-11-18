package acp

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const maxConfigBytes = 1 << 20

var (
	errProfile = errors.New("invalid or unsupported ACP profile")
	safeID     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	safeModel  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,255}$`)
)

type Profile struct {
	Backends      []BackendSpec `yaml:"backends"`
	LimitPatterns []string      `yaml:"limit_patterns"`
	Limits        struct {
		MaxTurns   int `yaml:"max_turns"`
		MaxMinutes int `yaml:"max_minutes"`
	} `yaml:"limits"`
}

type BackendSpec struct {
	ID        string   `yaml:"id"`
	Command   []string `yaml:"command"`
	Home      string   `yaml:"home"`
	Config    string   `yaml:"config"`
	Model     string   `yaml:"model"`
	Effort    string   `yaml:"effort"`
	AccountID string   `yaml:"account_id"`
}

func LoadProfile(path, name string) (Profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return Profile{}, errors.New("cannot open ACP profile config")
	}
	defer f.Close()
	return decodeProfile(f, name)
}

func decodeProfile(r io.Reader, name string) (Profile, error) {
	if !safeID.MatchString(name) {
		return Profile{}, errProfile
	}
	data, err := io.ReadAll(io.LimitReader(r, maxConfigBytes+1))
	if err != nil || len(data) > maxConfigBytes {
		return Profile{}, errProfile
	}
	var config struct {
		Profiles map[string]Profile `yaml:"profiles"`
	}
	d := yaml.NewDecoder(strings.NewReader(string(data)))
	d.KnownFields(true)
	if d.Decode(&config) != nil {
		return Profile{}, errProfile
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return Profile{}, errProfile
	}
	p, ok := config.Profiles[name]
	if !ok || p.validate() != nil {
		return Profile{}, errProfile
	}
	return p, nil
}

func (p Profile) validate() error {
	if len(p.Backends) == 0 || len(p.Backends) > 2 || p.Limits.MaxTurns != 0 || p.Limits.MaxMinutes != 0 {
		return errProfile
	}
	ids := map[string]bool{}
	kinds := map[string]bool{}
	accounts := map[string]bool{}
	homes := []string{}
	for _, b := range p.Backends {
		if !safeID.MatchString(b.ID) || !safeModel.MatchString(b.Model) || ids[b.ID] || len(b.Command) == 0 {
			return errProfile
		}
		executable := b.Command[0]
		if filepath.Base(executable) != executable && (!filepath.IsAbs(executable) || filepath.Clean(executable) != executable) {
			return errProfile
		}
		kind := filepath.Base(executable)
		// Both official CLIs require a pre-provisioned profile-specific home.
		if !filepath.IsAbs(b.Home) || filepath.Clean(b.Home) != b.Home || b.Home == "/" {
			return errProfile
		}
		for _, home := range homes {
			if b.Home == home || strings.HasPrefix(b.Home, home+string(filepath.Separator)) || strings.HasPrefix(home, b.Home+string(filepath.Separator)) {
				return errProfile
			}
		}
		homes = append(homes, b.Home)
		if kinds[kind] {
			return errProfile
		}
		if b.AccountID != "" && (!safeID.MatchString(b.AccountID) || accounts[b.AccountID]) {
			return errProfile
		}
		switch kind {
		case "codex-acp":
			if len(b.Command) != 1 || b.Config != "" || b.Effort != "high" {
				return errProfile
			}
		case "opencode":
			if len(b.Command) != 2 || b.Command[1] != "acp" || b.Config != "managed" || b.Effort != "" {
				return errProfile
			}
		default:
			return errProfile
		}
		ids[b.ID], kinds[kind] = true, true
		accounts[b.AccountID] = true
	}
	for _, pattern := range p.LimitPatterns {
		if strings.TrimSpace(pattern) != pattern || len(pattern) < 8 || len(pattern) > 256 {
			return errProfile
		}
	}
	return nil
}

func (p Profile) selection() []Backend {
	backends := make([]Backend, 0, len(p.Backends))
	for _, b := range p.Backends {
		backends = append(backends, Backend{ID: b.ID, Model: b.Model})
	}
	return backends
}
