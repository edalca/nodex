---
name: nodex
description: Use Nodex when analyzing, reviewing, locating, or reasoning about source-code comments and declarations in a repository. Nodex provides a deterministic index of comments, declarations, compact IDs, and bounded source context.
---

<!-- nodex-managed-skill:v1 -->

# Nodex

Nodex collects, indexes, localizes, and retrieves. The LLM analyzes.

Supported extensions are `.go`, `.js`, `.jsx`, `.ts`, and `.tsx`, matched case-sensitively. JavaScript-family comments remain individual physical units. Direct JSDoc relationships are ordered 0..N and use structural leading-trivia slots before declaration modifiers, exports and decorators. Blocks after decorators do not directly document that declaration. Nested declarations and overload signatures have independent relationships; docs never propagate. Names preserve raw source spelling; computed names and anonymous declarations may be empty, and constructors have no invented binding name.

Nodex does not validate JavaScript/TypeScript syntax, type-check, resolve symbols or modules, or interpret JSDoc tags. Detected unsafe recovery retains confidently observed comments and omits all declarations in that file, including intact neighbors. Arbitrarily broken source can also hide comments. Missing declarations or relationships are not quality judgments.

The workflow applies to source comments and to declarations. Nodex does not decide whether a comment is correct, incorrect, stale, useful, obsolete, good, or bad. It does not decide that a declaration requires documentation. It is not a linter.

Use this skill when the user asks to analyze code comments, review comments, inspect documentation comments, locate comments, investigate comments in source, reason about potentially stale or misleading comments, compare comments with the surrounding implementation, inspect which declarations have a directly associated documentation comment, or perform repository-wide comment analysis. Do not use it merely because a programming task contains source code. Use it when source comments or declaration documentation structure matter to the task.

## Start with status

The primary workflow begins with:

```bash
nodex index status
```

`nodex index status` reports whether the generated index is missing, current, stale, or corrupt. Do not rely on comment IDs or declaration IDs until that index is current.

If the generated index is missing or stale, run:

```bash
nodex index generate
```

before relying on IDs. Run `nodex index status` again after generation and use IDs only when the index is current.

## Choose source and workspace

With no global options, Nodex discovers the source root from the invocation directory's ancestors and uses that same root as the workspace base. Its `.nodex/` directory owns both `ignore.json` and `index/`.

To inspect a repository without writing Nodex state into it, select the source with `--root` from a temporary workspace:

```bash
workspace="$(mktemp -d)"
cd "$workspace"

nodex --root /path/to/project index generate
nodex --root /path/to/project index status
nodex --root /path/to/project index comments
nodex --root /path/to/project index declarations
nodex --root /path/to/project index show C000001 D000001
```

Use actual IDs returned by the current index for `show`. Keep the same workspace and source selection for subsequent commands.

Explicit `--root` selects source only, skips ancestor discovery, and defaults the workspace to invocation cwd. This temporary workspace also owns `.nodex/ignore.json`; authorized temporary ignore experimentation there leaves the source project's configuration untouched. The source project's `.nodex/ignore.json` does not override that workspace policy.

`--out-dir <path>` overrides the workspace base with an existing directory. State always lives at `<out-dir>/.nodex/`. For example, `nodex --root /path/to/project --out-dir "$workspace" index status` uses that workspace from any cwd. To use project-local state with explicit `--root`, set `--out-dir /path/to/project` too.

Both global options precede the top-level command, and relative paths are relative to invocation cwd. `--out-dir` does not affect source discovery or Agent Skill installation. State-independent commands accept well-formed global options without opening their paths. No control-directory symlink is needed or supported.

## Compact comment corpus

`nodex index comments` is the compact comment corpus. Each entry contains only:

- the comment ID
- the logical source-root-relative file path after `file:`
- the normalized comment text

Discovery provides an ID, a logical file path, and a compact fact. File paths are relative to the source root, including when using a detached workspace. Use `nodex index show` for physical lines and bounded source context.

## Compact declaration facts

`nodex index declarations` lists every indexed declaration. Each entry contains only:

- the declaration ID
- the logical source-root-relative file path after `file:`
- the declaration kind
- the declared names after `names:`, separated by a comma and a space when there are several, or `names:` with no value when the declaration has no name
- `docs:` followed by the ordered comment IDs of directly associated documentation comments, separated by a comma and a space, or `docs: none`

`docs: none` means only that the parser recorded no documentation comment directly on that declaration. It does not mean that documentation is required, that the declaration is defective, or that a comment should be added. Do not treat `docs: none` as a defect by itself.

The declaration list includes declarations that have direct documentation and declarations that do not.

## Reduce discovery structurally

Prefer this workflow: status → filtered discovery → show → analysis. Generate when status requires it, then use structural selectors to reduce large discovery streams before choosing IDs:

```bash
nodex index comments \
  --file internal/history internal/governance

nodex index declarations \
  --file internal/history \
  --kind type method \
  --name Service ProjectAuthority
```

Each filter may appear once, in any order, and takes one or more consecutive values until the next filter or the end of arguments. Unsupported flags, repeated flags, and missing values are usage errors. Values within one filter use OR; different filter categories use AND. Duplicate values are harmless and deduplicated internally. Results retain canonical fact order, snapshot-local IDs, and complete discovery blocks. No matches succeed with zero output bytes.

`--file` selects logical source-relative files or subtrees: path `p` matches selector `s` only when `p == s` or `p` begins with `s + "/"`. Thus `internal/history` does not match `internal/history2`. Use canonical non-empty relative slash-separated paths without `.` or `..` elements. Selectors need not exist and are not resolved through the filesystem. `--root` selects the source boundary; `--out-dir` selects the workspace base; `--file` selects indexed logical paths independently of both locations, including in detached workspaces.

`--kind` compares the stored kind exactly and case-sensitively, without aliases. Unknown kinds match nothing. `--name` matches any exact case-sensitive indexed name, including either name in a multi-name declaration. Nameless declarations do not match a supplied name filter. These selectors do not provide glob, regex, substring, fuzzy, semantic, or ranked search.

Filtering selects existing indexed facts and does not regenerate or modify state. Missing, stale, and corrupt snapshots remain unusable. Choose actual IDs from the filtered results for `index show`, then analyze the retrieved source context.

## Source location and context

For detailed physical source location and bounded source context, use:

```bash
nodex index show <ID...>
```

`nodex index show` accepts comment IDs and declaration IDs. It provides the logical file, the physical lines, and bounded structural source context. A comment result does not repeat the normalized comment text outside that source context. A declaration result identifies the declaration and does not repeat associated comment text outside the source context. Normalized comment text stays in `nodex index comments`. When several IDs are relevant, batch them into one `nodex index show` invocation.

Comment IDs and declaration IDs are snapshot-local. IDs may change after regeneration. After source changes, run `nodex index status` again before relying on previously observed IDs. If regeneration occurs, previously observed IDs must not be assumed to refer to the same comments or declarations.

After `nodex index show`, ordinary source-reading tools may be used when the broader task needs more code than the bounded context. Nodex is not a prohibition on normal code inspection. It provides the authoritative syntactic comment corpus, the declaration facts, and the localization workflow.

Do not read or reconstruct `.nodex/index/` directly. Do not parse `snapshot.json`, `comments.jsonl`, or `declarations.jsonl`. Use the public CLI commands.

When Nodex supports the relevant source, do not grep or regex the repository for comments as a substitute for Nodex.

## Ignore configuration

`nodex ignore list` shows the selected workspace's source-exclusion configuration. Use the same `--root` and `--out-dir` options as the index commands when operating detached. Inspect it when the included source set matters.

Do not run `nodex ignore enable` or `nodex ignore disable` without explicit user intent. Changing ignore configuration changes the source set. If ignore configuration changes, treat the existing generated index as requiring a new status and generation cycle. Run `nodex index status` again, and run `nodex index generate` when the index is missing or stale, before relying on IDs.
