# Nodex

**0.1.0-beta.1**

Nodex helps you review the comments in a codebase, and see which declarations have a documentation comment attached directly to them, without having to scan the whole repository yourself.

It finds the comments that actually belong to the source code, gives each one a simple ID, and lets you jump back to the relevant code when you need more context. It can also list declarations and say whether each one has a directly attached documentation comment.

```text
Nodex finds, organizes, and locates comments and declarations.
You or the LLM decide what they mean.
```

Nodex does not try to interpret comments for you. It does not decide whether a comment is correct, outdated, useful, misleading, or worth changing. It also does not decide that a declaration needs a documentation comment.

Nodex can show which declarations have a directly attached documentation comment and which do not. It reports that structure; you or the LLM decide whether anything needs to change.

Its job is to find the comments and declarations and show you where they are. The analysis is left to you or the LLM.

This beta currently includes support for Go source files.

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

`index comments` gives you the list of indexed comments with their IDs and normalized text.

`index declarations` gives you each declaration's kind, names, and the ID of its directly attached documentation comment, or `doc: none` when there is none.

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

`index comments` is the compact normalized comment text. `index declarations` is the compact list of declarations and their direct documentation links. `index show` is the source location and a bounded piece of the original source. The show result for a comment does not print the normalized comment a second time, and the show result for a declaration does not print the documentation comment as a separate copy.

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

`doc: none` on a declaration means only that no documentation comment is attached directly to that declaration. It does not mean the declaration is wrong or that a comment should be added.

## Commands

The current command surface is:

```text
nodex index generate
nodex index comments
nodex index declarations
nodex index show <ID...>
nodex index status
nodex version
nodex ignore ...
nodex skill ...
```

### `index generate`

Scans the project and writes the index under:

```text
.nodex/index/
```

### `index comments`

Prints the current comments as compact Markdown.

Each entry contains a comment ID and its normalized text.

### `index declarations`

Prints every indexed declaration as compact Markdown.

Each entry contains a declaration ID, its kind, its names, and either a comment ID or `doc: none`. Several names are written as `names: A, B`. A declaration with no name uses `names:` with nothing after it.

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

## Project root

Nodex normally starts from your current directory and walks upward until it finds the nearest project marker.

It recognizes:

```text
.nodex/
.git
```

If both exist in the same directory, `.nodex` takes precedence.

If Nodex does not find either marker, it uses the directory where the search started.

It does not use files such as `go.mod` or `go.work` to decide where the project begins.

You can choose the project root yourself:

```bash
nodex --root /path/to/project index status
```

`--root` must appear before the command.

These commands do not need a project:

```text
nodex version
nodex ignore presets
nodex skill targets
nodex skill show
```

## Ignoring files

Project-specific ignore settings live in:

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
index comments
index declarations
index show
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
nodex 0.1.0-beta.1
commit unknown
built unknown
```

The product version is defined directly in the source:

```go
const version = "0.1.0-beta.1"
```

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

Nodex uses only the Go standard library.
