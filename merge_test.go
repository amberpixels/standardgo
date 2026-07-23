package standardgo_test

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/amberpixels/standardgo"
)

func TestMergeRejectsDisable(t *testing.T) {
	_, err := standardgo.Merge(standardgo.Config, []byte("linters:\n  disable: [gosec]\n"))
	if err == nil {
		t.Fatal("expected disabling a locked linter to be refused")
	}
	if !strings.Contains(err.Error(), "not negotiable") {
		t.Errorf("error should explain why: %v", err)
	}
}

func TestMergeRejectsSettingsOverride(t *testing.T) {
	// gocritic is configured by the shared ruleset, so the overlay may not touch it.
	_, err := standardgo.Merge(standardgo.Config, []byte(
		"linters:\n  settings:\n    gocritic:\n      enable-all: false\n"))
	if err == nil {
		t.Fatal("expected overriding a locked linter's settings to be refused")
	}
}

func TestMergeAddsUnconfiguredSettings(t *testing.T) {
	// depguard is enabled but not configured by the shared ruleset, so a
	// project may supply its own rules for it.
	out, err := standardgo.Merge(standardgo.Config, []byte(
		"linters:\n  settings:\n    depguard:\n      rules:\n        main:\n          files: [\"**/x/**\"]\n"))
	if err != nil {
		t.Fatalf("adding settings for an unconfigured linter should be allowed: %v", err)
	}
	if !strings.Contains(string(out), "depguard") {
		t.Error("depguard settings were not merged in")
	}
}

func TestMergeAddsLinters(t *testing.T) {
	out, err := standardgo.Merge(standardgo.Config, []byte("linters:\n  enable: [dupl]\n"))
	if err != nil {
		t.Fatalf("adding a linter should be allowed: %v", err)
	}
	if !strings.Contains(string(out), "dupl") {
		t.Error("dupl was not added")
	}
}

// TestMergeReEnableIsNoOp pins the dedup behaviour now delegated to quick.Append:
// naming an already-locked linter must not duplicate it in the enable list.
func TestMergeReEnableIsNoOp(t *testing.T) {
	out, err := standardgo.Merge(standardgo.Config, []byte("linters:\n  enable: [gosec, gosec, dupl]\n"))
	if err != nil {
		t.Fatalf("re-enabling a locked linter should be allowed: %v", err)
	}

	var cfg map[string]any
	if err := yaml.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	linters, _ := cfg["linters"].(map[string]any)
	enabled, _ := linters["enable"].([]any)

	var gosec, dupl int
	for _, e := range enabled {
		switch e {
		case "gosec":
			gosec++
		case "dupl":
			dupl++
		}
	}
	if gosec != 1 {
		t.Errorf("gosec should appear exactly once, got %d", gosec)
	}
	if dupl != 1 {
		t.Errorf("dupl should have been added exactly once, got %d", dupl)
	}
}

// TestMergeRejectsNonStringLintersWithoutPanicking guards the invariant that
// keeps quick.Append safe here.
//
// quick.Append is generic over comparable and we instantiate it with any, so a
// non-comparable element (a slice or map) would panic with "hash of unhashable
// type" rather than fail cleanly. That is unreachable only because Overlay
// declares Enable as []string, which makes yaml.Unmarshal reject anything else
// before it can reach the append. If that field type is ever widened, these
// cases turn into panics.
func TestMergeRejectsNonStringLintersWithoutPanicking(t *testing.T) {
	for _, tc := range []struct{ name, overlay string }{
		{"nested list", "linters:\n  enable: [[a, b]]\n"},
		{"map element", "linters:\n  enable:\n    - {a: b}\n"},
		{"enable is a map", "linters:\n  enable: {a: b}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("must fail cleanly, not panic: %v", r)
				}
			}()

			if _, err := standardgo.Merge(standardgo.Config, []byte(tc.overlay)); err == nil {
				t.Error("expected a parse error for a non-string linter name")
			}
		})
	}
}

func TestMergeIgnoreRequiresPath(t *testing.T) {
	_, err := standardgo.Merge(standardgo.Config, []byte("ignore:\n  - linters: [gosec]\n"))
	if err == nil {
		t.Fatal("a pathless ignore is a global disable and must be refused")
	}
}

func TestMergeIgnoreRequiresLinters(t *testing.T) {
	_, err := standardgo.Merge(standardgo.Config, []byte("ignore:\n  - path: \"internal/\"\n"))
	if err == nil {
		t.Fatal("an ignore with no linters named must be refused")
	}
}

func TestMergeIgnoreAddsExclusion(t *testing.T) {
	out, err := standardgo.Merge(standardgo.Config, []byte(
		"ignore:\n  - path: \"internal/legacy/\"\n    linters: [gosec, funcorder]\n"))
	if err != nil {
		t.Fatalf("path-scoped ignore should be allowed: %v", err)
	}
	if !strings.Contains(string(out), "internal/legacy/") {
		t.Error("exclusion rule was not added")
	}
}

func TestMergeEmptyOverlayIsInert(t *testing.T) {
	out, err := standardgo.Merge(standardgo.Config, nil)
	if err != nil {
		t.Fatalf("an empty overlay must be a no-op: %v", err)
	}

	var got, want map[string]any
	if err := yaml.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(standardgo.Config, &want); err != nil {
		t.Fatal(err)
	}

	gotLinters, _ := got["linters"].(map[string]any)
	wantLinters, _ := want["linters"].(map[string]any)
	gotEnabled, _ := gotLinters["enable"].([]any)
	wantEnabled, _ := wantLinters["enable"].([]any)

	if len(gotEnabled) != len(wantEnabled) {
		t.Errorf("linter count changed: got %d, want %d", len(gotEnabled), len(wantEnabled))
	}
}

// TestConfigIsValidYAML guards the embed: a malformed ruleset should fail here
// rather than in every consuming project at once.
func TestConfigIsValidYAML(t *testing.T) {
	var cfg map[string]any
	if err := yaml.Unmarshal(standardgo.Config, &cfg); err != nil {
		t.Fatalf("embedded ruleset is not valid YAML: %v", err)
	}
	if cfg["version"] != "2" {
		t.Errorf("ruleset must declare version 2, got %v", cfg["version"])
	}
}
