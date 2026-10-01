---
name: nodex
description: Use Nodex when analyzing, reviewing, locating, or reasoning about source-code comments and declarations in a repository. Nodex provides a deterministic index of comments, declarations, compact IDs, and bounded source context.
---

<!-- nodex-managed-skill:v1 -->

# Nodex

Nodex collects, indexes, localizes, and retrieves. The LLM analyzes.

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

## Compact comment corpus

`nodex index comments` is the compact comment corpus. Each entry contains only:

- the comment ID
- the normalized comment text

Do not infer a source location from `nodex index comments`. The corpus has no file path and no source position.

## Compact declaration facts

`nodex index declarations` lists every indexed declaration. Each entry contains only:

- the declaration ID
- the declaration kind
- the declared names after `names:`, separated by a comma and a space when there are several, or `names:` with no value when the declaration has no name
- `doc:` followed by the comment ID of the directly associated documentation comment, or `doc: none`

`doc: none` means only that the parser recorded no documentation comment directly on that declaration. It does not mean that documentation is required, that the declaration is defective, or that a comment should be added. Do not treat `doc: none` as a defect by itself.

The declaration list includes declarations that have direct documentation and declarations that do not. It is not a filter.

## Source location and context

For source location or source context, use:

```bash
nodex index show <ID...>
```

`nodex index show` accepts comment IDs and declaration IDs. It provides the logical file, the physical lines, and bounded structural source context. A comment result does not repeat the normalized comment text outside that source context. A declaration result identifies the declaration and does not repeat associated comment text outside the source context. Normalized comment text stays in `nodex index comments`. When several IDs are relevant, batch them into one `nodex index show` invocation.

Comment IDs and declaration IDs are snapshot-local. IDs may change after regeneration. After source changes, run `nodex index status` again before relying on previously observed IDs. If regeneration occurs, previously observed IDs must not be assumed to refer to the same comments or declarations.

After `nodex index show`, ordinary source-reading tools may be used when the broader task needs more code than the bounded context. Nodex is not a prohibition on normal code inspection. It provides the authoritative syntactic comment corpus, the declaration facts, and the localization workflow.

Do not read or reconstruct `.nodex/index/` directly. Do not parse `snapshot.json`, `comments.jsonl`, or `declarations.jsonl`. Use the public CLI commands.

When Nodex supports the relevant source, do not grep or regex the repository for comments as a substitute for Nodex.

## Ignore configuration

`nodex ignore list` shows the project's source-exclusion configuration. Inspect it when the included source set matters.

Do not run `nodex ignore enable` or `nodex ignore disable` without explicit user intent. Changing ignore configuration changes the source set. If ignore configuration changes, treat the existing generated index as requiring a new status and generation cycle. Run `nodex index status` again, and run `nodex index generate` when the index is missing or stale, before relying on IDs.
