---
name: nodex
description: Use Nodex when analyzing, reviewing, locating, or reasoning about source-code comments in a repository. Nodex provides a deterministic comment index, compact comment IDs, and bounded source context.
---

<!-- nodex-managed-skill:v1 -->

# Nodex

Nodex collects, indexes, localizes, and retrieves. The LLM analyzes.

The workflow applies to source comments. Nodex does not decide whether a comment is correct, incorrect, stale, useful, obsolete, good, or bad. It is not a linter.

Use this skill when the user asks to analyze code comments, review comments, inspect documentation comments, locate comments, investigate comments in source, reason about potentially stale or misleading comments, compare comments with the surrounding implementation, or perform repository-wide comment analysis. Do not use it merely because a programming task contains source code. Use it when source comments matter to the task.

## Start with status

The primary workflow begins with:

```bash
nodex status
```

`nodex status` reports whether the generated comment index is missing, current, stale, or corrupt. Do not rely on comment IDs until that index is current.

If the generated index is missing or stale, run:

```bash
nodex generate
```

before relying on comment IDs. Run `nodex status` again after generation and use comment IDs only when the index is current.

## Compact corpus

`nodex comments` is the compact corpus operation. Each entry contains only:

- the comment ID
- the normalized comment text

Do not infer a source location from `nodex comments`. The corpus has no file path and no source position.

## Source location and context

For source location or source context, use:

```bash
nodex show <ID...>
```

`nodex show` provides bounded structural source context for each ID, including the logical file, the physical comment lines, and the stored normalized text. When several IDs are relevant, batch them into one `nodex show` invocation.

Comment IDs are snapshot-local. IDs may change after regeneration. After source changes, run `nodex status` again before relying on previously observed IDs. If regeneration occurs, previously observed IDs must not be assumed to refer to the same comments.

After `nodex show`, ordinary source-reading tools may be used when the broader task needs more code than the bounded context. Nodex is not a prohibition on normal code inspection. It provides the authoritative syntactic comment corpus and the localization workflow for comments.

Do not read or reconstruct `.nodex/index/` directly. Do not parse `snapshot.json` or `comments.jsonl`. Use the public CLI commands.

When Nodex supports the relevant source, do not grep or regex the repository for comments as a substitute for Nodex.

## Ignore configuration

`nodex ignore list` shows the project's source-exclusion configuration. Inspect it when the included source set matters.

Do not run `nodex ignore enable` or `nodex ignore disable` without explicit user intent. Changing ignore configuration changes the source set. If ignore configuration changes, treat the existing generated index as requiring a new status and generation cycle. Run `nodex status` again, and run `nodex generate` when the index is missing or stale, before relying on comment IDs.
