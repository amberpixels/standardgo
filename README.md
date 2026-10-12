<p align="center">
  <img src="logo.svg" alt="standardgo" width="560">
</p>

<div align="center">

[![CI](https://github.com/amberpixels/standardgo/actions/workflows/ci.yml/badge.svg)](https://github.com/amberpixels/standardgo/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/amberpixels/standardgo)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-yellow.svg)](LICENSE)

</div>

---

**One ruleset. No config files. No arguments.** standardgo is an unconfigurable
golangci-lint ruleset for amberpixels Go projects, shipped as a binary.

Every project that copies a `.golangci.yml` ends up with a different one: placeholders go
unreplaced, linters get commented out, and a year later no two repos agree on what "clean"
means.

standardgo removes the file. The ruleset - 46 linters and 3 formatters - is compiled into
this binary along with the golangci-lint engine itself, so a project pins both with one
line in `go.mod` and carries no lint config of its own.

It follows [standardrb](https://github.com/standardrb/standard) and
[standard](https://github.com/standard/standard): the config is not something you extend,
it is something you install.

## Install

```sh
go get -tool github.com/amberpixels/standardgo/cmd/standardgo
```

That writes a `tool` directive to your `go.mod`, which survives `go mod tidy` and pins the
ruleset like any other dependency. There is nothing to install on the machine, not even
golangci-lint: it ships inside the binary.

Delete your `.golangci.yml` after adopting. It will not be read.

## Quick Start

```sh
# lint the current module
go tool standardgo

# fix everything fixable, formatting included
go tool standardgo ./... --fix

# formatting only - a faster subset of --fix, good for editor-on-save
go tool standardgo fmt ./...

# what is actually enabled
go tool standardgo linters
```

`run` reports formatting problems as ordinary issues, and `--fix` applies formatter
and linter fixes alike, including removing an import a fixer just orphaned. So `--fix`
is a superset of `fmt`, with no ordering to remember. `fmt` skips the linters and costs
about half as much.

To upgrade the rules, run this in each repo:

```sh
go get -tool github.com/amberpixels/standardgo/cmd/standardgo@latest
```

## Configuration

There isn't any. `--config` is refused:

```
standardgo: --config is not accepted; the ruleset ships with this binary.
Project-local additions go in .standardgo.yml
```

A project may **add** to the ruleset via `.standardgo.yml`, never weaken it:

```yaml
# enable linters the shared ruleset does not
linters:
  enable:
    - dupl

  # configure linters the shared ruleset leaves unconfigured
  settings:
    depguard:
      rules:
        layering:
          files: ["**/internal/business/**"]
          deny:
            - pkg: github.com/amberpixels/myapp/internal/application
              desc: "business must not import application"

# the only way to get softer, and only under a path
ignore:
  - path: "internal/legacy/"
    linters: [gosec, funcorder]
```

Anything the shared ruleset already configures is off limits in both directions: making a
locked linter stricter is refused as firmly as making it softer:

```
standardgo: .standardgo.yml may not override settings for "gocritic",
which the shared ruleset already configures
```

"Stricter" has no definition for an arbitrary setting (is a different `gocritic` check set
stricter?), while "already configured" is decidable, so that is the line.

## Presets

A preset gives a convention shared across repos one definition instead of a copy per
project. It is a `standardgo-preset.yml` published by a module you already depend on:

```sh
go get github.com/amberpixels/dcba
```

```yaml
# .standardgo.yml
presets:
  - github.com/amberpixels/dcba
```

Those two lines expand to the full depguard ruleset enforcing
[DCBA](https://github.com/amberpixels/dcba) layering (three rules, ten deny entries) and
enable `depguard`. Any `${MODULE}` in the preset is rewritten to the module path from your
`go.mod`. A shared layering rule needs this: depguard matches deny entries as import-path
**prefixes** with no globbing, so the rule cannot name the packages it forbids until it
knows the module it lands in.

standardgo keeps no list of presets. It resolves the module you name through **your**
module graph, so a preset is versioned where it is defined and upgraded with `go get`, not
with a standardgo release. It is pinned in your `go.mod`, checksummed in your `go.sum` and
served from the module cache, so it cannot change what your project enforces without a
diff someone approved. Naming a module you have not required is refused, with the `go get`
to fix it.

`go mod tidy` drops a requirement nothing imports, so blank import the preset module
somewhere. The package that follows the convention is the natural place; dcba's
`internal/doc.go` template does this.

A preset goes through the same merge as `.standardgo.yml`, under the same rules: it cannot
disable a linter, override what the shared ruleset configures, or nest another preset.

Naming a preset **and** hand-writing what it provides is refused rather than merged, so a
project never half-inherits a convention:

```
standardgo: .standardgo.yml may not override settings for "depguard",
which preset "github.com/amberpixels/dcba" already configures
```

### Publishing one

Put a `standardgo-preset.yml` at the root of a module, written exactly like a
`.standardgo.yml`, and use `${MODULE}` wherever the consuming project's module path
belongs. There is no registry to add it to.

## Custom Linters

Alongside the upstream linters, standardgo ships amberpixels' own:

- **[lostfield](https://github.com/amberpixels/lostfield)** - validates Go type-converter
  functions so no field is silently dropped on the way from one struct to another.

These are golangci-lint module plugins, linked into this binary via blank imports. Go links
statically, so a project **cannot** add a plugin to a built standardgo; the plugin has to
live here. In exchange you never touch `.custom-gcl.yml` or run `golangci-lint custom`:
standardgo already is the custom binary that mechanism produces.

### Configuring a bundled plugin

Some plugins need per-project facts no shared default can guess. For `lostfield`, that is
what counts as a converter and which fields a persistence layer manages. So a project may
supply settings for a plugin the shared ruleset declares but does not tune:

```yaml
# .standardgo.yml
linters:
  settings:
    custom:
      lostfield:
        settings:
          only-converters: ["To*", "From*"]
          exclude-fields: ["^ID$", CreatedAt, UpdatedAt, DeletedAt]
          ignore-tags: ['lostfield:"ignore"']
```

These are YAML lists: the comma-separated form (`exclude-fields: 'A,B'`) is `lostfield`'s
CLI-flag syntax and is invalid here. Unknown keys and out-of-range values are rejected at
startup, not ignored, so a typo fails loudly.

`type`, `path` and `description` stay locked: they decide what code backs the plugin, and
that code is compiled into this binary. Once the shared ruleset tunes a plugin, that plugin
is closed, as with any linter it configures.

## How It Works

`.golangci.yml` is embedded with `go:embed`. On each run standardgo merges `.standardgo.yml`
if present (presets first, then the project's own additions), writes the result to a temp
file named after its content hash, and hands it to golangci-lint's `commands.Execute`,
linked into this binary rather than shelled out to.

Bundling the engine makes results reproducible: there is no second copy of golangci-lint,
so a laptop and CI cannot lint differently. The cost is that `go get -tool` pulls every
linter's dependencies into your `go.mod` as indirect requirements. They never enter your
build graph. If that noise bothers you, put the `tool` directive in a nested
`tools/go.mod`, build the binary from there, and run it from the repo root.

Engine upgrades arrive as Dependabot PRs. CI on this repo fails one if the new engine
deprecates a linter the ruleset enables, and reports any new linter the ruleset has not yet
taken a position on.

## Requirements

Go 1.26 or newer.

## Feedback

standardgo is a solo, opinionated project - but if you stumbled upon it and have ideas,
questions, or bug reports, an [issue](https://github.com/amberpixels/standardgo/issues) is always welcome :)

## License

[MIT](LICENSE) © [amberpixels](https://amberpixels.io)
