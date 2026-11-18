package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yesvus/sluice/internal/exporter"
	"github.com/yesvus/sluice/internal/webhook"
)

const (
	defaultListen     = "127.0.0.1:9312"
	defaultConfigPath = "config.local.yaml"
	defaultOnwatchURL = "http://127.0.0.1:9211"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: sluice <version|serve> [options]")
	}

	switch args[0] {
	case "version":
		fmt.Printf("sluice %s\n", version)
		return nil
	case "serve":
		return serve(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

type serveOptions struct {
	listen, config, onwatchURL string
}

func parseServeOptions(args []string) (serveOptions, error) {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	listen := fs.String("listen", envOrDefault("SLUICE_LISTEN", defaultListen), "address to listen on")
	config := fs.String("config", envOrDefault("SLUICE_CONFIG", defaultConfigPath), "path to local config file")
	// Unlike the other flags, this URL may embed credentials, so the env value
	// must stay out of the help-visible default.
	onwatchURL := fs.String("onwatch-url", defaultOnwatchURL, "onWatch base URL")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return serveOptions{}, flag.ErrHelp
		}
		return serveOptions{}, errors.New("serve: invalid flags")
	}
	if fs.NArg() > 0 {
		return serveOptions{}, errors.New("serve: unexpected positional arguments")
	}
	if !flagWasSet(fs, "onwatch-url") {
		if env := os.Getenv("SLUICE_ONWATCH_URL"); env != "" {
			*onwatchURL = env
		}
	}

	return serveOptions{listen: *listen, config: *config, onwatchURL: *onwatchURL}, nil
}

func serve(args []string) error {
	opts, err := parseServeOptions(args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /metrics", exporter.Handler)
	mux.HandleFunc("POST /hooks/onwatch", webhook.Handler)

	srv := &http.Server{
		Addr:              opts.listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("sluice serve listen=%s config=%s onwatch=%s", opts.listen, opts.config, redactURL(opts.onwatchURL))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case sig := <-sigCh:
		log.Printf("received %s, shutting down", sig)
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return err
	}
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

func redactURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "<unparseable-url>"
	}
	return (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String()
}
