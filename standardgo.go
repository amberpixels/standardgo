// Package standardgo is an unconfigurable golangci-lint configuration for
// amberpixels Go projects.
//
// The ruleset lives in exactly one place — the .golangci.yml embedded below —
// and is delivered as a binary rather than a file to copy. Projects that adopt
// standardgo carry no .golangci.yml of their own, so there is nothing to drift.
//
// golangci-lint has no config inheritance (no extends, no remote configs), so
// a shared ruleset cannot be referenced; it has to be distributed. That is the
// same design standard (JS) and standardrb (Ruby) chose deliberately, even
// though their engines do support inheritance.
package standardgo

import (
	_ "embed"

	// amberpixels custom linters are compiled in here. Go links statically, so a
	// consuming project cannot add a golangci-lint plugin to an already-built
	// binary - the plugin has to live in this one. Each import registers itself
	// with the bundled engine via init(), which is why they are blank imports.
	//
	// This replaces the .custom-gcl.yml / `golangci-lint custom` dance: standardgo
	// already is the custom binary that mechanism exists to produce.
	//
	// Import /plugin, not the module root: the root holds the analyzer alone and
	// registers nothing, so that libraries importing lostfield do not carry this.
	_ "github.com/amberpixels/lostfield/plugin"
)

// Config is the locked golangci-lint configuration.
//
//go:embed .golangci.yml
var Config []byte
