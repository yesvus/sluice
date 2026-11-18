package main

import (
	"io"
	"testing"
)

func TestACPOptions(t *testing.T) {
	t.Setenv("SLUICE_CONFIG", "/synthetic/env.yaml")
	opts, err := parseACPOptions([]string{"--profile", "worker"}, io.Discard)
	if err != nil || opts.profile != "worker" || opts.config != "/synthetic/env.yaml" {
		t.Fatalf("unexpected default: %+v, %v", opts, err)
	}
	opts, err = parseACPOptions([]string{"--profile", "worker", "--config", "/synthetic/flag.yaml"}, io.Discard)
	if err != nil || opts.config != "/synthetic/flag.yaml" {
		t.Fatalf("flag did not override env: %+v, %v", opts, err)
	}
	for _, args := range [][]string{nil, {"--profile", "worker", "extra"}, {"--profile", "worker", "--config", ""}} {
		if _, err := parseACPOptions(args, io.Discard); err == nil {
			t.Fatal("invalid arguments accepted")
		}
	}
}
