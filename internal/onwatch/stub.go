package onwatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yesvus/sluice/internal/quota"
)

const maxBody = 4 << 20

type Client struct {
	base       *url.URL
	user, pass string
	http       *http.Client
}

type Option func(*Client)

func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.http.Timeout = timeout
		}
	}
}

func New(baseURL, user, pass string, opts ...Option) *Client {
	c := &Client{user: user, pass: pass, http: &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
	if u, err := url.Parse(baseURL); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" {
		c.base = u
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

var _ quota.Source = (*Client)(nil)

type statusError struct {
	endpoint string
	status   int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("onwatch %s: HTTP %d", e.endpoint, e.status)
}

func (c *Client) get(ctx context.Context, endpoint string, query url.Values, out any) error {
	if c.base == nil {
		return fmt.Errorf("onwatch %s: invalid base URL", endpoint)
	}
	u := *c.base
	u.Path = strings.TrimRight(u.Path, "/") + endpoint
	u.RawPath = ""
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("onwatch %s: invalid request", endpoint)
	}
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth(c.user, c.pass)
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("onwatch %s: %w", endpoint, ctx.Err())
		}
		return fmt.Errorf("onwatch %s: transport failed", endpoint)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &statusError{endpoint, resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return fmt.Errorf("onwatch %s: invalid or oversized response", endpoint)
	}
	if out == nil {
		return nil
	}
	if json.Unmarshal(body, out) != nil || strings.TrimSpace(string(body)) == "null" {
		return fmt.Errorf("onwatch %s: malformed JSON or schema mismatch", endpoint)
	}
	return nil
}

type providerList struct {
	Providers []string `json:"providers"`
}

func (c *Client) providers(ctx context.Context) ([]string, error) {
	var response providerList
	if err := c.get(ctx, "/api/providers", nil, &response); err != nil {
		return nil, err
	}
	if response.Providers == nil {
		return nil, errors.New("onwatch /api/providers: missing providers list")
	}
	return response.Providers, nil
}

func (c *Client) Health(ctx context.Context) error {
	err := c.get(ctx, "/health", nil, nil)
	var status *statusError
	if errors.As(err, &status) && status.status == http.StatusNotFound {
		// v2.14.13 has no /health route. Discovery proves API reachability,
		// not provider health or freshness.
		_, err = c.providers(ctx)
	}
	return err
}

func (c *Client) Current(ctx context.Context) ([]quota.Snapshot, error) {
	providers, err := c.providers(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	both := false
	for _, provider := range providers {
		if provider == "both" {
			if both {
				return nil, errors.New("onwatch /api/providers: duplicate combined provider")
			}
			both = true
			continue
		}
		if !supported(provider) || seen[provider] {
			return nil, errors.New("onwatch /api/providers: unsupported or duplicate provider")
		}
		seen[provider] = true
	}
	if len(seen) == 0 {
		return nil, errors.New("onwatch /api/providers: no supported providers")
	}
	if both {
		var data map[string]json.RawMessage
		if err := c.get(ctx, "/api/current", url.Values{"provider": {"both"}}, &data); err != nil {
			return nil, err
		}
		return decodeCombined(data, seen)
	}
	if len(seen) != 1 {
		return nil, errors.New("onwatch /api/providers: missing combined provider")
	}
	for provider := range seen {
		if provider == "codex" || provider == "minimax" {
			// Single-provider current returns only one account.
			var data struct {
				Accounts []json.RawMessage `json:"accounts"`
			}
			endpoint, query := "/api/codex/usage", url.Values{"all": {"true"}}
			if provider == "minimax" {
				endpoint, query = "/api/minimax/accounts/usage", nil
			}
			if err := c.get(ctx, endpoint, query, &data); err != nil {
				return nil, err
			}
			return decodeAccounts(provider, data.Accounts)
		}
		var data json.RawMessage
		if err := c.get(ctx, "/api/current", url.Values{"provider": {provider}}, &data); err != nil {
			return nil, err
		}
		return decodeProvider(provider, data, false)
	}
	return nil, errors.New("onwatch /api/current: missing provider")
}
