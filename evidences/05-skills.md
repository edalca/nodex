# Skills

## Purpose

Deliver the canonical embedded Nodex Agent Skill safely.

## Scope

Built-in document identity, actual target inventory, local/home placement, idempotent managed updates and protected removal. No MCP Skill surface is specified.

## Group Invariants

Target changes only destination. Embedded prose describes public workflow and never executes parsing/indexing or defines their laws. Agent destinations are ordinary directories, not Nodex control state.

All fixtures and observations follow [the manual-validation reference law](README.md). Nodex collects, indexes, localizes and retrieves; human/LLM analyzes.

## Test Index

| ID | Name | Type | Priority | Spec | Status |
| --- | --- | --- | --- | --- | --- |
| SKL-PKG-001 | Canonical embedded provider-neutral Skill | POSITIVE | HIGH | 1 | ACTIVE |
| SKL-PKG-002 | Supported Skill target destinations | BOUNDARY | HIGH | 1 | ACTIVE |
| SKL-INSTALL-001 | Local and home placement without control state | BOUNDARY | HIGH | 1 | ACTIVE |
| SKL-INSTALL-002 | Exact idempotent managed installation | IDEMPOTENCY | HIGH | 1 | ACTIVE |
| SKL-INSTALL-003 | Non-clobbering installation and removal | SECURITY | CRITICAL | 1 | ACTIVE |

## Test Specifications

## SKL-PKG-001 — Canonical embedded provider-neutral Skill

Status:
ACTIVE

Spec Version:
1

Type:
POSITIVE

Priority:
HIGH

### Purpose

Deliver one built-in workflow document without making it executable Product authority.

### Preconditions

A current built binary and matching embedded source asset; no project required.

### Actions

Capture skill show bytes and compare with internal/skill/assets/nodex/SKILL.md from the source used to build. Inspect frontmatter, ownership marker and workflow boundary; run from unrelated directories without runtime asset availability.

### Expected Invariants

Skill show emits the exact embedded canonical bytes, including final newline, with name nodex and `<!-- nodex-managed-skill:v1 -->` ownership marker. The document is provider-neutral and describes public status/discovery/show/analysis workflow. Runtime checkout files/downloads are not content sources. Nodex distributes this Skill; it does not execute it, analyze source through it or use prose as parser/index authority.

### Allowed Differences

Canonical wording may evolve deliberately; this law freezes identity/delivery/boundary rather than every sentence across versions. Compare against the matching build source.

### Regression Criteria

Target/runtime location rewrites content, a download/checkout supplies bytes or Skill execution/semantic analysis becomes Nodex behavior.

### Dependencies

NONE

## SKL-PKG-002 — Supported Skill target destinations

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Select destinations without multiplying the canonical document.

### Preconditions

Binary target catalog and disposable existing project/home bases; unsupported and case-variant names.

### Actions

Run skill targets; inspect destinations through the skill facade and install each target in isolated bases. Attempt unknown/case-variant targets.

### Expected Invariants

Targets are agents, claude, codex, gemini and grok in lexical order with case-sensitive names and no aliases. Project paths are .agents/skills/nodex/SKILL.md for agents/gemini and .<target>/skills/nodex/SKILL.md for claude/codex/grok. Home paths are the analogous paths except gemini uses .gemini/config/skills/nodex/SKILL.md. Every target receives identical canonical bytes; unsupported names fail.

### Allowed Differences

Temporary absolute bases differ; destination paths, not provider behavior/integration, are the supported surface.

### Regression Criteria

An unsupported alias appears, the shared project path diverges or provider-specific bytes are emitted.

### Dependencies

SKL-PKG-001

## SKL-INSTALL-001 — Local and home placement without control state

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Install at the selected existing base while keeping Skills separate from workspace state.

### Preconditions

Disposable project with nested cwd/markers and explicit root; isolated home via controlled user-home environment/harness; invalid out-dir; saved source/control inventory.

### Actions

Install/uninstall locally under automatic and explicit root selection, then globally in the isolated home. Supply --out-dir pointing to an invalid path and try --root with --global. Try missing/non-directory bases through the facade.

### Expected Invariants

Local operations use ordinary Project source resolution, including explicit --root. --global uses the current user home without project discovery and rejects --root combination. --out-dir has no effect and is not opened. The base must already exist as a directory; installer creates only needed destination directories beneath it. Skill operations do not create/modify .nodex or installed-target state.

### Allowed Differences

Home/project absolute bases vary; use isolation to avoid modifying a real user installation. Source root selection law stays Project-owned.

### Regression Criteria

Out-dir changes destination, missing base is created, global opens project or Skill operations modify control/index state.

### Dependencies

SKL-PKG-002, PRJ-ROOT-001, PRJ-ROOT-002

## SKL-INSTALL-002 — Exact idempotent managed installation

Status:
ACTIVE

Spec Version:
1

Type:
IDEMPOTENCY

Priority:
HIGH

### Purpose

Install canonical bytes and upgrade only Nodex-managed copies.

### Preconditions

Isolated valid bases/targets; saved canonical bytes and mtimes; an older/different regular file carrying the ownership marker.

### Actions

Install fresh, compare exact bytes with skill show, reinstall and compare bytes/mtime. Replace the fixture with a different managed file and install again. Repeat agents then gemini at their shared project path.

### Expected Invariants

Fresh install writes exact embedded bytes via temporary file/sync/rename. An identical installed file is left in place. A differing regular file containing the ownership marker can be replaced by canonical bytes. Shared-path targets are idempotent; target names/timestamps are not inserted into content.

### Allowed Differences

Filesystem metadata changes on real replacement are allowed; no-op mtime comparison requires adequate resolution. The marker is ownership, not a content hash.

### Regression Criteria

Reinstall rewrites identical bytes, managed upgrade fails solely for content difference or installed bytes gain target/time data.

### Dependencies

SKL-INSTALL-001

## SKL-INSTALL-003 — Non-clobbering installation and removal

Status:
ACTIVE

Spec Version:
1

Type:
SECURITY

Priority:
CRITICAL

### Purpose

Protect unmanaged files and linked destinations during Skill operations.

### Preconditions

Disposable destinations containing unmanaged SKILL.md, managed SKILL.md, unrelated sibling files, intermediate/final links, non-regular targets and outside sentinels.

### Actions

Attempt install and uninstall against unmanaged/link/non-regular controls. Remove a managed file with/without siblings; uninstall again when absent. Compare sentinel/sibling/parent contents and directory inventory.

### Expected Invariants

Unmanaged SKILL.md is neither overwritten nor deleted. Symbolic links beneath the base and non-regular destinations fail without following/deleting targets. Managed uninstall removes only SKILL.md and an empty final nodex directory, retaining other files and parent directories. Missing destination uninstall succeeds without creating state. Preparation failure leaves no partial skill.

### Allowed Differences

A marker-carrying file is deliberately managed even if its content differs. No general atomic filesystem race defense or parent-directory garbage collection is promised.

### Regression Criteria

Unmanaged/sibling/outside content changes, links are followed/removed, parent directories are deleted or absent uninstall creates state.

### Dependencies

SKL-INSTALL-001
