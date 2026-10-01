package index

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/edalca/nodex/internal/syntax"
)

const (
	// SchemaVersion is the only snapshot schema this package writes or accepts.
	SchemaVersion = 1

	// IndexDir is the generated index directory relative to the project root.
	IndexDir = ".nodex/index"

	// SnapshotPath is the logical path of the snapshot commit marker.
	SnapshotPath = IndexDir + "/snapshot.json"

	// CommentsPath is the logical path of the persisted comments.
	CommentsPath = IndexDir + "/comments.jsonl"

	// DeclarationsPath is the logical path of the persisted declarations.
	DeclarationsPath = IndexDir + "/declarations.jsonl"

	digestPrefix = "sha256:"
)

var (
	// ErrInvalidDigest is returned when a digest is not "sha256:" followed
	// by 64 lowercase hexadecimal digits.
	ErrInvalidDigest = errors.New("digest is invalid")

	// ErrDuplicateSource is returned when two source fingerprints use one path.
	ErrDuplicateSource = errors.New("duplicate source path")

	// ErrUnsortedSources is returned when persisted sources are not ordered by path.
	ErrUnsortedSources = errors.New("sources are not ordered by path")

	// ErrEmptyLanguage is returned when a source or comment language is empty.
	ErrEmptyLanguage = errors.New("language is empty")
)

// Source is the fingerprint of one included supported source file.
//
// Path is a canonical logical project-relative path. Language is the
// language recognized for that path. Digest is the SHA-256 identity of the
// exact source bytes, in the form produced by DigestBytes. The fingerprint
// does not include a modification time, a file size by itself, or an
// absolute path.
type Source struct {
	Path     string
	Language syntax.Language
	Digest   string
}

// DigestBytes returns the SHA-256 identity of content.
//
// The form is "sha256:" and 64 lowercase hexadecimal digits. content is
// hashed exactly. Nil and empty content have the same digest. Line endings
// are not rewritten.
func DigestBytes(content []byte) string {
	sum := sha256.Sum256(content)
	var b strings.Builder
	b.Grow(len(digestPrefix) + hex.EncodedLen(len(sum)))
	b.WriteString(digestPrefix)
	b.WriteString(hex.EncodeToString(sum[:]))
	return b.String()
}

// Snapshot is the generation identity stored in snapshot.json.
//
// Schema is SchemaVersion. PolicyIdentity is the deterministic identity of
// the effective ignore policy. CommentsDigest is the SHA-256 identity of the
// exact comments.jsonl bytes. CommentCount is the number of comment records
// in that file. DeclarationsDigest is the SHA-256 identity of the exact
// declarations.jsonl bytes. DeclarationCount is the number of declaration
// records in that file. Sources lists every included supported source file
// and no other file, ordered by logical path.
type Snapshot struct {
	Schema             int
	PolicyIdentity     string
	CommentsDigest     string
	CommentCount       int
	DeclarationsDigest string
	DeclarationCount   int
	Sources            []Source
}

// Current reports whether snap was produced from policyIdentity and sources.
//
// A snapshot is current only when the policy identity is equal and the
// source fingerprints are equal. Fingerprints are ordered by logical path
// before the comparison, so the order of sources does not matter. Comment
// text, declaration facts, the derived-file digests and counts, and
// modification times are not compared.
func Current(snap Snapshot, policyIdentity string, sources []Source) bool {
	if snap.PolicyIdentity != policyIdentity {
		return false
	}
	left := append([]Source(nil), snap.Sources...)
	right := append([]Source(nil), sources...)
	slices.SortStableFunc(left, compareSourcePath)
	slices.SortStableFunc(right, compareSourcePath)
	return slices.Equal(left, right)
}

func compareSourcePath(a, b Source) int {
	return cmp.Compare(a.Path, b.Path)
}

func validateDigest(text string) error {
	if len(text) != len(digestPrefix)+sha256.Size*2 || !strings.HasPrefix(text, digestPrefix) {
		return ErrInvalidDigest
	}
	rest := text[len(digestPrefix):]
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return ErrInvalidDigest
		}
	}
	return nil
}

func orderedSources(sources []Source) ([]Source, error) {
	ordered := make([]Source, len(sources))
	copy(ordered, sources)
	slices.SortStableFunc(ordered, compareSourcePath)
	if err := validateSourceSet(ordered); err != nil {
		return nil, err
	}
	return ordered, nil
}

func validateSourceSet(sources []Source) error {
	for i, src := range sources {
		if err := validateSource(src); err != nil {
			return fmt.Errorf("source [%d] path %q: %w", i, src.Path, err)
		}
		if i == 0 {
			continue
		}
		prev := sources[i-1].Path
		if prev == src.Path {
			return fmt.Errorf("%w: %s", ErrDuplicateSource, src.Path)
		}
		if prev > src.Path {
			return ErrUnsortedSources
		}
	}
	return nil
}

func validateSource(src Source) error {
	if err := logicalPathError(src.Path); err != nil {
		return err
	}
	if !utf8.ValidString(src.Path) {
		return errors.New("path is not valid UTF-8")
	}
	if src.Language == "" {
		return ErrEmptyLanguage
	}
	if !utf8.ValidString(string(src.Language)) {
		return errors.New("language is not valid UTF-8")
	}
	return validateDigest(src.Digest)
}
