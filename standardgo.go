// Package standardgo is an unconfigurable golangci-lint configuration for
// amberpixels Go projects.
//
// The ruleset lives in one place, the .golangci.yml embedded below, and ships
// as a binary rather than a file to copy, so adopting projects carry no
// .golangci.yml of their own and have nothing to drift.
//
// golangci-lint has no config inheritance (no extends, no remote configs), so
// a shared ruleset has to be distributed rather than referenced. standard (JS)
// and standardrb (Ruby) distribute theirs too, though their engines can inherit.
package standardgo

import (
	_ "embed"

	// Custom linters are compiled in: Go links statically, so a project cannot
	// add a plugin to a built binary. Each registers with the engine from init().
	// Import /plugin, not the module root: the root holds the analyzer alone and
	// registers nothing, so libraries importing lostfield do not carry the plugin.
	_ "github.com/amberpixels/lostfield/plugin"
)

// Config is the locked golangci-lint configuration.
//
//go:embed .golangci.yml
var Config []byte
