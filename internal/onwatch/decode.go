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
	case "anthropic", "codex", "opencode", "antigravity", "minimax", "synthetic", "zai", "openrouter":
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
}

func decodeProvider(provider string, data json.RawMessage, requireAccount bool) ([]quota.Snapshot, error) {
	if provider == "synthetic" || provider == "zai" || provider == "openrouter" {
		return decodeCounters(provider, data)
	}
	var response providerResponse
	if !supported(provider) || json.Unmarshal(data, &response) != nil || response.CapturedAt == nil || response.CapturedAt.IsZero() || response.Quotas == nil {
		return nil, errSchema
	}
	if response.Paused || response.AuthRequired || response.IsStale || len(response.Quotas) == 0 {
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
		if provider == "antigravity" {
			window, percent, resets = row.ModelID, row.UsagePercent, row.ResetTime
		}
		if provider == "minimax" {
			percent, resets = row.UsagePercent, row.ResetAt
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
		out = append(out, s)
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
	for _, key := range keys {
		var rows []quota.Snapshot
		var err error
		if key == "codexAccounts" || key == "minimaxAccounts" {
			provider := key[:len(key)-len("Accounts")]
			if !allowed[provider] || data[provider] != nil {
				return nil, errSchema
			}
			var accounts []json.RawMessage
			if json.Unmarshal(data[key], &accounts) != nil {
				return nil, errSchema
			}
			rows, err = decodeAccounts(provider, accounts)
		} else {
			if !allowed[key] {
				return nil, errSchema
			}
			rows, err = decodeProvider(key, data[key], false)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	// Hidden providers are absent by design; never fabricate observations.
	if len(out) == 0 {
		return nil, errUnavailable
	}
	return out, nil
}
