# Nodex

Nodex helps you review the comments in a codebase, and see which declarations have a documentation comment attached directly to them, without having to scan the whole repository yourself.

It finds the comments that actually belong to the source code, gives each one a simple ID, and lets you jump back to the relevant code when you need more context. It can also list declarations and say whether each one has a directly attached documentation comment.

```text
Nodex finds, organizes, and locates comments and declarations.
You or the LLM decide what they mean.
```

Nodex does not try to interpret comments for you. It does not decide whether a comment is correct, outdated, useful, misleading, or worth changing. It also does not decide that a declaration needs a documentation comment.

Nodex can show which declarations have a directly attached documentation comment and which do not. It reports that structure; you or the LLM decide whether anything needs to change.

Its job is to find the comments and declarations and show you where they are. The analysis is left to you or the LLM.

This beta supports Go (`.go`), JavaScript (`.js`), JSX (`.jsx`), TypeScript (`.ts`), and TSX (`.tsx`). Recognition is case-sensitive. JavaScript and JSX share a grammar; TypeScript and TSX use distinct grammars. Other extensions are not recognized.

For JavaScript-family source, Nodex records individual physical comments, declarations, raw structural names, exact byte ranges, and original-byte context. Declaration families include functions and generators, classes and members, constructors, getters/setters, variables and destructuring, interfaces and signatures, type aliases, enums and members, namespaces/modules, ambient declarations, object members, and anonymous default exports. Kinds describe source structure; they do not classify runtime values.

Direct JSDoc relationships are ordered 0..N: several leading blocks remain separate comments with separate IDs. Attachment uses a declaration's structural leading-trivia slot before its export, ambient, decorator and modifier tokens. It does not cross intervening syntax. A block after a decorator remains a physical comment without documenting that declaration. Overload signatures and nested declarations keep independent relationships. Names keep exact source spelling, including escapes and literal property quotes; computed names and anonymous declarations may be empty. Constructors have no invented binding name.

Nodex does not validate JavaScript/TypeScript syntax, type-check, resolve symbols or modules, or interpret JSDoc tags. If the parser exposes unsafe recovery, Nodex keeps confidently observed comments and omits every declaration in that file. Some comments can remain unobserved on arbitrarily broken input. This conservative policy can also omit intact neighboring declarations; it never guesses replacement relationships.

## Quick start

Build Nodex from source:

```bash
go build -o nodex ./cmd/nodex
```

Then, from a project:

```bash
nodex index generate
nodex index comments
nodex index declarations
nodex index show C000001
```

`index generate` scans the project and builds the index.

`index comments` gives you the list of indexed comments with their IDs, logical source-relative file paths, and normalized text.

`index declarations` gives you each declaration's ID, logical source-relative file path, kind, names, and the ordered IDs of its directly attached documentation comments, or `docs: none` when there are none.

`index show` takes a comment ID or a declaration ID and shows you where it came from, together with the surrounding code.

A normal workflow looks like this:

```bash
nodex index status
nodex index generate
nodex index comments
nodex index declarations
nodex index show C000001
```

Use `index status` whenever the project may have changed. If the index is missing or out of date, run `index generate` again before using existing IDs.

## Why Nodex exists

Comments are useful context for understanding a codebase, but finding and reviewing all of them can be noisy and repetitive. Seeing which declarations have a documentation comment attached to them usually means walking the syntax tree again.

Nodex gives you a small, predictable way to work with both.

Instead of asking an LLM to search through the whole repository for comments, you can first generate an index:

```bash
nodex index generate
```

Then get the comments:

```bash
nodex index comments
```

The declaration facts:

```bash
nodex index declarations
```

And when one of them needs a closer look:

```bash
nodex index show C000001
nodex index show D000001
```

`index comments` provides comment IDs, logical file paths, and compact normalized text. `index declarations` provides declaration IDs, logical file paths, and compact facts with direct documentation links. `index show` is the source location and a bounded piece of the original source. The show result for a comment does not print the normalized comment a second time, and the show result for a declaration does not print the documentation comment as a separate copy.

This keeps the first pass simple and lets you fetch source context only when it is actually useful.

## Comment and declaration IDs

Each indexed comment gets an ID such as:

```text
C000001
C000002
C000003
```

Each indexed declaration gets an ID such as:

```text
D000001
D000002
D000003
```

The IDs are predictable inside one generated index, but they are not permanent.

If the source changes and you run `nodex index generate` again, a comment or declaration may receive a different ID.

Think of the IDs as short references for the current snapshot of the project.

`docs: none` on a declaration means only that no documentation comment is attached directly to that declaration. It does not mean the declaration is wrong or that a comment should be added.

## Commands

The current command surface is:

```text
nodex index generate
nodex index comments [--file <path> [<path>...]]
nodex index declarations [--file <path> [<path>...]] [--kind <kind> [<kind>...]] [--name <name> [<name>...]]
nodex index show <ID...>
nodex index status
nodex version
nodex ignore ...
nodex skill ...
```

### `index generate`

Scans the source project and writes the index under the selected workspace base:

```text
.nodex/index/
```

### `index comments`

Prints the current comments as compact Markdown.

Each entry contains only a comment ID, its logical source-root-relative file path, and its normalized text.

```md
## C000001

file: `internal/example/example.go`

Example adds nothing.
```

### `index declarations`

Prints indexed declarations as compact Markdown. Without filters, every declaration is included.

Each entry contains only a declaration ID, its logical source-root-relative file path, its kind, its names, and ordered comment IDs separated by a comma and a space, or `docs: none`. Documentation IDs follow physical source order and preserve separate comment identities. Several names are written as `names: A, B`. A declaration with no name uses `names:` with nothing after it.

```md
## D000001

file: `internal/example/example.go`
kind: function
names: Example
docs: C000001
```

### Discovery filters

Reduce discovery before retrieving source context:

```bash
nodex index comments \
  --file internal/history internal/governance

nodex index declarations \
  --file internal/history \
  --kind type method \
  --name Service ProjectAuthority
```

Each filter may appear once and consumes one or more consecutive values until the next filter or the end of the command. Filters may appear in any order. Unsupported flags and missing values are usage errors. Duplicate values are harmless and deduplicated internally; use `--kind type method` rather than repeating `--kind`.

Values within one filter use **OR**. Different filter categories use **AND**. Results keep the canonical index order and their existing IDs and Markdown blocks. No matches succeed with zero output bytes.

`--file` selects an indexed logical source-relative file or subtree: a selector `s` matches a path `p` only when `p == s` or `p` begins with `s + "/"`. For example, `internal/history` includes its descendants but excludes `internal/history2`. Selectors must be canonical, non-empty relative slash-separated paths, without `.` or `..` elements. They are not resolved through the filesystem and need not exist. Globs, regexes, and substring matching have no special meaning.

`--kind` compares the stored declaration kind exactly and case-sensitively, without aliases. Unknown kinds simply match nothing. `--name` compares each indexed name exactly and case-sensitively; any matching name selects the declaration. Declarations with no names do not match a name filter.

`--root` selects the source project boundary, `--out-dir` selects the Nodex workspace base, and `--file` selects facts by logical source paths. File selectors have the same meaning in detached workspaces. Filtering uses the current index and does not regenerate or modify state; missing, stale, and corrupt indexes still fail.

### `index show`

Shows one or more comments or declarations together with their source location and nearby code.

```bash
nodex index show C000001 D000004
```

### `index status`

Shows whether the current index is:

```text
missing
current
stale
corrupt
```

`index status` only checks the project. It does not change anything.

### `version`

Shows the Nodex version and information about the current build.

## Source root and workspace

In the default project-local workflow, Nodex reads source and stores state in the same project:

```bash
cd /srv/agentary
nodex index generate
```

The source root is `/srv/agentary`, the workspace base is `/srv/agentary`, and the control directory is `/srv/agentary/.nodex/`. From a project subdirectory, both locations use the discovered ancestor root.

Nodex normally starts from the invocation working directory and walks upward to the nearest `.nodex/` or `.git` marker. A real `.nodex` directory takes precedence in the same directory; `.git` may be a directory or regular file. Symbolic-link markers and invalid marker types are errors. Without a marker, Nodex uses the invocation directory. It does not use `go.mod` or `go.work` to find the root.

To inspect a source project without writing Nodex state into it, use a separate existing workspace:

```bash
mkdir -p /tmp/agentary
cd /tmp/agentary
nodex --root /srv/agentary index generate
nodex --root /srv/agentary index status
```

`--root` selects only the source project and skips ancestor discovery. **An explicit `--root` changes the default workspace base to the invocation working directory.** Here all state belongs to `/tmp/agentary/.nodex/`, including `ignore.json` and `index/`. Nodex does not create or modify `/srv/agentary/.nodex/`, and any ignore document there does not override the workspace policy.

Use `--out-dir` to select a workspace base explicitly:

```bash
mkdir -p /tmp/nodex-agentary
nodex \
  --root /srv/agentary \
  --out-dir /tmp/nodex-agentary \
  index generate
```

This stores state under `/tmp/nodex-agentary/.nodex/`. `--out-dir` must identify an existing directory; it selects the base containing `.nodex`, not `.nodex` itself or an index file. It does not affect source discovery. Without `--root`, source discovery still starts from the invocation directory.

To retain project-local state with an explicit source root:

```bash
nodex \
  --root /srv/agentary \
  --out-dir /srv/agentary \
  index generate
```

The global syntax is:

```text
nodex [--root <path>] [--out-dir <path>] <command> ...
```

Both options must appear before the top-level command, in either order. Relative paths are interpreted from the invocation working directory. The `--root=<path>` and `--out-dir=<path>` forms are also accepted; duplicate options and missing values are usage errors.

| Options | Source root | Workspace base |
| --- | --- | --- |
| Neither | Discovered from invocation cwd | Resolved source root |
| `--root` only | Explicit root | Invocation cwd |
| `--out-dir` only | Discovered from invocation cwd | Explicit out-dir |
| Both | Explicit root | Explicit out-dir |

Every mode reads/writes ignore configuration at `<workspace>/.nodex/ignore.json` and index state at `<workspace>/.nodex/index/`. Read commands never create state or regenerate an index. Source discovery always excludes `.git` and `.nodex` directories at any depth, including a workspace's control directory inside the source tree. Ordinary files elsewhere in that workspace remain ordinary source candidates.

Snapshot currentness compares the effective policy identity and source-relative byte fingerprints. Absolute source and workspace paths are not stored; an identical source tree can use the same snapshot.

These commands do not open a source project or workspace, and accept well-formed global options without inspecting their paths:

```text
nodex version
nodex ignore presets
nodex skill targets
nodex skill show
```

`--out-dir` has no effect on any skill command. Project-local skill installation still uses the resolved or explicit source root, and `--global` still uses the user's home.

## Ignoring files

Ignore settings for the selected workspace live in:

```text
.nodex/ignore.json
```

If that file does not exist, no optional presets or manual path exclusions are enabled.

`.git` and `.nodex` are always ignored by Nodex and do not need to appear in the file.

Available commands:

```bash
nodex ignore presets
nodex ignore list
nodex ignore enable go:tests
nodex ignore disable go:tests
```

No preset is enabled by default.

The current Go presets are:

```text
go:generated
go:tests
go:vendor
```

`go:generated` ignores Go files recognized as generated by Go.

`go:tests` ignores files ending in `_test.go`.

`go:vendor` ignores Go files inside a `vendor` directory.

You can also use:

```text
go:all
```

`go:all` is a command-line shortcut that enables all currently available Go presets.

The shortcut itself is not stored in `.nodex/ignore.json`; the concrete preset IDs are.

Manual path patterns from `.nodex/ignore.json` are applied together with enabled presets.

Changing ignore settings does not automatically rebuild the index. Use `nodex index status` to check whether a new `index generate` is needed.

## Agent skill

Nodex includes a provider-neutral Agent Skill that teaches coding agents how to use the Nodex workflow.

The basic flow is:

```text
index status
index generate when needed
filtered index comments or index declarations
index show
analysis by the human or LLM
```

Nodex does not run the skill itself. It copies the embedded skill file to the location expected by the selected agent.

Available commands:

```bash
nodex skill targets
nodex skill show
nodex skill install codex
nodex skill uninstall codex
nodex skill install codex --global
```

Supported targets:

```text
agents
claude
codex
gemini
grok
```

By default, a skill is installed for the current project:

```bash
nodex skill install codex
```

Use `--global` to install it for the current user instead:

```bash
nodex skill install codex --global
```

Global skill operations do not need a project and cannot be combined with `--root`.

## Version

```bash
nodex version
```

A normal build prints:

```text
nodex <version>
commit <commit>
built <build-date>
```

The product version is defined directly in the source at `cmd/nodex/version.go`.

Changing the Nodex version means changing that value in the code.

`commit` and `buildDate` describe the particular binary that was built.

Local builds leave both as `unknown`.

A release build can provide them when linking:

```bash
go build \
  -ldflags "-X main.commit=<commit> -X main.buildDate=<buildDate>" \
  -o nodex \
  ./cmd/nodex
```

The product version itself is not changed through linker flags.

The running binary does not inspect Git or another version-control system to discover this information.

## Requirements

Nodex currently requires:

```text
Go 1.27.1
```

Module:

```text
github.com/edalca/nodex
```

Nodex uses the Go standard library for Go syntax and a pinned pure-Go gotreesitter runtime with embedded grammars for JavaScript-family syntax. Building and running Nodex requires no C toolchain, Node, or TypeScript compiler.
