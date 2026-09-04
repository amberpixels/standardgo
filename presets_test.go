package standardgo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/amberpixels/standardgo"
)

const (
	testModule   = "github.com/amberpixels/runwell"
	presetModule = "github.com/amberpixels/dcba"
)

// fixture resolves preset modules to testdata instead of the module cache, so
// the merge is tested without a go command in the loop.
func fixture(t *testing.T) standardgo.Project {
	t.Helper()

	return standardgo.Project{
		Module: testModule,
		Resolve: func(_ context.Context, module string) (string, error) {
			switch module {
			case presetModule:
				return "testdata/preset", nil
			case "github.com/amberpixels/nopreset":
				return "testdata/nopreset", nil
			default:
				return "", errors.New("module " + module + ": not a known dependency")
			}
		},
	}
}

func overlay(presets ...string) []byte {
	var b strings.Builder

	b.WriteString("presets:\n")
	for _, p := range presets {
		b.WriteString("  - " + p + "\n")
	}

	return []byte(b.String())
}

func TestMergeAppliesPreset(t *testing.T) {
	out, err := standardgo.Merge(t.Context(), standardgo.Config, overlay(presetModule), fixture(t))
	if err != nil {
		t.Fatalf("naming a preset the project depends on should be allowed: %v", err)
	}

	got := string(out)
	if !strings.Contains(got, "layering") {
		t.Error("preset rules were not merged in")
	}
	if !strings.Contains(got, testModule+"/internal/application") {
		t.Error("preset was not expanded against the linted module")
	}

	// The preset enables the linter it configures, so the reference is the whole
	// of what a project has to write.
	var cfg map[string]any
	if err := yaml.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	linters, _ := cfg["linters"].(map[string]any)
	enabled, _ := linters["enable"].([]any)
	if !containsAny(enabled, "depguard") {
		t.Error("preset configured depguard without enabling it")
	}
}

// TestMergePresetExpandsEveryPlaceholder guards the substitution: a placeholder
// that survives into the effective config becomes a deny prefix matching
// nothing, so the rule silently passes instead of failing loudly.
func TestMergePresetExpandsEveryPlaceholder(t *testing.T) {
	out, err := standardgo.Merge(t.Context(), standardgo.Config, overlay(presetModule), fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "${MODULE}") {
		t.Error("effective config still carries an unexpanded ${MODULE}")
	}
}

func TestMergePresetRequiresModulePath(t *testing.T) {
	proj := fixture(t)
	proj.Module = ""

	_, err := standardgo.Merge(t.Context(), standardgo.Config, overlay(presetModule), proj)
	if err == nil {
		t.Fatal("a preset that needs the module path must not be applied without one")
	}
	if !strings.Contains(err.Error(), "go.mod") {
		t.Errorf("error should say what is missing: %v", err)
	}
}

// TestMergePresetNotADependency pins the diagnosis a project gets when it names
// a preset it has not required: the fix is a go get, not a standardgo release.
func TestMergePresetNotADependency(t *testing.T) {
	_, err := standardgo.Merge(t.Context(), standardgo.Config, overlay("github.com/nobody/nothing"), fixture(t))
	if err == nil {
		t.Fatal("a preset outside the module graph must be refused")
	}
	if !strings.Contains(err.Error(), "github.com/nobody/nothing") {
		t.Errorf("error should name the module: %v", err)
	}
}

func TestMergePresetModuleWithoutPresetFile(t *testing.T) {
	_, err := standardgo.Merge(t.Context(), standardgo.Config, overlay("github.com/amberpixels/nopreset"), fixture(t))
	if err == nil {
		t.Fatal("a module that publishes no preset must be refused")
	}
	if !strings.Contains(err.Error(), standardgo.PresetFile) {
		t.Errorf("error should name the file that is missing: %v", err)
	}
}

func TestMergePresetsNeedAResolver(t *testing.T) {
	_, err := standardgo.Merge(
		t.Context(),
		standardgo.Config,
		overlay(presetModule),
		standardgo.Project{Module: testModule},
	)
	if err == nil {
		t.Fatal("presets cannot be resolved without a resolver")
	}
}

// TestMergeRejectsPresetOverlap pins the conflict rule: a project that names a
// preset and also hand-writes what the preset provides is refused, and told
// which preset already owns the key rather than being pointed at the shared
// ruleset, which never configured depguard at all.
func TestMergeRejectsPresetOverlap(t *testing.T) {
	ov := append(overlay(presetModule),
		"linters:\n  settings:\n    depguard:\n      rules:\n        mine:\n          files: [\"**/x/**\"]\n"...)

	_, err := standardgo.Merge(t.Context(), standardgo.Config, ov, fixture(t))
	if err == nil {
		t.Fatal("a preset and a hand-written block for the same linter must not both apply")
	}
	if !strings.Contains(err.Error(), presetModule) {
		t.Errorf("error should name the preset that owns the key: %v", err)
	}
}

func TestModulePath(t *testing.T) {
	mod, err := standardgo.ModulePath(".")
	if err != nil {
		t.Fatalf("reading our own go.mod should work: %v", err)
	}
	if mod != "github.com/amberpixels/standardgo" {
		t.Errorf("got module path %q", mod)
	}
}

// TestModulePathWithoutGoMod pins the empty-not-error contract: a directory with
// no module is only a problem for a preset that needs the path, and that refusal
// belongs to the preset, with a message naming it.
func TestModulePathWithoutGoMod(t *testing.T) {
	mod, err := standardgo.ModulePath(t.TempDir())
	if err != nil {
		t.Fatalf("a missing go.mod is not an error here: %v", err)
	}
	if mod != "" {
		t.Errorf("got module path %q, want empty", mod)
	}
}

// TestModuleDirResolvesOwnDependency exercises the real resolver against a
// module this repo actually requires.
func TestModuleDirResolvesOwnDependency(t *testing.T) {
	dir, err := standardgo.ModuleDir(t.Context(), "github.com/amberpixels/k1")
	if err != nil {
		t.Fatalf("resolving a required module should work: %v", err)
	}
	if !strings.Contains(dir, "k1") {
		t.Errorf("got directory %q", dir)
	}
}

func TestModuleDirRejectsNonDependency(t *testing.T) {
	_, err := standardgo.ModuleDir(t.Context(), "github.com/nobody/nothing")
	if err == nil {
		t.Fatal("a module outside the graph must not resolve")
	}
	if !strings.Contains(err.Error(), "go get") {
		t.Errorf("error should say how to fix it: %v", err)
	}
}

func containsAny(haystack []any, want string) bool {
	for _, v := range haystack {
		if v == want {
			return true
		}
	}

	return false
}
