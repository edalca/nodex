# Environment

## Purpose

CLI/runtime and supported build boundaries outside the five bounded roots.

## Scope

Public command grammar, state-independent dispatch, version identity and toolchain independence. Root/path selection, catalog content and source extraction remain with their owning groups.

## Group Invariants

Usage prose is not frozen. Build identity is source/link metadata, never runtime repository identity.

All fixtures and observations follow [the manual-validation reference law](README.md). Nodex collects, indexes, localizes and retrieves; human/LLM analyzes.

## Test Index

| ID | Name | Type | Priority | Spec | Status |
| --- | --- | --- | --- | --- | --- |
| ENV-CLI-001 | Public command and global-option grammar | NEGATIVE | HIGH | 1 | ACTIVE |
| ENV-CLI-002 | State-independent inspection commands | BOUNDARY | HIGH | 1 | ACTIVE |
| ENV-BUILD-001 | Source-defined version and linked metadata | BOUNDARY | HIGH | 1 | ACTIVE |
| ENV-BUILD-002 | CGO-free build and runtime | BOUNDARY | HIGH | 1 | ACTIVE |

## Test Specifications

## ENV-CLI-001 — Public command and global-option grammar

Status:
ACTIVE

Spec Version:
1

Type:
NEGATIVE

Priority:
HIGH

### Purpose

Keep command routing and usage errors predictable.

### Preconditions

A built Nodex binary; disposable cwd; capture stdout, stderr and exit status separately.

### Actions

Invoke index generate/comments/declarations/status/show, version, ignore presets/list/enable/disable, and skill targets/show/install/uninstall with their documented arguments. Try no command, unknown commands, former top-level generate/comments/status/show, duplicate or missing global values, and global options after a command. Compare separated and equals forms of --root and --out-dir, in either order before the command.

### Expected Invariants

The top-level commands are index, version, ignore and skill. Snapshot operations require index. Global --root and --out-dir accept separated and equals non-empty values at most once before the top-level command. Unknown commands/options and missing arguments fail with a usage diagnostic. A global -- separator ends option parsing.

### Allowed Differences

Exact diagnostic/help wording and fixture-dependent successful output may differ; no general --help success contract is asserted.

### Regression Criteria

An obsolete top-level snapshot command succeeds, malformed global options are accepted, or options after a command change global selection.

### Dependencies

NONE

## ENV-CLI-002 — State-independent inspection commands

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Allow public catalogs and build identity to be inspected without a project.

### Preconditions

A built binary; an existing disposable cwd with invalid project markers; nonexistent option paths; record the cwd tree before invocation.

### Actions

Run version, ignore presets, skill targets and skill show, each with well-formed --root and --out-dir pointing to nonexistent paths. Compare with invocation without those options.

### Expected Invariants

Each command succeeds without opening source/workspace paths or resolving markers. Its output equals the same command without global path options. No control or destination state is created.

### Allowed Differences

Build metadata can differ between binaries; compare within the same binary. Catalog/content correctness belongs to its owning group.

### Regression Criteria

An independent command fails because a supplied path or project marker is invalid, or creates state.

### Dependencies

ENV-CLI-001

## ENV-BUILD-001 — Source-defined version and linked metadata

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Distinguish Product version from build metadata.

### Preconditions

The supported Go toolchain and current source; build outputs outside the checkout.

### Actions

Read the version constant in cmd/nodex/version.go. Build normally, then build with -ldflags "-X main.commit=manual-commit -X main.buildDate=manual-date". Run both version commands from unrelated disposable directories.

### Expected Invariants

Output has `nodex <source version>`, `commit <metadata>` and `built <metadata>` lines. Ordinary builds use unknown for both metadata fields; the linked build reports the supplied values with the same Product version. Invocation cwd does not determine identity. Runtime does not inspect version control.

### Allowed Differences

The source Product version and deliberately linked metadata are dynamic; this spec freezes neither a development revision nor a release date.

### Regression Criteria

Product version is inferred from cwd/VCS or changed by metadata linking, or an ordinary build invents metadata.

### Dependencies

ENV-CLI-002

## ENV-BUILD-002 — CGO-free build and runtime

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Keep supported indexing available without a native compiler or JavaScript toolchain.

### Preconditions

Go toolchain matching go.mod; pinned modules available; a representative supported host and authored five-language fixture; external build destination.

### Actions

Build with CGO_ENABLED=0 go build -o `<external-binary>` ./cmd/nodex. Inspect go version -m. With no C compiler, Node or TypeScript compiler required by the invocation, run version and index generate on the authored fixture.

### Expected Invariants

The build succeeds with CGO disabled. The binary indexes Go, JS, JSX, TS and TSX through its configured capabilities without invoking C, Node or a TypeScript compiler.

### Allowed Differences

Binary size, Go build metadata, host path and build duration may differ. Binary byte reproducibility and every possible OS/architecture are outside this law.

### Regression Criteria

Building or running the supported workflow requires CGO, a native compiler, Node or a TypeScript compiler.

### Dependencies

NONE
