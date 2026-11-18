package onwatch

import (
	"encoding/json"
	"math"
	"time"

	"github.com/yesvus/sluice/internal/quota"
)

type counterRow struct {
	Usage          *float64   `json:"usage"`
	Limit          *float64   `json:"limit"`
	Percent        *float64   `json:"percent"`
	RenewsAt       *time.Time `json:"renewsAt"`
	TimeUntilReset string     `json:"timeUntilReset"`
}

func decodeCounters(provider string, data json.RawMessage) ([]quota.Snapshot, error) {
	var envelope struct {
		CapturedAt   *time.Time  `json:"capturedAt"`
		Subscription *counterRow `json:"subscription"`
		Search       *counterRow `json:"search"`
		ToolCalls    *counterRow `json:"toolCalls"`
		TokensLimit  *counterRow `json:"tokensLimit"`
		TimeLimit    *counterRow `json:"timeLimit"`
		Credits      *counterRow `json:"credits"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.CapturedAt == nil || envelope.CapturedAt.IsZero() {
		return nil, errSchema
	}
	type named struct {
		name string
		row  *counterRow
	}
	var rows []named
	switch provider {
	case "synthetic":
		rows = []named{{"subscription", envelope.Subscription}, {"search", envelope.Search}, {"toolCalls", envelope.ToolCalls}}
	case "zai":
		// toolCalls is a breakdown, not an independent quota.
		rows = []named{{"tokensLimit", envelope.TokensLimit}, {"timeLimit", envelope.TimeLimit}}
	case "openrouter":
		rows = []named{{"credits", envelope.Credits}}
	default:
		return nil, errSchema
	}
	out := make([]quota.Snapshot, 0, len(rows))
	for _, item := range rows {
		row := item.row
		if row == nil || row.Usage == nil || row.Percent == nil || *row.Usage < 0 || *row.Percent < 0 {
			return nil, errSchema
		}
		if row.Limit == nil || *row.Limit == 0 {
			// Empty/default Synthetic and Z.ai rows and unlimited/unknown
			// OpenRouter limits cannot establish finite quota headroom.
			if row.Limit == nil && provider != "openrouter" {
				return nil, errSchema
			}
			continue
		}
		if *row.Limit < 0 {
			return nil, errSchema
		}
		resets := row.RenewsAt
		if row.TimeUntilReset == "N/A" || provider == "openrouter" {
			resets = nil
		}
		if resets != nil && resets.IsZero() {
			return nil, errSchema
		}
		out = append(out, quota.Snapshot{
			Provider: provider, Window: item.name,
			Utilization: math.Min(*row.Percent/100, 1),
			Used:        row.Usage, Limit: row.Limit,
			ResetsAt: resets, ObservedAt: *envelope.CapturedAt,
		})
	}
	if len(out) == 0 {
		return nil, errUnavailable
	}
	return out, nil
}
