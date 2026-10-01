// Package skill distributes the canonical Nodex Agent Skill.
//
// The skill text is embedded in this package. Every caller receives those
// same bytes. Targets differ only by the relative directory where the
// document is installed. This package does not discover a project root,
// resolve a home directory, read source, parse comments, or match an ignore
// policy. The caller supplies the base directory, which is either a project
// root or a user home directory.
package skill

import (
	"bytes"
	_ "embed"
)

// Marker is the stable ownership comment stored in the canonical skill.
//
// A regular file that contains Marker is a Nodex-managed skill. Install may
// replace it. Uninstall may remove it. A file that does not contain Marker
// is left unchanged. Marker is not a content hash, a timestamp, or a provider
// name. It stays the same when the skill text changes.
const Marker = "<!-- nodex-managed-skill:v1 -->"

// skillDocument is the canonical SKILL.md embedded in the binary.
//
//go:embed assets/nodex/SKILL.md
var skillDocument []byte

// Document returns a copy of the canonical skill bytes.
//
// The copy includes the embedded file's final newline. Changing the returned
// slice does not change later installs. The bytes are the embedded asset.
// They are not read from a repository checkout and they are not rewritten
// for a target.
func Document() []byte {
	return bytes.Clone(skillDocument)
}
