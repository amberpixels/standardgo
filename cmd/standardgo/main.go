// Command standardgo lints a Go project with the locked amberpixels ruleset.
//
// The golangci-lint engine is compiled into this binary rather than shelled out
// to, so the engine version and the ruleset version are pinned together by a
// single line in the consuming project's go.mod. There is nothing to install on
// the machine and nothing for CI to drift away from.
//
//	go tool standardgo            # lint
//	go tool standardgo fmt        # format
//	go tool standardgo ./... --fix
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"syscall"

	"github.com/golangci/golangci-lint/v2/pkg/commands"
	"github.com/golangci/golangci-lint/v2/pkg/exitcodes"

	"github.com/amberpixels/standardgo"
)

// engineModule is the engine whose version we report alongside our own.
const engineModule = "github.com/golangci/golangci-lint/v2"

// passthrough are subcommands forwarded to the engine as-is.
var passthrough = []string{
	"run", "fmt", "config", "cache", "custom", "help", "linters", "formatters", "version",
}

// configless are subcommands that must not be handed a --config flag.
var configless = []string{"cache", "custom", "help", "version"}

func main() {
	os.Exit(start())
}

// start is where the work happens, so that main holds the one os.Exit and this
// function's deferred cleanup actually runs.
func start() int {
	// Resolving a preset shells out to the go command, which can be slow on a
	// cold module cache. Ctrl-C should kill that, not wait for it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "standardgo:", err)

		return exitcodes.Failure
	}

	return 0
}

func run(ctx context.Context) error {
	args := os.Args[1:]

	// Refuse --config outright. Allowing it would let any project swap the
	// ruleset out and quietly opt back into the drift standardgo exists to
	// prevent. standardrb strips the same flag for the same reason.
	for _, arg := range args {
		if arg == "-c" || arg == "--config" || strings.HasPrefix(arg, "--config=") {
			return errors.New(
				"--config is not accepted; the ruleset ships with this binary." +
					" Project-local additions go in " + standardgo.OverlayFile)
		}
	}

	sub := "run"
	if len(args) > 0 && slices.Contains(passthrough, args[0]) {
		sub, args = args[0], args[1:]
	}

	engineArgs := append([]string{os.Args[0], sub}, args...)

	if !slices.Contains(configless, sub) {
		cfg, err := writeConfig(ctx)
		if err != nil {
			return err
		}
		engineArgs = append(engineArgs, "--config", cfg)
	}

	// commands.Execute reads os.Args directly, so the engine is driven by
	// rewriting them rather than by a parameter.
	os.Args = engineArgs

	return commands.Execute(buildInfo())
}

// writeConfig materialises the effective config and returns its path.
//
// The filename is derived from the content hash, so repeated runs reuse one
// file instead of accumulating temp files. That matters because the engine
// exits the process on lint failure and never unwinds a deferred cleanup.
func writeConfig(ctx context.Context) (string, error) {
	conf := standardgo.Config

	overlay, err := os.ReadFile(standardgo.OverlayFile)
	switch {
	case err == nil:
		// Only presets need either of these, but resolving the module path here
		// keeps Merge a function of its inputs rather than something that reaches
		// for the filesystem halfway through a config transform.
		mod, merr := standardgo.ModulePath(".")
		if merr != nil {
			return "", merr
		}

		proj := standardgo.Project{Module: mod, Resolve: standardgo.ModuleDir}
		if conf, err = standardgo.Merge(ctx, conf, overlay, proj); err != nil {
			return "", err
		}
	case !os.IsNotExist(err):
		return "", fmt.Errorf("read %s: %w", standardgo.OverlayFile, err)
	}

	sum := sha256.Sum256(conf)
	path := filepath.Join(os.TempDir(), "standardgo-"+hex.EncodeToString(sum[:8])+".yml")

	//nolint:gosec // G703: path is os.TempDir() plus a hash of our own content; no external input reaches it.
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, conf) {
		return path, nil
	}
	//nolint:gosec // G703: same construction as the read above.
	if err := os.WriteFile(path, conf, 0o600); err != nil {
		return "", fmt.Errorf("write effective config: %w", err)
	}

	return path, nil
}

// buildInfo reports standardgo's own version and the engine it was built
// against, so `standardgo version` shows what actually produced the results.
func buildInfo() commands.BuildInfo {
	info := commands.BuildInfo{
		GoVersion: runtime.Version(),
		Version:   "unknown",
		Commit:    "?",
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}

	info.BuildInfo = bi
	if bi.Main.Version != "" {
		info.Version = bi.Main.Version
	}
	for _, dep := range bi.Deps {
		if dep.Path == engineModule {
			info.Version = fmt.Sprintf("%s (golangci-lint %s)", info.Version, dep.Version)
			break
		}
	}

	return info
}
