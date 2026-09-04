package standardgo

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/amberpixels/k1/quick"
	"go.yaml.in/yaml/v3"
)

// OverlayFile is the project-local overlay read from the working directory.
const OverlayFile = ".standardgo.yml"

// Overlay is the optional project-local .standardgo.yml.
//
// It deliberately cannot change a rule the shared config already sets — in
// either direction. Making a locked linter stricter is refused just as firmly
// as making it softer, because "stricter" is not decidable in general
// (is a different gocritic check set stricter?) while "already configured" is.
// This mirrors standardrb's extend_config, which drops any cop Standard has
// already configured.
//
// The one sanctioned way to get softer is Ignore, and it is path-scoped: a
// linter may be silenced under a glob, never globally.
type Overlay struct {
	// Presets are module paths, each naming a module that publishes a
	// standardgo-preset.yml. A preset is an overlay a project references instead
	// of pasting, so a convention shared across repos (its layering rules, say)
	// has one definition, versioned where it is defined rather than here.
	Presets []string `yaml:"presets"`

	Linters struct {
		Enable   []string       `yaml:"enable"`
		Disable  []string       `yaml:"disable"`
		Settings map[string]any `yaml:"settings"`
	} `yaml:"linters"`

	Ignore []IgnoreRule `yaml:"ignore"`
}

// IgnoreRule silences specific linters under a path glob.
type IgnoreRule struct {
	Path    string   `yaml:"path"`
	Linters []string `yaml:"linters"`
}

// Merge applies overlay onto the locked config and returns the result.
// It returns an error rather than silently dropping anything the overlay is
// not permitted to do, so a rejected override is visible instead of confusing.
//
// proj says which module is being linted and how to find the presets it names;
// resolving one runs the go command, which is what ctx is for.
func Merge(ctx context.Context, locked, overlay []byte, proj Project) ([]byte, error) {
	var ov Overlay
	if err := yaml.Unmarshal(overlay, &ov); err != nil {
		return nil, fmt.Errorf("parse %s: %w", OverlayFile, err)
	}

	if len(ov.Linters.Disable) > 0 {
		return nil, fmt.Errorf(
			"%s may not disable linters (found %v); the shared ruleset is not negotiable."+
				" To silence one under a path, use:\n\nignore:\n  - path: \"internal/legacy/\"\n    linters: %v",
			OverlayFile, ov.Linters.Disable, ov.Linters.Disable)
	}

	var base map[string]any
	if err := yaml.Unmarshal(locked, &base); err != nil {
		return nil, fmt.Errorf("parse locked config: %w", err)
	}

	lint, _ := base["linters"].(map[string]any)
	if lint == nil {
		return nil, errors.New("locked config has no linters section")
	}

	// Presets are applied before the project's own additions so that a project
	// which also hand-writes what a preset provides is told which preset already
	// owns the key, instead of being refused by the shared ruleset it never
	// touched. owners carries that provenance across the two passes.
	owners := map[string]string{}

	for _, name := range ov.Presets {
		preset, err := loadPreset(ctx, name, proj)
		if err != nil {
			return nil, err
		}
		if err := apply(lint, preset, owners, fmt.Sprintf("preset %q", name)); err != nil {
			return nil, err
		}
	}

	if err := apply(lint, &ov, owners, OverlayFile); err != nil {
		return nil, err
	}

	return yaml.Marshal(base)
}

// apply merges one overlay's additions into the locked linters section.
func apply(lint map[string]any, ov *Overlay, owners map[string]string, src string) error {
	addLinters(lint, ov.Linters.Enable)

	if err := addSettings(lint, ov.Linters.Settings, owners, src); err != nil {
		return err
	}

	return addIgnores(lint, ov.Ignore, src)
}

// ownerOf names whatever already holds a key, for a refusal message.
func ownerOf(owners map[string]string, key string) string {
	if src, ok := owners[key]; ok {
		return src
	}

	return "the shared ruleset"
}

// addLinters enables extra linters. Re-enabling an already-locked linter is a
// no-op rather than an error: it is redundant, not a violation.
func addLinters(lint map[string]any, add []string) {
	if len(add) == 0 {
		return
	}

	// The enable list arrives from YAML as []any, so the additions are widened to
	// match. quick.Append skips values already present, which is exactly the
	// re-enable-is-a-no-op rule, and interface comparison of two any-wrapped
	// strings is ordinary string equality.
	enabled, _ := lint["enable"].([]any)
	lint["enable"] = quick.Append(enabled, widen(add)...)
}

// widen converts a []string to the []any that YAML round-tripping needs.
//
// k1's cast.AsSliceOfAny does this, but its documented contract is to panic when
// the input is not a slice, and its package doc scopes it to testing "where
// panics are acceptable". The panic happens to be unreachable for a []string
// today; that is a property of the current implementation, not a promise. Four
// obvious lines are the better trade in the one binary every repo runs.
func widen(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}

	return out
}

// customKey is the settings key holding every bundled plugin's configuration.
const customKey = "custom"

// pluginIdentity are the keys that decide which code a plugin name binds to.
// An overlay may tune a bundled plugin, never swap out what backs it.
var pluginIdentity = []string{"type", "path", "description", "original-url"}

// addSettings adds settings for linters the shared config does not configure.
func addSettings(lint, add map[string]any, owners map[string]string, src string) error {
	if len(add) == 0 {
		return nil
	}

	settings, _ := lint["settings"].(map[string]any)
	if settings == nil {
		settings = map[string]any{}
	}

	for name, value := range add {
		if name == customKey {
			if err := addCustomSettings(settings, value, owners, src); err != nil {
				return err
			}

			continue
		}

		if _, taken := settings[name]; taken {
			return fmt.Errorf(
				"%s may not override settings for %q, which %s already configures",
				src, name, ownerOf(owners, name))
		}
		settings[name] = value
		owners[name] = src
	}
	lint["settings"] = settings

	return nil
}

// addCustomSettings merges plugin settings into the locked custom namespace.
//
// `custom` is not one linter's settings, it is the map holding every bundled
// plugin's. Refusing the key outright would lock every plugin at once,
// including ones the shared ruleset only declares and never tunes, so the
// add-never-override rule is applied one level down instead: per plugin.
//
// A plugin's own `settings` block stays a unit, exactly like an ordinary
// linter's settings. Once the shared ruleset tunes a plugin, that plugin is
// closed. The namespace is the special case, not what sits inside it.
func addCustomSettings(settings map[string]any, value any, owners map[string]string, src string) error {
	add, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf(
			"%s: linters.settings.custom must map a plugin name to its settings", src)
	}

	plugins, _ := settings[customKey].(map[string]any)
	if plugins == nil {
		plugins = map[string]any{}
	}

	for name, value := range add {
		declared, ok := plugins[name]
		if !ok {
			return fmt.Errorf(
				"%s: unknown plugin %q. Plugins are compiled into this binary, so an overlay may"+
					" configure the bundled ones (%v) but cannot add another",
				src, name, slices.Sorted(maps.Keys(plugins)))
		}

		locked, ok := declared.(map[string]any)
		if !ok {
			return fmt.Errorf("locked config declares plugin %q as %T, want a map", name, declared)
		}

		fields, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: settings for plugin %q must be a map", src, name)
		}

		if err := addPluginFields(locked, fields, name, owners, src); err != nil {
			return err
		}
	}
	settings[customKey] = plugins

	return nil
}

// addPluginFields merges one plugin's overlay keys onto its locked declaration.
func addPluginFields(locked, add map[string]any, name string, owners map[string]string, src string) error {
	for key, value := range add {
		if slices.Contains(pluginIdentity, key) {
			return fmt.Errorf(
				"%s may not set %q on plugin %q; that decides what backs the plugin, and the"+
					" plugin is compiled into this binary",
				src, key, name)
		}

		owned := customKey + "." + name + "." + key
		if _, taken := locked[key]; taken {
			return fmt.Errorf(
				"%s may not override %q for plugin %q, which %s already configures",
				src, key, name, ownerOf(owners, owned))
		}
		locked[key] = value
		owners[owned] = src
	}

	return nil
}

// addIgnores appends path-scoped exclusions.
func addIgnores(lint map[string]any, ignores []IgnoreRule, src string) error {
	if len(ignores) == 0 {
		return nil
	}

	exclusions, _ := lint["exclusions"].(map[string]any)
	if exclusions == nil {
		exclusions = map[string]any{}
		lint["exclusions"] = exclusions
	}
	rules, _ := exclusions["rules"].([]any)

	for _, ig := range ignores {
		switch {
		case ig.Path == "":
			return fmt.Errorf(
				"%s: ignore entry needs a path; a bare linter list would disable it globally", src)
		case len(ig.Linters) == 0:
			return fmt.Errorf(
				"%s: ignore entry for %q must name the linters it silences", src, ig.Path)
		}

		rules = append(rules, map[string]any{
			"path":    ig.Path,
			"linters": widen(ig.Linters),
		})
	}
	exclusions["rules"] = rules

	return nil
}
