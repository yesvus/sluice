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

var errSchema = errors.New("onwatch /api/current: unsupported or malformed quota schema")

func supported(provider string) bool {
	switch provider {
	case "anthropic", "codex", "opencode", "antigravity":
		return true
	default:
		return false
	}
}

type providerResponse struct {
	CapturedAt *time.Time `json:"capturedAt"`
	AccountID  *int64     `json:"accountId"`
	Quotas     []quotaRow `json:"quotas"`
}

type quotaRow struct {
	Name          string     `json:"name"`
	ModelID       string     `json:"modelId"`
	Utilization   *float64   `json:"utilization"`
	UsagePercent  *float64   `json:"usagePercent"`
	Used          *float64   `json:"used"`
	Limit         *float64   `json:"limit"`
	PercentOnly   *bool      `json:"percentOnly"`
	ResetsAt      *time.Time `json:"resetsAt"`
	ResetTime     *time.Time `json:"resetTime"`
	LastUpdatedAt *time.Time `json:"lastUpdatedAt"`
}

func decodeProvider(provider string, data json.RawMessage, requireAccount bool) ([]quota.Snapshot, error) {
	var response providerResponse
	if !supported(provider) || json.Unmarshal(data, &response) != nil || response.CapturedAt == nil || response.CapturedAt.IsZero() || response.Quotas == nil {
		return nil, errSchema
	}
	account := ""
	if provider == "codex" && response.AccountID != nil {
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
		if window == "" || windows[window] || percent == nil || *percent < 0 || math.IsNaN(*percent) || math.IsInf(*percent, 0) {
			return nil, errSchema
		}
		windows[window] = true
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
		out = append(out, s)
	}
	return out, nil
}

func decodeAccounts(accounts []json.RawMessage) ([]quota.Snapshot, error) {
	if accounts == nil {
		return nil, errSchema
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
		rows, err := decodeProvider("codex", account, true)
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
		if key == "codexAccounts" {
			if !allowed["codex"] || data["codex"] != nil {
				return nil, errSchema
			}
			var accounts []json.RawMessage
			if json.Unmarshal(data[key], &accounts) != nil {
				return nil, errSchema
			}
			rows, err = decodeAccounts(accounts)
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
	// Hidden/no-data providers must not turn into zero utilization samples.
	return out, nil
}
