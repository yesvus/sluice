package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/yesvus/sluice/internal/acp"
)

const defaultACPConfigPath = "/etc/sluice/profiles.yaml"

type acpOptions struct {
	profile, config string
}

func parseACPOptions(args []string, output io.Writer) (acpOptions, error) {
	fs := flag.NewFlagSet("acp", flag.ContinueOnError)
	fs.SetOutput(output)
	profile := fs.String("profile", "", "ACP profile name (required)")
	config := fs.String("config", defaultACPConfigPath, "path to operator-controlled profile config")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return acpOptions{}, err
		}
		return acpOptions{}, errors.New("acp: invalid flags")
	}
	if *profile == "" || fs.NArg() != 0 {
		return acpOptions{}, errors.New("acp: --profile is required; positional arguments are unsupported")
	}
	if !flagWasSet(fs, "config") {
		*config = envOrDefault("SLUICE_CONFIG", defaultACPConfigPath)
	}
	if !filepath.IsAbs(*config) || filepath.Clean(*config) != *config {
		return acpOptions{}, errors.New("acp: config path must be absolute and clean")
	}
	return acpOptions{profile: *profile, config: *config}, nil
}

type stdioStream struct {
	io.ReadCloser
	io.Writer
}

func runACP(args []string) error {
	opts, err := parseACPOptions(args, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	workdir, err := os.Getwd()
	if err != nil {
		return errors.New("acp: cannot resolve launch working directory")
	}
	config, err := resolveACPConfig(opts.config, workdir)
	if err != nil {
		return err
	}
	p, err := acp.LoadProfile(config, opts.profile)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	return acp.Serve(ctx, stdioStream{ReadCloser: os.Stdin, Writer: os.Stdout}, opts.profile, p, workdir, os.Stderr)
}

func resolveACPConfig(path, workdir string) (string, error) {
	config, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", errors.New("acp: cannot resolve profile config")
	}
	workspace, err := filepath.EvalSymlinks(workdir)
	if err != nil {
		return "", errors.New("acp: cannot resolve launch working directory")
	}
	relative, err := filepath.Rel(workspace, config)
	if err != nil || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return "", errors.New("acp: profile config must be outside launch working directory")
	}
	return config, nil
}
