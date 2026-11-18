package onwatch

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/yesvus/sluice/internal/quota"
)

var (
	errSchema      = errors.New("onwatch: unsupported or malformed quota schema")
	errUnavailable = errors.New("onwatch: quota observation unavailable or stale")
)

func supported(provider string) bool {
	switch provider {
	case "anthropic", "codex", "opencode", "antigravity", "minimax", "synthetic", "zai", "openrouter",
		"gemini", "copilot", "cursor", "ollama", "commandcode", "muse", "mistral":
		return true
	default:
		return false
	}
}

type providerResponse struct {
	CapturedAt   *time.Time `json:"capturedAt"`
	AccountID    *int64     `json:"accountId"`
	Quotas       []quotaRow `json:"quotas"`
	Paused       bool       `json:"paused"`
	AuthRequired bool       `json:"authRequired"`
	IsStale      bool       `json:"isStale"`
	CLIActive    bool       `json:"cliActive"`
	Status       string     `json:"status"`
}

type quotaRow struct {
	Name          string     `json:"name"`
	ModelID       string     `json:"modelId"`
	Utilization   *float64   `json:"utilization"`
	UsagePercent  *float64   `json:"usagePercent"`
	Used          *float64   `json:"used"`
	Limit         *float64   `json:"limit"`
	Total         *float64   `json:"total"`
	ResetAt       *time.Time `json:"resetAt"`
	IsStale       bool       `json:"isStale"`
	PercentOnly   *bool      `json:"percentOnly"`
	ResetsAt      *time.Time `json:"resetsAt"`
	ResetTime     *time.Time `json:"resetTime"`
	LastUpdatedAt *time.Time `json:"lastUpdatedAt"`
	ResetDate     *time.Time `json:"resetDate"`
	Entitlement   *float64   `json:"entitlement"`
	Remaining     *float64   `json:"remaining"`
	Unlimited     *bool      `json:"unlimited"`
	LimitUnknown  bool       `json:"limitUnknown"`
	Format        string     `json:"format"`
}

func decodeProvider(provider string, data json.RawMessage, requireAccount bool) ([]quota.Snapshot, error) {
	if provider == "synthetic" || provider == "zai" || provider == "openrouter" {
		return decodeCounters(provider, data)
	}
	var response providerResponse
	if !supported(provider) || json.Unmarshal(data, &response) != nil || response.CapturedAt == nil || response.CapturedAt.IsZero() || response.Quotas == nil {
		return nil, errSchema
	}
	if response.Paused || response.AuthRequired || response.IsStale || response.CLIActive || len(response.Quotas) == 0 ||
		(provider == "mistral" && (response.Status == "reconnect" || response.Status == "stale" || response.Status == "waiting")) {
		return nil, errUnavailable
	}
	account := ""
	if (provider == "codex" || provider == "minimax") && response.AccountID != nil {
		if *response.AccountID <= 0 {
			return nil, errSchema
		}
		account = strconv.FormatInt(*response.AccountID, 10)
	}
	if requireAccount && account == "" {
		return nil, errSchema
	}
	out := make([]quota.Snapshot, 0, len(response.Quotas))
	windows := make(map[string]bool)
	for _, row := range response.Quotas {
		window, percent, resets := row.Name, row.Utilization, row.ResetsAt
		if provider == "antigravity" || provider == "gemini" {
			window, percent, resets = row.ModelID, row.UsagePercent, row.ResetTime
		}
		if provider == "minimax" {
			percent, resets = row.UsagePercent, row.ResetAt
		}
		if provider == "copilot" {
			percent, resets = row.UsagePercent, row.ResetDate
			if row.Unlimited == nil || row.Entitlement == nil || row.Remaining == nil || *row.Entitlement < 0 || *row.Remaining < 0 {
				return nil, errSchema
			}
			if *row.Unlimited || *row.Entitlement == 0 {
				continue
			}
			if *row.Remaining > *row.Entitlement {
				return nil, errSchema
			}
		}
		if row.LimitUnknown && (provider == "ollama" || provider == "commandcode") {
			continue
		}
		if row.IsStale {
			return nil, errUnavailable
		}
		if window == "" || windows[window] || percent == nil || *percent < 0 || math.IsNaN(*percent) || math.IsInf(*percent, 0) {
			return nil, errSchema
		}
		windows[window] = true
		if resets != nil && resets.IsZero() {
			return nil, errSchema
		}
		observed := *response.CapturedAt
		if row.LastUpdatedAt != nil {
			if row.LastUpdatedAt.IsZero() {
				return nil, errSchema
			}
			observed = *row.LastUpdatedAt
		}
		s := quota.Snapshot{Provider: provider, AccountKey: account, Window: window, Utilization: math.Min(*percent/100, 1), ResetsAt: resets, ObservedAt: observed}
		if provider == "opencode" {
			// Percent fixtures can include zero placeholders in used/limit.
			if row.PercentOnly == nil {
				return nil, errSchema
			}
			if !*row.PercentOnly {
				if row.Used == nil || row.Limit == nil || *row.Used < 0 || *row.Limit <= 0 {
					return nil, errSchema
				}
				s.Used, s.Limit = row.Used, row.Limit
			}
		}
		if provider == "minimax" {
			if row.Used == nil || row.Total == nil || *row.Used < 0 || *row.Total <= 0 {
				return nil, errUnavailable
			}
			s.Used, s.Limit = row.Used, row.Total
		}
		if provider == "copilot" {
			used := *row.Entitlement - *row.Remaining
			s.Used, s.Limit = &used, row.Entitlement
		}
		switch provider {
		case "cursor", "ollama", "commandcode", "muse", "mistral":
			switch row.Format {
			case "percent":
				// These providers may expose derived percent/100 counters.
				// Preserve utilization, not fabricated absolute usage.
			case "currency", "dollars", "credits", "count":
				if row.Used == nil || row.Limit == nil || *row.Used < 0 || *row.Limit <= 0 {
					return nil, errUnavailable
				}
				s.Used, s.Limit = row.Used, row.Limit
			default:
				return nil, errSchema
			}
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, errUnavailable
	}
	return out, nil
}

func decodeAccounts(provider string, accounts []json.RawMessage) ([]quota.Snapshot, error) {
	if accounts == nil {
		return nil, errSchema
	}
	if len(accounts) == 0 {
		return nil, errUnavailable
	}
	out := []quota.Snapshot{}
	seen := map[int64]bool{}
	for _, account := range accounts {
		var envelope struct {
			ID *int64 `json:"accountId"`
		}
		if json.Unmarshal(account, &envelope) != nil || envelope.ID == nil || *envelope.ID <= 0 || seen[*envelope.ID] {
			return nil, errSchema
		}
		seen[*envelope.ID] = true
		rows, err := decodeProvider(provider, account, true)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

func combinedProvider(key string) string {
	switch key {
	case "codexAccounts":
		return "codex"
	case "minimaxAccounts":
		return "minimax"
	default:
		return key
	}
}

func decodeCombined(data map[string]json.RawMessage, allowed map[string]bool) ([]quota.Snapshot, error) {
	if data == nil {
		return nil, errSchema
	}
	out := []quota.Snapshot{}
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	observed := make(map[string]bool, len(allowed))
	for _, key := range keys {
		provider := combinedProvider(key)
		if !allowed[provider] || observed[provider] {
			return nil, errSchema
		}
		observed[provider] = true
		var rows []quota.Snapshot
		var err error
		if key == "codexAccounts" || key == "minimaxAccounts" {
			var accounts []json.RawMessage
			if json.Unmarshal(data[key], &accounts) != nil {
				return nil, errSchema
			}
			rows, err = decodeAccounts(provider, accounts)
		} else {
			rows, err = decodeProvider(key, data[key], false)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	// Do not return partial success if a selected provider lacks telemetry.
	if len(out) == 0 || len(observed) != len(allowed) {
		return nil, errUnavailable
	}
	return out, nil
}
