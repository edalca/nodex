# Project

## Purpose

Establish source and workspace filesystem boundaries.

## Scope

Neutral marker resolution, explicit selection, independent workspace placement, regular-file discovery and exact confined logical reads. Exclusion meanings and language recognition belong elsewhere.

## Group Invariants

Logical paths are source-root-relative. Repository development conventions do not implicitly become exclusions. Control-directory pruning is Project-owned.

All fixtures and observations follow [the manual-validation reference law](README.md). Nodex collects, indexes, localizes and retrieves; human/LLM analyzes.

## Test Index

| ID | Name | Type | Priority | Spec | Status |
| --- | --- | --- | --- | --- | --- |
| PRJ-ROOT-001 | Nearest neutral project marker | BOUNDARY | CRITICAL | 1 | ACTIVE |
| PRJ-ROOT-002 | Explicit source boundary | BOUNDARY | HIGH | 1 | ACTIVE |
| PRJ-ROOT-003 | Independent workspace selection | BOUNDARY | CRITICAL | 1 | ACTIVE |
| PRJ-PATH-001 | Regular-file discovery and logical identity | BOUNDARY | HIGH | 1 | ACTIVE |
| PRJ-PATH-002 | Confined exact-byte source reads | SECURITY | CRITICAL | 1 | ACTIVE |

## Test Specifications

## PRJ-ROOT-001 — Nearest neutral project marker

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
CRITICAL

### Purpose

Resolve one source boundary from invocation cwd.

### Preconditions

Disposable nested trees with .nodex directories, .git directories/files, manifests, and marker-free branches; inspect resolution through the public project facade or source inventory from CLI.

### Actions

From nested directories resolve with no explicit root. Vary nearer markers, a same-directory real .nodex beside invalid .git, a regular .git file, manifests without markers, and no marker. Replace the selected marker with a symlink, regular .nodex file or invalid .git type.

### Expected Invariants

The nearest marker stops the ancestor walk. A real .nodex takes precedence over .git in that directory; .git may be a real directory or regular file. An examined symlink marker or invalid marker type fails. Manifests do not select or split roots. With no marker, the starting directory is the root.

### Allowed Differences

Absolute temporary locations and unrelated ancestors differ; isolate marker-free cases so no ancestor marker interferes.

### Regression Criteria

Resolution crosses the nearest marker, follows an examined marker link, selects a language manifest or splits the tree.

### Dependencies

NONE

## PRJ-ROOT-002 — Explicit source boundary

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Establish the actual directory selected by --root independently of marker discovery.

### Preconditions

Disposable existing source directory, directory link, missing path and regular file; unrelated invocation cwd.

### Actions

Open the explicit source path using project.Open or --root. Compare relative, absolute and directory-link paths, and reject empty/missing/non-directory selections. Place nearer/ancestor markers around explicit selections.

### Expected Invariants

Explicit --root bypasses ancestor discovery. Open establishes an absolute cleaned existing directory boundary with links in the supplied base path resolved. Relative options are interpreted from invocation cwd. Invalid selections fail without creating a root.

### Allowed Differences

The canonical absolute boundary varies by fixture; persisted logical identity is tested separately.

### Regression Criteria

Explicit selection is replaced by a marker root, a missing base is created, or a non-directory is accepted.

### Dependencies

NONE

## PRJ-ROOT-003 — Independent workspace selection

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
CRITICAL

### Purpose

Place control state beneath the selected workspace base.

### Preconditions

Existing source, invocation and output directories; distinct conflicting source-root/workspace ignore documents; authored supported source.

### Actions

Exercise all four combinations of absent/present --root and --out-dir, including relative options and an output-base link. Inspect locations before generation, then generate and inspect control state. Repeat with missing/non-directory output bases.

### Expected Invariants

Neither option: discovered source root is workspace. --root only: invocation cwd is workspace. --out-dir only: discovered source stays independent and explicit out-dir is workspace. Both: both explicit selections apply. Workspace bases are existing canonical directories with links resolved; selection creates nothing. Ignore configuration and index use `<workspace>/.nodex`; a different source-root ignore document has no effect. An inside-source workspace excludes only its .nodex control tree through Project discovery.

### Allowed Differences

Absolute bases may differ; ordinary workspace source files remain candidates according to the owning selection laws.

### Regression Criteria

Source selection depends on an output marker, state/policy falls back to the source root, or selecting a base creates it.

### Dependencies

PRJ-ROOT-001, PRJ-ROOT-002

## PRJ-PATH-001 — Regular-file discovery and logical identity

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Discover canonical source-relative filesystem candidates without repository-convention exclusions.

### Preconditions

Disposable tree with nested regular files, links, a non-regular entry, .git/.nodex directories at several depths, .gitignore, .outputs and agent skill directories; zero exclusion policy.

### Actions

Call project.Files through a disposable facade harness. Compare returned paths to the authored inventory. Supply an include-all policy and repeat; inspect source inventory through generation where useful.

### Expected Invariants

Only regular files within the boundary are returned, recursively in lexical order as canonical slash-separated relative paths. Links and other non-regular entries are skipped. Directories named .git or .nodex are never traversed or re-included. .gitignore is not consulted. Ordinary development, output, evidence and agent directories receive no implicit exclusion.

### Allowed Differences

Fixture inventory and filesystem support for non-regular entries may vary; record which controls were constructed. Syntax recognition belongs to Syntax.

### Regression Criteria

Discovery escapes through a link, emits absolute/unclean paths, traverses control directories or silently gives a repository convention Product meaning.

### Dependencies

PRJ-ROOT-002

## PRJ-PATH-002 — Confined exact-byte source reads

Status:
ACTIVE

Spec Version:
1

Type:
SECURITY

Priority:
CRITICAL

### Purpose

Read only a canonical logical regular file beneath the established boundary.

### Preconditions

A disposable Project facade harness; a known byte file, final and intermediate links, directories, controls and an outside sentinel.

### Actions

Read the valid logical path and compare bytes. Attempt empty, dot, absolute, escaping and unclean paths, control components, directory/non-regular targets and final/intermediate symlinks. Read a valid manually excluded file directly.

### Expected Invariants

ReadFile returns the exact valid file bytes. Invalid logical paths are rejected without cleaning or repair. Control components, links and non-regular targets are rejected; outside bytes are not returned. ReadFile does not apply exclusions.

### Allowed Differences

Error wording and temporary paths vary. No race-proof filesystem transaction guarantee is asserted.

### Regression Criteria

A rejected path is repaired, an exclusion changes a direct valid read, or a read follows a link/escapes the boundary.

### Dependencies

PRJ-ROOT-002
