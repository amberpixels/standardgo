package standardgo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
	"golang.org/x/mod/modfile"
)

// PresetFile is the file a module publishes to offer a standardgo preset.
//
// It sits at the module root and is written in the .standardgo.yml shape, so a
// preset is an overlay that happens to be distributed rather than typed.
const PresetFile = "standardgo-preset.yml"

// modulePlaceholder is what a preset writes where the linted project's module
// path belongs.
//
// It exists because depguard matches deny entries as import-path prefixes with
// no globbing, so a shared layering rule cannot name the packages it forbids
// without knowing the module it is being applied to. That is also why the
// substitution happens here rather than in depguard: by the time depguard
// compiles its rules it has no module context left, while standardgo runs in
// the project root with go.mod next to it.
const modulePlaceholder = "${MODULE}"

// Resolver maps a preset's module path to the directory holding it.
type Resolver func(ctx context.Context, module string) (string, error)

// Project is what a merge needs to know about the module being linted.
type Project struct {
	// Module is the linted project's own module path, substituted into presets.
	// It may be empty when there is no go.mod; only a preset that carries the
	// placeholder then fails, and it says so.
	Module string

	// Resolve locates the presets the overlay names. ModuleDir is the real one;
	// tests supply their own so the merge never shells out.
	Resolve Resolver
}

// loadPreset reads one preset as an overlay.
//
// A preset is subject to the same add-never-override rules as a hand-written
// overlay and goes through the same merge code: a preset that disabled a locked
// linter would be no more legitimate than a project doing it directly.
func loadPreset(ctx context.Context, module string, proj Project) (*Overlay, error) {
	if proj.Resolve == nil {
		return nil, fmt.Errorf("%s names presets but no resolver was supplied", OverlayFile)
	}

	dir, err := proj.Resolve(ctx, module)
	if err != nil {
		return nil, fmt.Errorf("preset %q: %w", module, err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, PresetFile))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, fmt.Errorf(
			"preset %q: module %s has no %s, so it publishes no preset", module, module, PresetFile)
	case err != nil:
		return nil, fmt.Errorf("preset %q: %w", module, err)
	}

	if bytes.Contains(raw, []byte(modulePlaceholder)) {
		if proj.Module == "" {
			return nil, fmt.Errorf(
				"preset %q needs the linted project's module path, and no go.mod was found."+
					" Run standardgo from the module root",
				module)
		}
		raw = bytes.ReplaceAll(raw, []byte(modulePlaceholder), []byte(proj.Module))
	}

	var ov Overlay
	if err := yaml.Unmarshal(raw, &ov); err != nil {
		return nil, fmt.Errorf("parse preset %q: %w", module, err)
	}

	if len(ov.Presets) > 0 {
		return nil, fmt.Errorf(
			"preset %q declares presets of its own (%v); presets do not nest", module, ov.Presets)
	}
	if len(ov.Linters.Disable) > 0 {
		return nil, fmt.Errorf(
			"preset %q disables linters (%v); the shared ruleset is not negotiable from a preset either",
			module, ov.Linters.Disable)
	}

	return &ov, nil
}

// ModuleDir locates a module in the linted project's own module graph.
//
// Going through the module graph rather than fetching a URL is what makes a
// preset reviewable: the version is pinned in the project's go.mod, its content
// is checksummed in go.sum, and it comes from the module cache, so a preset
// cannot change what a project enforces without a diff someone approved.
func ModuleDir(ctx context.Context, module string) (string, error) {
	dir, err := goListDir(ctx, module)
	if err != nil {
		return "", err
	}

	// A module that is required but not yet in the cache reports an empty Dir.
	// Downloading it is the same pinned, checksummed fetch a build would do.
	if dir == "" {
		out, derr := exec.CommandContext(ctx, "go", "mod", "download", module).CombinedOutput()
		if derr != nil {
			return "", fmt.Errorf("go mod download %s: %w: %s", module, derr, bytes.TrimSpace(out))
		}
		if dir, err = goListDir(ctx, module); err != nil {
			return "", err
		}
	}

	if dir == "" {
		return "", fmt.Errorf("module %s resolved to no directory", module)
	}

	return dir, nil
}

// goListDir asks the go command where a module of the graph lives on disk.
func goListDir(ctx context.Context, module string) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Dir}}", module)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf(
			"%s. A preset is an ordinary dependency: add it with `go get %s`",
			strings.TrimSpace(stderr.String()), module)
	}

	return strings.TrimSpace(string(out)), nil
}

// ModulePath reads the module path from the go.mod in dir.
//
// It returns "" when there is no go.mod, which is not an error here: only a
// preset that carries the placeholder actually needs the value, and refusing to
// lint a directory that has no module would be a new restriction.
func ModulePath(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("read go.mod: %w", err)
	}

	mod := modfile.ModulePath(data)
	if mod == "" {
		return "", errors.New("go.mod has no module directive")
	}

	return mod, nil
}
