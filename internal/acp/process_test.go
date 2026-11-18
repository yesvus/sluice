package acp

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFakeCLIProcess(t *testing.T) {
	if os.Getenv("SLUICE_SYNTHETIC_CHILD") != "1" {
		return
	}
	if os.Getenv("SLUICE_TEST_PARENT_ONLY") != "" {
		os.Exit(5)
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fmt.Fprintln(os.Stdout, scanner.Text())
	}
	os.Exit(0)
}

func TestStartProcess(t *testing.T) {
	t.Setenv("SLUICE_TEST_PARENT_ONLY", "synthetic")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	child, err := startProcess(ctx, executable, []string{"-test.run=^TestFakeCLIProcess$"}, []string{"SLUICE_SYNTHETIC_CHILD=1"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	if _, err := child.Write([]byte("synthetic ACP line\n")); err != nil {
		t.Fatal(err)
	}
	result, err := bufio.NewReader(child).ReadString('\n')
	if err != nil || strings.TrimSpace(result) != "synthetic ACP line" {
		t.Fatalf("subprocess transport failed: %v", err)
	}
	// Cancellation must terminate the child, and repeated Close must be safe.
	cancel()
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStartProcessRejectsUnsafeInputs(t *testing.T) {
	ctx := context.Background()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []struct {
		executable string
		env        []string
		dir        string
	}{
		{"relative", []string{}, t.TempDir()},
		{executable, nil, t.TempDir()},
		{executable, []string{}, "relative"},
	} {
		if p, err := startProcess(ctx, args.executable, nil, args.env, args.dir); err == nil {
			_ = p.Close()
			t.Fatal("unsafe subprocess inputs accepted")
		}
	}
}
