package standardgo

import (
	"errors"
	"fmt"

	"github.com/amberpixels/k1/quick"
	"gopkg.in/yaml.v3"
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
func Merge(locked, overlay []byte) ([]byte, error) {
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

	addLinters(lint, ov.Linters.Enable)

	if err := addSettings(lint, ov.Linters.Settings); err != nil {
		return nil, err
	}
	if err := addIgnores(lint, ov.Ignore); err != nil {
		return nil, err
	}

	return yaml.Marshal(base)
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

// addSettings adds settings for linters the shared config does not configure.
func addSettings(lint, add map[string]any) error {
	if len(add) == 0 {
		return nil
	}

	settings, _ := lint["settings"].(map[string]any)
	if settings == nil {
		settings = map[string]any{}
	}

	for name, value := range add {
		if _, taken := settings[name]; taken {
			return fmt.Errorf(
				"%s may not override settings for %q, which the shared ruleset already configures",
				OverlayFile, name)
		}
		settings[name] = value
	}
	lint["settings"] = settings

	return nil
}

// addIgnores appends path-scoped exclusions.
func addIgnores(lint map[string]any, ignores []IgnoreRule) error {
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
				"%s: ignore entry needs a path; a bare linter list would disable it globally", OverlayFile)
		case len(ig.Linters) == 0:
			return fmt.Errorf(
				"%s: ignore entry for %q must name the linters it silences", OverlayFile, ig.Path)
		}

		rules = append(rules, map[string]any{
			"path":    ig.Path,
			"linters": widen(ig.Linters),
		})
	}
	exclusions["rules"] = rules

	return nil
}
