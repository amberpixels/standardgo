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

# fix what can be fixed, then reformat (this order - fixers can leave imports dangling)
go tool standardgo ./... --fix
go tool standardgo fmt ./...

# what is actually enabled
go tool standardgo linters
```

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

## Custom Linters

Alongside the upstream linters, standardgo ships amberpixels' own:

- **[lostfield](https://github.com/amberpixels/lostfield)** - validates Go type-converter
  functions so no field is silently dropped on the way from one struct to another.

These are golangci-lint module plugins, linked into this binary via blank imports. Go links
statically, so a project **cannot** add a plugin to an already-built standardgo - the plugin
has to live here. In exchange you never touch `.custom-gcl.yml` or run `golangci-lint
custom`: standardgo already is the custom binary that mechanism exists to produce.

## How It Works

`.golangci.yml` is embedded with `go:embed`. On each run standardgo merges `.standardgo.yml`
if present, writes the result to a temp file named after its content hash, and hands it to
golangci-lint's `commands.Execute`, which is linked into this binary rather than shelled
out to.

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
