// Command es-sampler copies documents from a source Elasticsearch cluster to a
// destination cluster on a loop.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/elastic/es-sampler/internal/sampler"
)

func main() {
	// Buffer warnings from .env parsing so we can emit them through the
	// real logger once --verbose is known (from ParseConfig).
	var buffered []string
	bufLog := sampler.Logger(func(msg string) { buffered = append(buffered, msg) })

	envFile, explicit := envFileFromArgs(os.Args[1:])
	if explicit {
		if _, err := os.Stat(envFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error: --env-file %s: %v\n", envFile, err)
			os.Exit(1)
		}
	}
	if err := sampler.LoadDotEnv(envFile, bufLog); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cfg, err := sampler.ParseConfig(os.Args[1:])
	if err != nil {
		if err == sampler.ErrHelpRequested {
			fmt.Println(strings.TrimSpace(sampler.HelpText))
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	log := sampler.NewLogger(cfg.Verbose)
	for _, msg := range buffered {
		log(msg)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := sampler.Run(ctx, cfg, log); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: %v\n", err)
		os.Exit(1)
	}
}

// envFileFromArgs returns the path to load env vars from and whether the user
// explicitly supplied it (via --env-file). When not explicit, ".env" is used.
func envFileFromArgs(args []string) (path string, explicit bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--env-file":
			if i+1 < len(args) {
				return args[i+1], true
			}
		case strings.HasPrefix(a, "--env-file="):
			return strings.TrimPrefix(a, "--env-file="), true
		}
	}
	return ".env", false
}
