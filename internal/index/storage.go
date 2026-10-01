package index

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/edalca/nodex/internal/syntax"
)

var (
	// ErrAbsent means snapshot.json is not present, so no committed index exists.
	// A comments.jsonl file without that marker is not a committed index.
	ErrAbsent = errors.New("no generated index")

	// ErrCorrupt means the persisted index is malformed, incomplete, or inconsistent.
	// Load returns a nil index with this error. A digest mismatch between
	// snapshot.json and either derived file is corrupt, including when the
	// files come from different generations.
	ErrCorrupt = errors.New("persisted index is corrupt")

	// ErrDigestMismatch means the comments file bytes are not the bytes named
	// by the snapshot's comments digest.
	ErrDigestMismatch = errors.New("comments digest does not match snapshot")

	// ErrDeclarationDigestMismatch means the declarations file bytes are not
	// the bytes named by the snapshot's declarations digest.
	ErrDeclarationDigestMismatch = errors.New("declarations digest does not match snapshot")

	// ErrCountMismatch means the snapshot comment count is not the number of
	// comment records.
	ErrCountMismatch = errors.New("comment count does not match snapshot")

	// ErrDeclarationCountMismatch means the snapshot declaration count is not
	// the number of declaration records.
	ErrDeclarationCountMismatch = errors.New("declaration count does not match snapshot")

	// ErrUnsupportedSchema means the snapshot schema is not SchemaVersion.
	ErrUnsupportedSchema = errors.New("snapshot schema is unsupported")

	// ErrTrailingData means a JSON value is followed by another value.
	ErrTrailingData = errors.New("trailing JSON data")

	// ErrIDSequence means a persisted comment ID is not the canonical ordinal
	// of its position. The first record is C000001 and each next record is
	// the next ordinal.
	ErrIDSequence = errors.New("comment ID is not the canonical ordinal for its position")

	// ErrOutOfOrder means persisted comments are not in canonical index order.
	ErrOutOfOrder = errors.New("comments are not in canonical order")

	// ErrDeclarationIDSequence means a persisted declaration ID is not the
	// canonical ordinal of its position. The first record is D000001 and
	// each next record is the next ordinal.
	ErrDeclarationIDSequence = errors.New("declaration ID is not the canonical ordinal for its position")

	// ErrDeclarationsOutOfOrder means persisted declarations are not in
	// canonical index order.
	ErrDeclarationsOutOfOrder = errors.New("declarations are not in canonical order")

	// ErrDanglingDoc means a declaration names a documentation comment that
	// is not a comment record in the same source.
	ErrDanglingDoc = errors.New("declaration documentation does not name a comment in the same source")

	errMissingField = errors.New("required field is missing")
	errNotRegular   = errors.New("not a regular file")
	errSymlink      = errors.New("symbolic link")
)

// UnknownFieldError reports a JSON field the snapshot schema does not define.
type UnknownFieldError struct {
	Name string
}

// Error reports the unknown field name.
func (e *UnknownFieldError) Error() string {
	if e == nil {
		return "unknown snapshot field"
	}
	return fmt.Sprintf("unknown snapshot field %q", e.Name)
}

// Persist writes idx and its input snapshot beneath workspaceBase/.nodex.
//
// workspaceBase is an existing directory containing the control directory.
// policyIdentity is the identity of the effective ignore policy. sources are
// the included supported files. Their order does not change persisted bytes.
// idx supplies the comments and declarations.
// A nil index is an empty index.
//
// Persist validates and encodes the complete new state before it publishes
// anything. It creates .nodex/ and .nodex/index/ when they are missing. It
// does not create or modify .nodex/ignore.json. Comments and declarations
// are written to temporary files in the index directory, synced, and closed.
// The snapshot is prepared the same way. comments.jsonl is renamed into
// place first, then declarations.jsonl. snapshot.json is renamed into place
// last and is the commit marker.
//
// The renames are not one filesystem transaction. A crash between them can
// leave a derived file from one generation beside a snapshot from another.
// Load rejects that set. On error before the first rename, an existing
// committed index is left in place. Temporary files created by the failed
// call are removed.
func Persist(workspaceBase, policyIdentity string, sources []Source, idx *Index) (Snapshot, error) {
	if workspaceBase == "" {
		return Snapshot{}, errors.New("workspace base is empty")
	}
	if err := validateDigest(policyIdentity); err != nil {
		return Snapshot{}, fmt.Errorf("policy identity: %w", err)
	}
	ordered, err := orderedSources(sources)
	if err != nil {
		return Snapshot{}, err
	}
	entries := entriesOf(idx)
	if err := validateEntries(entries); err != nil {
		return Snapshot{}, err
	}
	decls := declarationsOf(idx)
	if err := validateDeclarations(decls, entries); err != nil {
		return Snapshot{}, err
	}
	commentBytes, err := encodeComments(entries)
	if err != nil {
		return Snapshot{}, err
	}
	declarationBytes, err := encodeDeclarations(decls)
	if err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{
		Schema:             SchemaVersion,
		PolicyIdentity:     policyIdentity,
		CommentsDigest:     DigestBytes(commentBytes),
		CommentCount:       len(entries),
		DeclarationsDigest: DigestBytes(declarationBytes),
		DeclarationCount:   len(decls),
		Sources:            ordered,
	}
	snapshotBytes, err := encodeSnapshot(snap)
	if err != nil {
		return Snapshot{}, err
	}
	if _, err := decodeSnapshot(snapshotBytes); err != nil {
		return Snapshot{}, fmt.Errorf("encoded snapshot: %w", err)
	}
	indexDir, err := ensureIndexDir(workspaceBase)
	if err != nil {
		return Snapshot{}, err
	}
	if err := publish(indexDir, commentBytes, declarationBytes, snapshotBytes); err != nil {
		return Snapshot{}, err
	}
	snap.Sources = append([]Source(nil), ordered...)
	return snap, nil
}

// Load reads the committed index under the workspace base.
//
// When snapshot.json is absent, Load returns a nil index, the zero snapshot,
// and ErrAbsent. A snapshot that exists without comments.jsonl or
// declarations.jsonl, a digest that does not match its file, a count that
// does not match the records, or any record that Build would reject returns
// a nil index and an error wrapping ErrCorrupt. Load does not return a
// partial index.
//
// On success the index is immutable. Entries and Declarations return copies.
func Load(workspaceBase string) (*Index, Snapshot, error) {
	if workspaceBase == "" {
		return nil, Snapshot{}, errors.New("workspace base is empty")
	}
	data, err := readCommitted(workspaceBase, SnapshotPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, Snapshot{}, ErrAbsent
		}
		if errors.Is(err, errSymlink) || errors.Is(err, errNotRegular) {
			return nil, Snapshot{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
		}
		return nil, Snapshot{}, err
	}
	snap, err := decodeSnapshot(data)
	if err != nil {
		return nil, Snapshot{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	comments, err := readCommitted(workspaceBase, CommentsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, Snapshot{}, fmt.Errorf("%w: %s is missing", ErrCorrupt, CommentsPath)
		}
		return nil, Snapshot{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if DigestBytes(comments) != snap.CommentsDigest {
		return nil, Snapshot{}, fmt.Errorf("%w: %w", ErrCorrupt, ErrDigestMismatch)
	}
	entries, err := parseComments(comments)
	if err != nil {
		return nil, Snapshot{}, err
	}
	if len(entries) != snap.CommentCount {
		return nil, Snapshot{}, fmt.Errorf("%w: %w", ErrCorrupt, ErrCountMismatch)
	}
	declarations, err := readCommitted(workspaceBase, DeclarationsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, Snapshot{}, fmt.Errorf("%w: %s is missing", ErrCorrupt, DeclarationsPath)
		}
		return nil, Snapshot{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if DigestBytes(declarations) != snap.DeclarationsDigest {
		return nil, Snapshot{}, fmt.Errorf("%w: %w", ErrCorrupt, ErrDeclarationDigestMismatch)
	}
	decls, err := parseDeclarations(declarations)
	if err != nil {
		return nil, Snapshot{}, err
	}
	if len(decls) != snap.DeclarationCount {
		return nil, Snapshot{}, fmt.Errorf("%w: %w", ErrCorrupt, ErrDeclarationCountMismatch)
	}
	if err := validateDeclarations(decls, entries); err != nil {
		return nil, Snapshot{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	return indexFrom(entries, decls), snap, nil
}

func entriesOf(idx *Index) []Entry {
	if idx == nil || len(idx.entries) == 0 {
		return []Entry{}
	}
	out := make([]Entry, len(idx.entries))
	copy(out, idx.entries)
	return out
}

func declarationsOf(idx *Index) []Declaration {
	if idx == nil || len(idx.decls) == 0 {
		return []Declaration{}
	}
	out := make([]Declaration, len(idx.decls))
	for i, decl := range idx.decls {
		out[i] = copyDeclaration(decl)
	}
	return out
}

func indexFrom(entries []Entry, decls []Declaration) *Index {
	stored := make([]Entry, len(entries))
	copy(stored, entries)
	byID := make(map[uint64]int, len(stored))
	for i, entry := range stored {
		byID[entry.ID.n] = i
	}
	storedDecls := make([]Declaration, len(decls))
	declByID := make(map[uint64]int, len(decls))
	for i, decl := range decls {
		storedDecls[i] = copyDeclaration(decl)
		declByID[decl.ID.n] = i
	}
	return &Index{entries: stored, byID: byID, decls: storedDecls, declByID: declByID}
}

func ensureIndexDir(workspaceBase string) (string, error) {
	nodex := filepath.Join(workspaceBase, ".nodex")
	if err := mkdirPlain(nodex); err != nil {
		return "", err
	}
	indexDir := filepath.Join(nodex, "index")
	if err := mkdirPlain(indexDir); err != nil {
		return "", err
	}
	return indexDir, nil
}

func mkdirPlain(path string) error {
	err := os.Mkdir(path, 0o755)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symbolic link", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}

// publish installs the derived files. comments.jsonl is renamed first,
// declarations.jsonl next, and snapshot.json last. Temporary files stay in
// indexDir until their rename succeeds.
func publish(indexDir string, comments, declarations, snapshot []byte) error {
	commentsTemp, err := writeTemp(indexDir, ".comments-*", comments)
	if err != nil {
		return err
	}
	commentsKept := false
	defer func() {
		if !commentsKept {
			_ = os.Remove(commentsTemp)
		}
	}()
	declarationsTemp, err := writeTemp(indexDir, ".declarations-*", declarations)
	if err != nil {
		return err
	}
	declarationsKept := false
	defer func() {
		if !declarationsKept {
			_ = os.Remove(declarationsTemp)
		}
	}()
	snapshotTemp, err := writeTemp(indexDir, ".snapshot-*", snapshot)
	if err != nil {
		return err
	}
	snapshotKept := false
	defer func() {
		if !snapshotKept {
			_ = os.Remove(snapshotTemp)
		}
	}()

	if err := os.Rename(commentsTemp, filepath.Join(indexDir, "comments.jsonl")); err != nil {
		return err
	}
	commentsKept = true
	if err := os.Rename(declarationsTemp, filepath.Join(indexDir, "declarations.jsonl")); err != nil {
		return err
	}
	declarationsKept = true
	if err := os.Rename(snapshotTemp, filepath.Join(indexDir, "snapshot.json")); err != nil {
		return err
	}
	snapshotKept = true
	// The directory sync publishes the renames. It is not a second transaction
	// around the files. A crash between the renames is still detectable
	// because the snapshot digests will not match the derived files.
	return syncDir(indexDir)
}

func writeTemp(dir, pattern string, data []byte) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	name := f.Name()
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(name)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	remove = false
	return name, nil
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func readCommitted(workspaceBase, logical string) ([]byte, error) {
	current := workspaceBase
	elements := strings.Split(logical, "/")
	var info os.FileInfo
	for i, elem := range elements {
		current = filepath.Join(current, elem)
		var err error
		info, err = os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: %s", errSymlink, logical)
		}
		last := i == len(elements)-1
		if !last {
			if !info.IsDir() {
				return nil, fmt.Errorf("%w: %s", errNotRegular, current)
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: %s", errNotRegular, logical)
		}
	}
	return os.ReadFile(current)
}

type snapshotDTO struct {
	Schema             int         `json:"schema"`
	PolicyIdentity     string      `json:"policy_identity"`
	CommentsDigest     string      `json:"comments_digest"`
	CommentCount       int         `json:"comment_count"`
	DeclarationsDigest string      `json:"declarations_digest"`
	DeclarationCount   int         `json:"declaration_count"`
	Sources            []sourceDTO `json:"sources"`
}

type sourceDTO struct {
	Path     string `json:"path"`
	Language string `json:"language"`
	Digest   string `json:"digest"`
}

type entryDTO struct {
	ID       string   `json:"id"`
	Path     string   `json:"path"`
	Language string   `json:"language"`
	Range    rangeDTO `json:"range"`
	Text     string   `json:"text"`
}

type declarationDTO struct {
	ID       string   `json:"id"`
	Path     string   `json:"path"`
	Language string   `json:"language"`
	Kind     string   `json:"kind"`
	Names    []string `json:"names"`
	Range    rangeDTO `json:"range"`
	Doc      *string  `json:"doc"`
}

type rangeDTO struct {
	Start positionDTO `json:"start"`
	End   positionDTO `json:"end"`
}

type positionDTO struct {
	Offset int `json:"offset"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

func encodeSnapshot(snap Snapshot) ([]byte, error) {
	dto := snapshotDTO{
		Schema:             snap.Schema,
		PolicyIdentity:     snap.PolicyIdentity,
		CommentsDigest:     snap.CommentsDigest,
		CommentCount:       snap.CommentCount,
		DeclarationsDigest: snap.DeclarationsDigest,
		DeclarationCount:   snap.DeclarationCount,
		Sources:            make([]sourceDTO, len(snap.Sources)),
	}
	for i, src := range snap.Sources {
		dto.Sources[i] = sourceDTO{Path: src.Path, Language: string(src.Language), Digest: src.Digest}
	}
	return encodeJSON(dto)
}

func encodeComments(entries []Entry) ([]byte, error) {
	if len(entries) == 0 {
		return []byte{}, nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, entry := range entries {
		dto := entryDTO{
			ID:       entry.ID.String(),
			Path:     entry.Path,
			Language: string(entry.Language),
			Range: rangeDTO{
				Start: positionDTO{Offset: entry.Range.Start.Offset, Line: entry.Range.Start.Line, Column: entry.Range.Start.Column},
				End:   positionDTO{Offset: entry.Range.End.Offset, Line: entry.Range.End.Line, Column: entry.Range.End.Column},
			},
			Text: entry.Text,
		}
		if err := enc.Encode(dto); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func encodeDeclarations(decls []Declaration) ([]byte, error) {
	if len(decls) == 0 {
		return []byte{}, nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, decl := range decls {
		names := decl.Names
		if names == nil {
			names = []string{}
		}
		dto := declarationDTO{
			ID:       decl.ID.String(),
			Path:     decl.Path,
			Language: string(decl.Language),
			Kind:     string(decl.Kind),
			Names:    names,
			Range: rangeDTO{
				Start: positionDTO{Offset: decl.Range.Start.Offset, Line: decl.Range.Start.Line, Column: decl.Range.Start.Column},
				End:   positionDTO{Offset: decl.Range.End.Offset, Line: decl.Range.End.Line, Column: decl.Range.End.Column},
			},
		}
		if decl.Doc.Valid() {
			text := decl.Doc.String()
			dto.Doc = &text
		}
		if err := enc.Encode(dto); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeSnapshot(data []byte) (Snapshot, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return Snapshot{}, errors.New("snapshot is empty")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var fields map[string]json.RawMessage
	if err := dec.Decode(&fields); err != nil {
		return Snapshot{}, err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return Snapshot{}, ErrTrailingData
		}
		return Snapshot{}, fmt.Errorf("%w: %v", ErrTrailingData, err)
	}
	if err := rejectUnknown(fields, "schema", "policy_identity", "comments_digest", "comment_count", "declarations_digest", "declaration_count", "sources"); err != nil {
		return Snapshot{}, err
	}
	for _, key := range []string{"schema", "policy_identity", "comments_digest", "comment_count", "declarations_digest", "declaration_count", "sources"} {
		if _, ok := fields[key]; !ok {
			return Snapshot{}, fmt.Errorf("%w: %s", errMissingField, key)
		}
	}
	if err := exactSchema(fields["schema"]); err != nil {
		return Snapshot{}, err
	}
	policy, err := decodeString(fields["policy_identity"])
	if err != nil {
		return Snapshot{}, fmt.Errorf("policy_identity: %w", err)
	}
	if err := validateDigest(policy); err != nil {
		return Snapshot{}, fmt.Errorf("policy_identity: %w", err)
	}
	commentsDigest, err := decodeString(fields["comments_digest"])
	if err != nil {
		return Snapshot{}, fmt.Errorf("comments_digest: %w", err)
	}
	if err := validateDigest(commentsDigest); err != nil {
		return Snapshot{}, fmt.Errorf("comments_digest: %w", err)
	}
	count, err := exactNonNegativeInt(fields["comment_count"])
	if err != nil {
		return Snapshot{}, fmt.Errorf("comment_count: %w", err)
	}
	declarationsDigest, err := decodeString(fields["declarations_digest"])
	if err != nil {
		return Snapshot{}, fmt.Errorf("declarations_digest: %w", err)
	}
	if err := validateDigest(declarationsDigest); err != nil {
		return Snapshot{}, fmt.Errorf("declarations_digest: %w", err)
	}
	declarationCount, err := exactNonNegativeInt(fields["declaration_count"])
	if err != nil {
		return Snapshot{}, fmt.Errorf("declaration_count: %w", err)
	}
	sources, err := decodeSources(fields["sources"])
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		Schema:             SchemaVersion,
		PolicyIdentity:     policy,
		CommentsDigest:     commentsDigest,
		CommentCount:       count,
		DeclarationsDigest: declarationsDigest,
		DeclarationCount:   declarationCount,
		Sources:            sources,
	}, nil
}

func exactSchema(raw json.RawMessage) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte(strconv.Itoa(SchemaVersion))) {
		return nil
	}
	return ErrUnsupportedSchema
}

func exactNonNegativeInt(raw json.RawMessage) (int, error) {
	token := bytes.TrimSpace(raw)
	if len(token) == 0 || token[0] < '0' || token[0] > '9' {
		return 0, errors.New("must be an integer")
	}
	for _, c := range token {
		if c < '0' || c > '9' {
			return 0, errors.New("must be an integer")
		}
	}
	n, err := strconv.Atoi(string(token))
	if err != nil {
		return 0, errors.New("must be an integer")
	}
	return n, nil
}

func decodeString(raw json.RawMessage) (string, error) {
	literal := bytes.TrimSpace(raw)
	if len(literal) == 0 || literal[0] != '"' {
		return "", errors.New("must be a string")
	}
	var text string
	if err := json.Unmarshal(literal, &text); err != nil {
		return "", errors.New("must be a string")
	}
	return text, nil
}

func rejectUnknown(fields map[string]json.RawMessage, allowed ...string) error {
	ok := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		ok[key] = struct{}{}
	}
	var unknown []string
	for key := range fields {
		if _, allowed := ok[key]; !allowed {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return &UnknownFieldError{Name: unknown[0]}
}

func decodeSources(raw json.RawMessage) ([]Source, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, errors.New("sources is null")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, errors.New("sources must be an array")
	}
	sources := make([]Source, 0, len(items))
	for i, item := range items {
		src, err := decodeSource(item)
		if err != nil {
			return nil, fmt.Errorf("sources[%d]: %w", i, err)
		}
		sources = append(sources, src)
	}
	if err := validateSourceSet(sources); err != nil {
		return nil, err
	}
	return sources, nil
}

func decodeSource(raw json.RawMessage) (Source, error) {
	fields, err := decodeObject(raw)
	if err != nil {
		return Source{}, err
	}
	if err := rejectUnknown(fields, "path", "language", "digest"); err != nil {
		return Source{}, err
	}
	for _, key := range []string{"path", "language", "digest"} {
		if _, ok := fields[key]; !ok {
			return Source{}, fmt.Errorf("%w: %s", errMissingField, key)
		}
	}
	pathText, err := decodeString(fields["path"])
	if err != nil {
		return Source{}, fmt.Errorf("path: %w", err)
	}
	language, err := decodeString(fields["language"])
	if err != nil {
		return Source{}, fmt.Errorf("language: %w", err)
	}
	digest, err := decodeString(fields["digest"])
	if err != nil {
		return Source{}, fmt.Errorf("digest: %w", err)
	}
	src := Source{Path: pathText, Language: syntax.Language(language), Digest: digest}
	if err := validateSource(src); err != nil {
		return Source{}, err
	}
	return src, nil
}

func parseComments(data []byte) ([]Entry, error) {
	if len(data) == 0 {
		return []Entry{}, nil
	}
	if data[len(data)-1] != '\n' {
		return nil, fmt.Errorf("%w: comments file does not end with a newline", ErrCorrupt)
	}
	entries := make([]Entry, 0)
	lineNo := 0
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			return nil, fmt.Errorf("%w: comments file does not end with a newline", ErrCorrupt)
		}
		line := data[:i]
		data = data[i+1:]
		lineNo++
		if len(bytes.TrimSpace(line)) == 0 {
			return nil, fmt.Errorf("%w: comments line %d is blank", ErrCorrupt, lineNo)
		}
		entry, err := parseCommentLine(line)
		if err != nil {
			return nil, fmt.Errorf("%w: comments line %d: %w", ErrCorrupt, lineNo, err)
		}
		entries = append(entries, entry)
	}
	if err := validateEntries(entries); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	return entries, nil
}

func parseCommentLine(line []byte) (Entry, error) {
	dec := json.NewDecoder(bytes.NewReader(line))
	var fields map[string]json.RawMessage
	if err := dec.Decode(&fields); err != nil {
		return Entry{}, err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return Entry{}, ErrTrailingData
		}
		return Entry{}, fmt.Errorf("%w: %v", ErrTrailingData, err)
	}
	if err := rejectUnknown(fields, "id", "path", "language", "range", "text"); err != nil {
		return Entry{}, err
	}
	for _, key := range []string{"id", "path", "language", "range", "text"} {
		if _, ok := fields[key]; !ok {
			return Entry{}, fmt.Errorf("%w: %s", errMissingField, key)
		}
	}
	idText, err := decodeString(fields["id"])
	if err != nil {
		return Entry{}, fmt.Errorf("id: %w", err)
	}
	id, err := ParseID(idText)
	if err != nil {
		return Entry{}, err
	}
	pathText, err := decodeString(fields["path"])
	if err != nil {
		return Entry{}, fmt.Errorf("path: %w", err)
	}
	language, err := decodeString(fields["language"])
	if err != nil {
		return Entry{}, fmt.Errorf("language: %w", err)
	}
	rng, err := decodeRange(fields["range"])
	if err != nil {
		return Entry{}, fmt.Errorf("range: %w", err)
	}
	text, err := decodeString(fields["text"])
	if err != nil {
		return Entry{}, fmt.Errorf("text: %w", err)
	}
	return Entry{
		ID:       id,
		Path:     pathText,
		Language: syntax.Language(language),
		Range:    rng,
		Text:     text,
	}, nil
}

func decodeRange(raw json.RawMessage) (syntax.Range, error) {
	fields, err := decodeObject(raw)
	if err != nil {
		return syntax.Range{}, err
	}
	if err := rejectUnknown(fields, "start", "end"); err != nil {
		return syntax.Range{}, err
	}
	for _, key := range []string{"start", "end"} {
		if _, ok := fields[key]; !ok {
			return syntax.Range{}, fmt.Errorf("%w: %s", errMissingField, key)
		}
	}
	start, err := decodePosition(fields["start"])
	if err != nil {
		return syntax.Range{}, fmt.Errorf("start: %w", err)
	}
	end, err := decodePosition(fields["end"])
	if err != nil {
		return syntax.Range{}, fmt.Errorf("end: %w", err)
	}
	rng := syntax.Range{Start: start, End: end}
	if err := validateRange(rng); err != nil {
		return syntax.Range{}, err
	}
	return rng, nil
}

func decodePosition(raw json.RawMessage) (syntax.Position, error) {
	fields, err := decodeObject(raw)
	if err != nil {
		return syntax.Position{}, err
	}
	if err := rejectUnknown(fields, "offset", "line", "column"); err != nil {
		return syntax.Position{}, err
	}
	for _, key := range []string{"offset", "line", "column"} {
		if _, ok := fields[key]; !ok {
			return syntax.Position{}, fmt.Errorf("%w: %s", errMissingField, key)
		}
	}
	offset, err := exactNonNegativeInt(fields["offset"])
	if err != nil {
		return syntax.Position{}, fmt.Errorf("offset: %w", err)
	}
	line, err := exactNonNegativeInt(fields["line"])
	if err != nil {
		return syntax.Position{}, fmt.Errorf("line: %w", err)
	}
	column, err := exactNonNegativeInt(fields["column"])
	if err != nil {
		return syntax.Position{}, fmt.Errorf("column: %w", err)
	}
	return syntax.Position{Offset: offset, Line: line, Column: column}, nil
}

func decodeObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("must be an object")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var fields map[string]json.RawMessage
	if err := dec.Decode(&fields); err != nil {
		return nil, err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, ErrTrailingData
		}
		return nil, fmt.Errorf("%w: %v", ErrTrailingData, err)
	}
	return fields, nil
}

type positionKey struct {
	path string
	rng  syntax.Range
}

func validateEntries(entries []Entry) error {
	seen := make(map[positionKey]int, len(entries))
	for i, entry := range entries {
		want := uint64(i) + 1
		if !entry.ID.Valid() || entry.ID.n != want {
			return fmt.Errorf("comment [%d]: %w", i, ErrIDSequence)
		}
		if err := logicalPathError(entry.Path); err != nil {
			return fmt.Errorf("comment [%d] path %q: %w", i, entry.Path, err)
		}
		if !utf8.ValidString(entry.Path) || !utf8.ValidString(entry.Text) || !utf8.ValidString(string(entry.Language)) {
			return fmt.Errorf("comment [%d]: text is not valid UTF-8", i)
		}
		if entry.Language == "" {
			return fmt.Errorf("comment [%d]: %w", i, ErrEmptyLanguage)
		}
		if err := validateRange(entry.Range); err != nil {
			return fmt.Errorf("comment [%d]: %w", i, err)
		}
		key := positionKey{path: entry.Path, rng: entry.Range}
		if prev, ok := seen[key]; ok {
			return &DuplicatePositionError{Path: entry.Path, Index: prev, Other: i, Range: entry.Range}
		}
		seen[key] = i
		if i > 0 && compareEntry(entries[i-1], entry) >= 0 {
			return fmt.Errorf("comment [%d]: %w", i, ErrOutOfOrder)
		}
	}
	return nil
}

func compareEntry(a, b Entry) int {
	return compareCollected(
		collected{path: a.Path, rng: a.Range},
		collected{path: b.Path, rng: b.Range},
	)
}

func parseDeclarations(data []byte) ([]Declaration, error) {
	if len(data) == 0 {
		return []Declaration{}, nil
	}
	if data[len(data)-1] != '\n' {
		return nil, fmt.Errorf("%w: declarations file does not end with a newline", ErrCorrupt)
	}
	decls := make([]Declaration, 0)
	lineNo := 0
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			return nil, fmt.Errorf("%w: declarations file does not end with a newline", ErrCorrupt)
		}
		line := data[:i]
		data = data[i+1:]
		lineNo++
		if len(bytes.TrimSpace(line)) == 0 {
			return nil, fmt.Errorf("%w: declarations line %d is blank", ErrCorrupt, lineNo)
		}
		decl, err := parseDeclarationLine(line)
		if err != nil {
			return nil, fmt.Errorf("%w: declarations line %d: %w", ErrCorrupt, lineNo, err)
		}
		decls = append(decls, decl)
	}
	return decls, nil
}

func parseDeclarationLine(line []byte) (Declaration, error) {
	dec := json.NewDecoder(bytes.NewReader(line))
	var fields map[string]json.RawMessage
	if err := dec.Decode(&fields); err != nil {
		return Declaration{}, err
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return Declaration{}, ErrTrailingData
		}
		return Declaration{}, fmt.Errorf("%w: %v", ErrTrailingData, err)
	}
	if err := rejectUnknown(fields, "id", "path", "language", "kind", "names", "range", "doc"); err != nil {
		return Declaration{}, err
	}
	for _, key := range []string{"id", "path", "language", "kind", "names", "range", "doc"} {
		if _, ok := fields[key]; !ok {
			return Declaration{}, fmt.Errorf("%w: %s", errMissingField, key)
		}
	}
	idText, err := decodeString(fields["id"])
	if err != nil {
		return Declaration{}, fmt.Errorf("id: %w", err)
	}
	id, err := ParseDeclID(idText)
	if err != nil {
		return Declaration{}, err
	}
	pathText, err := decodeString(fields["path"])
	if err != nil {
		return Declaration{}, fmt.Errorf("path: %w", err)
	}
	language, err := decodeString(fields["language"])
	if err != nil {
		return Declaration{}, fmt.Errorf("language: %w", err)
	}
	kind, err := decodeString(fields["kind"])
	if err != nil {
		return Declaration{}, fmt.Errorf("kind: %w", err)
	}
	names, err := decodeNames(fields["names"])
	if err != nil {
		return Declaration{}, err
	}
	rng, err := decodeRange(fields["range"])
	if err != nil {
		return Declaration{}, fmt.Errorf("range: %w", err)
	}
	doc, err := decodeDocID(fields["doc"])
	if err != nil {
		return Declaration{}, fmt.Errorf("doc: %w", err)
	}
	return Declaration{
		ID:       id,
		Path:     pathText,
		Language: syntax.Language(language),
		Kind:     syntax.Kind(kind),
		Names:    names,
		Range:    rng,
		Doc:      doc,
	}, nil
}

func decodeNames(raw json.RawMessage) ([]string, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, errors.New("names is null")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, errors.New("names must be an array")
	}
	names := make([]string, len(items))
	for i, item := range items {
		text, err := decodeString(item)
		if err != nil {
			return nil, fmt.Errorf("names[%d]: %w", i, err)
		}
		if text == "" {
			return nil, fmt.Errorf("names[%d] is empty", i)
		}
		names[i] = text
	}
	return names, nil
}

func decodeDocID(raw json.RawMessage) (ID, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ID{}, nil
	}
	text, err := decodeString(raw)
	if err != nil {
		return ID{}, errors.New("must be a comment ID or null")
	}
	id, err := ParseID(text)
	if err != nil {
		return ID{}, err
	}
	return id, nil
}

func validateDeclarations(decls []Declaration, comments []Entry) error {
	byComment := make(map[uint64]Entry, len(comments))
	for _, comment := range comments {
		byComment[comment.ID.n] = comment
	}
	seen := make(map[declKey]int, len(decls))
	for i, decl := range decls {
		want := uint64(i) + 1
		if !decl.ID.Valid() || decl.ID.n != want {
			return fmt.Errorf("declaration [%d]: %w", i, ErrDeclarationIDSequence)
		}
		if err := logicalPathError(decl.Path); err != nil {
			return fmt.Errorf("declaration [%d] path %q: %w", i, decl.Path, err)
		}
		if !utf8.ValidString(decl.Path) || !utf8.ValidString(string(decl.Language)) || !utf8.ValidString(string(decl.Kind)) {
			return fmt.Errorf("declaration [%d]: text is not valid UTF-8", i)
		}
		if decl.Language == "" {
			return fmt.Errorf("declaration [%d]: %w", i, ErrEmptyLanguage)
		}
		if decl.Kind == "" {
			return fmt.Errorf("declaration [%d]: %w", i, ErrEmptyKind)
		}
		if err := validateRange(decl.Range); err != nil {
			return fmt.Errorf("declaration [%d]: %w", i, err)
		}
		for n, name := range decl.Names {
			if name == "" || !utf8.ValidString(name) {
				return fmt.Errorf("declaration [%d]: name [%d] is invalid", i, n)
			}
		}
		if decl.Doc.Valid() {
			comment, ok := byComment[decl.Doc.n]
			if !ok || comment.Path != decl.Path {
				return fmt.Errorf("declaration [%d]: %w", i, ErrDanglingDoc)
			}
		}
		key := declKey{rng: decl.Range, kind: decl.Kind}
		if prev, ok := seen[key]; ok && decls[prev].Path == decl.Path {
			return &DuplicateDeclarationError{Path: decl.Path, Index: prev, Other: i, Range: decl.Range}
		}
		seen[key] = i
		if i > 0 && compareStoredDecl(decls[i-1], decl) >= 0 {
			return fmt.Errorf("declaration [%d]: %w", i, ErrDeclarationsOutOfOrder)
		}
	}
	return nil
}

func compareStoredDecl(a, b Declaration) int {
	return compareDecl(
		collectedDecl{path: a.Path, kind: a.Kind, rng: a.Range},
		collectedDecl{path: b.Path, kind: b.Kind, rng: b.Range},
	)
}
