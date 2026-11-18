package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/yesvus/sluice/internal/acp"
)

type acpOptions struct {
	profile, config string
}

func parseACPOptions(args []string, output io.Writer) (acpOptions, error) {
	fs := flag.NewFlagSet("acp", flag.ContinueOnError)
	fs.SetOutput(output)
	profile := fs.String("profile", "", "ACP profile name (required)")
	config := fs.String("config", defaultConfigPath, "path to operator-controlled profile config")
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
		*config = envOrDefault("SLUICE_CONFIG", defaultConfigPath)
	}
	if *config == "" {
		return acpOptions{}, errors.New("acp: config path is required")
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
	p, err := acp.LoadProfile(opts.config, opts.profile)
	if err != nil {
		return err
	}
	workdir, err := os.Getwd()
	if err != nil {
		return errors.New("acp: cannot resolve launch working directory")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	return acp.Serve(ctx, stdioStream{ReadCloser: os.Stdin, Writer: os.Stdout}, opts.profile, p, workdir, os.Stderr)
}
