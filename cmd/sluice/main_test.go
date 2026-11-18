package main

import "testing"

func TestRedactURL(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"https://example.test:9211", "https://example.test:9211"},
		{"https://fake-user:fake-pass@example.test/path-secret?key=fake-key#fake-fragment", "https://example.test"},
		{"http://[::1]:9211/path", "http://[::1]:9211"},
		{"https://%zz", "<unparseable-url>"},
		{"relative/path", "<unparseable-url>"},
		{"https:opaque-secret", "<unparseable-url>"},
		{"ftp://example.test", "<unparseable-url>"},
	} {
		if got := redactURL(tc.raw); got != tc.want {
			t.Errorf("redactURL(%q) = %q; want %q", tc.raw, got, tc.want)
		}
	}
}

func TestServeOptionsFlagPrecedence(t *testing.T) {
	t.Setenv("SLUICE_LISTEN", "127.0.0.1:1111")
	t.Setenv("SLUICE_CONFIG", "fake-env.yaml")
	t.Setenv("SLUICE_ONWATCH_URL", "https://env.example.test")
	got, err := parseServeOptions(nil)
	if err != nil || got != (serveOptions{"127.0.0.1:1111", "fake-env.yaml", "https://env.example.test"}) {
		t.Fatalf("environment defaults: %+v, %v", got, err)
	}
	got, err = parseServeOptions([]string{"--listen", "127.0.0.1:2222", "--config", "fake-flag.yaml", "--onwatch-url", "https://flag.example.test"})
	if err != nil || got != (serveOptions{"127.0.0.1:2222", "fake-flag.yaml", "https://flag.example.test"}) {
		t.Fatalf("flag overrides: %+v, %v", got, err)
	}
}
