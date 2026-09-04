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

Every project that copies a `.golangci.yml` eventually has a different one. Placeholders
go unreplaced, a linter gets commented out during a bad afternoon, and a year later no two
repos agree on what "clean" means.

standardgo removes the file. The ruleset - 46 linters and 4 formatters - is compiled into
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
ruleset like any other dependency. Nothing needs to be installed on the machine - not even
golangci-lint, which ships inside the binary.

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
and linter fixes alike - including cleaning up an import a fixer just orphaned. So
`--fix` is a superset of `fmt`, and there is no ordering to remember. `fmt` exists
because it skips the linters entirely and costs about half as much.

Upgrading the rules everywhere is one command per repo:

```sh
go get -tool github.com/amberpixels/standardgo/cmd/standardgo@latest
```

## Configuration

There isn't any, and that is the point. `--config` is refused:

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

Anything the shared ruleset already configures is off limits in both directions - making a
locked linter stricter is refused just as firmly as making it softer:

```
standardgo: .standardgo.yml may not override settings for "gocritic",
which the shared ruleset already configures
```

That sounds arbitrary until you try to define "stricter" for an arbitrary setting. Is a
different `gocritic` check set stricter? Nobody can say. "Already configured" is decidable,
so that is the line.

## Presets

A convention shared across repos should have one definition, not one copy per project. A
preset is a `standardgo-preset.yml` published by a module you already depend on:

```sh
go get github.com/amberpixels/dcba
```

```yaml
# .standardgo.yml
presets:
  - github.com/amberpixels/dcba
```

Those two lines expand to the full depguard ruleset enforcing
[DCBA](https://github.com/amberpixels/dcba) layering - three rules, ten deny entries - and
enable `depguard` along with it. Any `${MODULE}` in the preset is rewritten to the module
path from your `go.mod`, which is what makes such a preset shareable at all: depguard
matches deny entries as import-path **prefixes** with no globbing, so a layering rule
cannot name the packages it forbids until it knows the module it lands in.

standardgo does not know what presets exist. It resolves the module path you name through
**your** module graph, so a preset is versioned where it is defined and upgraded with
`go get`, not with a standardgo release. That also means it is pinned in your `go.mod`,
checksummed in your `go.sum`, and served from the module cache: a preset cannot change what
your project enforces without a diff someone approved. Naming a module you have not
required is refused, with the `go get` to fix it.

Because `go mod tidy` drops a requirement nothing imports, a preset module should be blank
imported somewhere - dcba's `internal/doc.go` template does this, which is the natural home
for it: the package that follows the convention declares the dependency on it.

A preset is written in the `.standardgo.yml` shape and goes through the same merge, so it
is bound by the same add-never-override rules: it cannot disable a linter, cannot override
what the shared ruleset configures, and cannot nest another preset.

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
statically, so a project **cannot** add a plugin to an already-built standardgo - the plugin
has to live here. In exchange you never touch `.custom-gcl.yml` or run `golangci-lint
custom`: standardgo already is the custom binary that mechanism exists to produce.

### Configuring a bundled plugin

Some plugins only make sense once a project tells them where to look. `lostfield` is one:
what counts as a converter, and which fields a persistence layer manages, are per-project
facts no shared default can guess. So a project may supply settings for a plugin the shared
ruleset declares but does not tune:

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

Note that these are YAML lists. The comma-separated form (`exclude-fields: 'A,B'`) is
`lostfield`'s CLI-flag syntax and is not valid here. Unknown keys and out-of-range values
are rejected at startup rather than ignored, so a typo fails loudly.

`type`, `path` and `description` stay locked. Settings are an addition; those three decide
what code backs the plugin, and the plugin is compiled into this binary. The same
already-configured rule applies as everywhere else: once the shared ruleset tunes a plugin,
that plugin is closed.

## How It Works

`.golangci.yml` is embedded with `go:embed`. On each run standardgo merges `.standardgo.yml`
if present - presets first, then the project's own additions - writes the result to a temp
file named after its content hash, and hands it to golangci-lint's `commands.Execute`,
which is linked into this binary rather than shelled out to.

Bundling the engine is what makes results reproducible: a project cannot lint differently
on a laptop and in CI, because there is no second copy of golangci-lint to disagree with.
The cost is that `go get -tool` pulls every linter's dependencies into your `go.mod` as
indirect requirements. They never enter your build graph. If that noise bothers you, put
the `tool` directive in a nested `tools/go.mod`, build the binary from there, and run it
from the repo root.

Engine upgrades arrive as ordinary Dependabot PRs. CI on this repo fails such a PR if the
new engine deprecates a linter the ruleset enables, and reports any linter the engine has
gained that the ruleset has not yet taken a position on.

## Requirements

Go 1.26 or newer.

## Feedback

standardgo is a solo, opinionated project - but if you stumbled upon it and have ideas,
questions, or bug reports, an [issue](https://github.com/amberpixels/standardgo/issues) is always welcome :)

## License

[MIT](LICENSE) © [amberpixels](https://amberpixels.io)
