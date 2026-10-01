# AGENTS.md

These rules are mandatory for every human or agent modifying Nodex. Prefer executable enforcement when a rule can be verified objectively. When a requested change conflicts with this contract, redesign the implementation rather than violating it.

## Architecture and dependencies

Every first-level package under `internal/` is a bounded root representing an architectural capability or territory.

Cross-root dependencies are always root-to-root: `internal/foo` may import `internal/bar`, but never `internal/bar/contracts`, `internal/bar/types`, `internal/bar/schema`, `internal/bar/parser`, or any other subpackage of `bar`. Subpackages are private implementation details. Names such as `contracts`, `schema`, `types`, `driver`, `postgres`, and `oauth` receive no exemption. Packages within one root may depend on that root's subpackages, forming an internal DAG.

A root package is a semantic facade, not a barrel package. Expose only stable concepts and capabilities that genuinely belong to its API. Do not bypass boundaries through aliases, bulk re-exports, generic containers, or dependency indirection.

Dependencies between bounded roots must form a DAG. Solve reverse requirements through consumer-owned ports and composition. The consuming root defines the interface when it needs a capability without depending on another territory's implementation; composition supplies the implementation. Do not move interfaces to providers merely to invert dependencies.

`cmd/...` constructs implementations and adapters and connects bounded roots. Composition must not contain product or domain logic.

Neutral foundations are explicit exceptions with a clear, reusable, self-contained responsibility. They may own neutral primitives and mechanisms, but never product rules. Do not create generic `utils`, `helpers`, `common`, or `shared` escape packages. Code and types belonging to a territory remain there; do not create generic `model`, `types`, or `schema` roots merely because several packages need data structures.

Prefer simple structures over speculative abstractions. Do not create packages, interfaces, foundations, or extension mechanisms for hypothetical future needs. A subpackage must represent a coherent responsibility, not an individual file or tiny concept; a couple of helpers generally belong in the existing package.

## Package, documentation, and test conventions

Under `internal/`, allow only `internal/<root>` and `internal/<root>/<subpackage>`; deeper package trees are forbidden.

Packages with multiple files keep a conceptual root file matching the package name as their main reading entry point, such as `internal/index/index.go`. General package documentation belongs there. Every exported symbol carries GoDoc next to its declaration; do not centralize documentation for symbols declared elsewhere.

Internal comments should add information not evident from the code. GoDoc and code comments must describe the current contract, invariants, limits, and behavior without requiring knowledge of prompts, reports, development phases, ADRs, roadmaps, migrations, tickets, or implementation history. Do not narrate history. References to external technical standards may add precision, but explain the relevant semantics locally.

Prefer one canonical `<package>_test.go` per package. Split tests only for a real reason, avoiding unnecessary fragmentation.

`tests/architecture` must inspect packages and imports and reject objective violations, including cross-root subpackage imports, forbidden generic packages, excessive depth, invalid root dependencies, and root cycles. Important checks must include controlled artificial violations proving their failure paths. Enforce objective rules mechanically; do not replace semantic rules with weak string-matching heuristics. Semantic quality remains a review concern when no reliable mechanical check exists. Validate changes with `go test ./...`, `go vet ./...`, and `go test ./tests/architecture`.

## Repository conventions are not product assumptions

Development directories, paths, naming conventions, tools, workflows, architecture rules, and other repository-specific artifacts do not automatically become special behavior in repositories analyzed by Nodex. Including, excluding, or otherwise treating a path specially requires an independent product decision. New development conventions must not silently acquire product meaning.

## Product boundaries and ownership

Nodex is a multilingual source-code indexing tool; Go is the first supported language. Nodex collects, indexes, localizes, manages snapshots, and retrieves source comments. It must not judge comment meaning or quality, rewrite comments, call an LLM, classify comments as good or bad, or become an intelligent linter. Semantic analysis belongs outside Nodex.

Five bounded roots own the work:

- `internal/project` resolves a project root, discovers regular files inside that filesystem boundary, and reads the exact bytes of a canonical logical path within it.
- `internal/ignore` compiles and matches manual exclusions and stores enabled concrete preset identifiers and ordered manual patterns. It takes configuration bytes from the caller and does not open the document, discover files, or interpret preset semantics.
- `internal/syntax` interprets caller-supplied source into language-neutral documents and comments and owns supported languages and language-aware presets. It returns bounded structural context around a comment range as a physical source slice carrying no syntax tree. It does not walk the filesystem or read ignore configuration.
- `internal/index` orders comments, assigns snapshot-local IDs, persists the index, and owns currentness comparison. It does not discover files, match exclusions, or parse source.
- `internal/skill` embeds one canonical provider-neutral Agent Skill and installs or removes that same document at a caller-supplied base. Targets select project-relative or home-relative destinations without changing the text. The package does not resolve project roots or homes, read source, parse comments, match exclusions, or persist Nodex state.

`cmd/nodex` composes these roots. Nodex distributes the embedded skill to describe the public command workflow to an external agent; it does not execute the skill.

## Language architecture

Each language implementation is a private subpackage of `internal/syntax`, currently `internal/syntax/golang` for Go. Adding a language adds a subpackage there, never a first-level `internal/` package. Consumers outside `syntax` import only `internal/syntax`, never its language implementation or other subpackages. The public syntax API is language-neutral; Go syntax-tree types stay inside `internal/syntax/golang`.

Language recognition uses case-sensitive path suffixes without inspecting contents; Go currently recognizes `.go`. Parsing returns every language-defined comment unit in physical order, with its exact raw bytes and normalized text that preserves directives. Positions use zero-based byte offsets, one-based lines and byte columns, and half-open ranges; source line directives must not relocate them. Unsupported paths or malformed source return an error without a partial document. Go parsing does not type-check, resolve imports, or select files by build constraints.

Context identifies a comment by its exact physical range, never its text. Bounds belong to `syntax`; snippets are contiguous original source bytes without reformatting or inserted ellipses.

The language-neutral preset catalog is `internal/syntax/presets.go`; each language owns its semantics in its implementation package's `presets.go`. Syntax decides which presets this binary supports, expands aggregate selectors, and determines whether an enabled preset excludes a supported source. Current Go presets are `go:tests`, `go:vendor`, and `go:generated`, defined in `internal/syntax/golang/presets.go`. `go:all` expands to those concrete identifiers and is not a preset. `go:tests` matches Go basenames ending in `_test.go`; `go:vendor` matches Go files beneath a `vendor` path element; `go:generated` uses Go's structural generated-file convention, not a loose text search. Platform-specific names and build-tagged files remain ordinary source unless a manual exclusion or enabled preset removes them.

## Project resolution and control directory

Automatic root resolution starts at the process working directory and walks ancestors to the nearest language-neutral marker. A real `.nodex` directory takes precedence over `.git` in the same directory. `.git` may be a real directory or regular file. Symbolic links named `.nodex` or `.git` are errors and are not followed; any other invalid marker type is also an error. Resolution stops at the marker, does not use language manifests such as `go.mod` or `go.work`, and does not split the tree into several roots. Without a marker, the starting directory is the root.

`--root <path>`, before the command, is the only global flag. It selects that directory through `project.Open` without walking ancestors. `Open` establishes an absolute, cleaned boundary and resolves links in the supplied root path to the actual directory.

Discovery returns regular files in lexical order with canonical, slash-separated paths relative to that boundary. It skips symbolic links and other non-regular entries and never traverses directories named `.git` or `.nodex` at any depth; exclusions cannot re-include them. It applies the caller-supplied `project.ExclusionPolicy` port without interpreting policy semantics or reading `.gitignore`. `ReadFile` rejects absolute, escaping, or unclean logical paths, control-directory components, directories, and symbolic links, including intermediate links; it does not repair invalid paths or apply exclusions.

`.nodex/` is Nodex's only project-local control directory: persistent project configuration and generated state live beneath it. Discovery always excludes it as a product decision independent of ignore patterns. Do not require other root-level control files such as `.nodexignore`, `.nodexrc`, or `nodex.yaml`.

External agents' skill directories are not Nodex control directories. Project-local skill installation uses ordinary root resolution and writes only the agent's destination, without installed-target state under `.nodex/`. Skill commands do not create or modify `.nodex/`. Provider targets differ only by installation path.

Skill operations require an existing base directory and reject symbolic links beneath it. Installation writes the embedded bytes, leaves an identical skill in place, and replaces only a file carrying the Nodex ownership marker. Uninstallation removes only a managed skill and its empty final `nodex` directory; other files and parent directories remain. Unmanaged destinations must never be overwritten or deleted. The installed document comes from the embedded asset; runtime downloads or checkout files must not supply its content.

## Ignore policy

Project-specific exclusions belong to persistent `.nodex/ignore.json`. Schema 1 requires integer `schema: 1`, a `presets` array of concrete preset identifiers, and an `exclude` array of manual patterns. Manual patterns preserve order and duplicates. Preset identifiers form a set: reject duplicates and canonicalize in lexical order. No preset is enabled by default. A missing document means no presets or manual exclusions; reading it must not create it. Manual patterns are project decisions, not built-in repository conventions. Reject unknown fields, invalid UTF-8, null arrays, trailing data, or invalid entries as document errors; never use a partially compiled policy.

Manual matching is exact and ordered: the last matching rule decides, and `!` negates a rule. Patterns use slash-separated elements; a trailing unescaped `/` is directory-only, a leading or internal `/` anchors a pattern at the project root, and a bare name matches at any depth. `**` is recursive only as a whole element. An excluded ancestor seals its subtree, so negation can re-include a file only when its ancestors remain traversable. Do not clean candidate paths, fold case, normalize Unicode, or treat dotfiles specially.

`internal/ignore` stores the policy; `internal/syntax` owns language-aware preset meanings. Aggregate selectors such as `go:all` are command-line expansion tokens for the concrete presets known by the running binary and must never be persisted. Reject documents containing aggregate tokens or unknown identifiers before generation or currentness proceeds. Manual path exclusions apply during discovery.

Ignore reads and writes reject symbolic links at `.nodex` or `ignore.json`. Edits preserve manual patterns and publish a validated document through a temporary file and rename. An edit that leaves policy identity unchanged does not rewrite or create the document.

## Generated index and currentness

`.nodex/index/` is disposable derived state. Generation may create it and replace `snapshot.json` and `comments.jsonl`; `snapshot.json` is the commit marker for the pair. Publish through temporary files in the index directory, replacing comments first and the snapshot last. The snapshot records the exact comments-file digest and count; loading rejects inconsistent or malformed state, including mixed generations. Absence of the snapshot means no committed index. State directories must be real directories, and loading rejects symbolic links in committed paths.

Comment ordering is deterministic by logical path and physical position, never by text or caller input order. IDs start at `C000001` and are local to that snapshot, not permanent references across regeneration. Stored entries contain normalized text and physical locations, not raw source, syntax trees, or structural context. Do not store absolute paths, timestamps, or machine identity.

A snapshot is current only when its recorded ignore-policy identity and ordered source fingerprints match the effective policy and included supported sources. Policy identity covers the canonical preset set and compiled manual rules. Each fingerprint contains the logical path, recognized language, and SHA-256 digest of the exact source bytes. Comment meaning and file modification times are not part of this comparison. Policy identity hashes the compiled policy, not JSON bytes; formatting, object-field order, and preset ordering do not make a snapshot stale.

Sources excluded by enabled presets are absent from the fingerprint set; changing them does not make a snapshot stale. Changing the enabled preset set changes policy identity and makes an existing snapshot stale. Changes within `.nodex/index/` are never source inputs because discovery excludes `.nodex/`.

Currentness belongs to `internal/index` and must not parse source to rediscover comments. Source selection may inspect structure when a structural preset is enabled; this is not comment indexing. Without structural presets, currentness compares only policy identity and byte fingerprints.

## Command behavior

- `generate` persists the comment index.
- `comments` emits the current index as compact Markdown containing only comment IDs and normalized text.
- `status` reports missing, current, stale, or corrupt without modifying the project. These states are successful inspections; operational failures to inspect the project are errors.
- `show` resolves one or more snapshot-local IDs from a current index to logical files, physical comment lines, stored normalized text, and bounded structural source context. Source bytes are read to check currentness, then reread for context only after the snapshot is current. The context read rejects bytes that no longer match the fingerprint. Comment parsing for retrieval occurs only while building context for that current snapshot. Requested ID order and duplicates are preserved; a malformed or unknown ID fails without partial successful output.
- `comments`, `status`, and `show` never regenerate missing or stale indexes. `comments` and `show` fail for missing, corrupt, or stale indexes. Changing ignore configuration does not regenerate an index either.
- `ignore list`, `ignore enable`, and `ignore disable` use the resolved or explicit root. `ignore presets` lists selectors without opening a project.
- `skill targets` and `skill show` list targets and emit the canonical skill, respectively, without opening a project. Project-local `skill install` and `skill uninstall` use ordinary root resolution, including explicit `--root`. Their `--global` option uses the current user's home without resolving a project and cannot be combined with `--root`.
- `version` prints the source-defined product version and linked commit and build date without opening a project. Product version is the constant `0.1.0-beta.1`; `commit` and `buildDate` default to `unknown`, and release builds may override only those two metadata values. Runtime code must not inspect version control or infer build identity from the working directory.
