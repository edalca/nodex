# Index

## Purpose

Construct, persist, compare and retrieve derived neutral facts.

## Scope

Explicit generation, deterministic snapshot-local identity, exact relationship resolution, disposable storage integrity, currentness and public queries. Extraction/attachment/context byte selection remain Syntax-owned.

## Group Invariants

The snapshot commits the derived set; IDs are local to it. Currentness compares effective inputs. Inspection/query commands never regenerate. Docs IDs resolve existing physical facts and never carry comment text.

All fixtures and observations follow [the manual-validation reference law](README.md). Nodex collects, indexes, localizes and retrieves; human/LLM analyzes.

## Test Index

| ID | Name | Type | Priority | Spec | Status |
| --- | --- | --- | --- | --- | --- |
| IDX-GEN-001 | Explicit generation from included sources | POSITIVE | CRITICAL | 1 | ACTIVE |
| IDX-GEN-002 | Byte-identical unchanged regeneration | IDEMPOTENCY | CRITICAL | 1 | ACTIVE |
| IDX-GEN-003 | Canonical snapshot-local IDs and exact Docs resolution | BOUNDARY | CRITICAL | 1 | ACTIVE |
| IDX-STATE-001 | Fingerprint and policy currentness | BOUNDARY | CRITICAL | 1 | ACTIVE |
| IDX-STATE-002 | Read-only state inspection and refusal | BOUNDARY | HIGH | 1 | ACTIVE |
| IDX-STORAGE-001 | Snapshot commit marker and integrity | DURABILITY | CRITICAL | 1 | ACTIVE |
| IDX-STORAGE-002 | Strict neutral persisted records | NEGATIVE | HIGH | 1 | ACTIVE |
| IDX-QUERY-001 | Compact comment discovery | POSITIVE | HIGH | 1 | ACTIVE |
| IDX-QUERY-002 | Compact declaration discovery | POSITIVE | HIGH | 1 | ACTIVE |
| IDX-QUERY-003 | Logical discovery filters | BOUNDARY | HIGH | 1 | ACTIVE |
| IDX-QUERY-004 | Current ID retrieval and source reread guard | BOUNDARY | CRITICAL | 1 | ACTIVE |

## Test Specifications

## IDX-GEN-001 — Explicit generation from included sources

Status:
ACTIVE

Spec Version:
1

Type:
POSITIVE

Priority:
CRITICAL

### Purpose

Build one committed comment/declaration snapshot from the effective source set.

### Preconditions

Existing disposable source/workspace with supported clean files, unsupported files, comment-free source and optional exclusions; independently authored inventory.

### Actions

Run index generate from an empty workspace. Inspect generated counts, all three derived files and loaded facts. Repeat with no supported sources. Make an included Go file malformed or introduce a controlled read/discovery/policy failure before publication and compare saved committed bytes.

### Expected Invariants

Generation collects comments and declarations from one parse of each included supported source and stores both, including undocumented declarations. Unsupported/manual/preset-excluded sources contribute no fingerprints/facts. Empty inventories produce a usable empty snapshot. Failures during selection, parsing or preparation publish no partial replacement of an existing committed index. Only explicit generate regenerates state.

### Allowed Differences

Counts and facts depend on authored inventory; ECMAScript recovery can succeed with comments but no declarations under its Syntax law. Failure after publication begins has the storage limitations in IDX-STORAGE-001.

### Regression Criteria

Generation drops comment-free declarations, fingerprints excluded/unsupported inputs, parses files independently for comments versus declarations or replaces committed bytes on pre-publication failure.

### Dependencies

PRJ-PATH-001, PRJ-ROOT-003, IGN-PRESET-001, SYN-CORE-002

## IDX-GEN-002 — Byte-identical unchanged regeneration

Status:
ACTIVE

Spec Version:
1

Type:
IDEMPOTENCY

Priority:
CRITICAL

### Purpose

Keep canonical derived output deterministic across fixtures and repeated generation.

### Preconditions

Included supported sources and effective policy unchanged; disposable workspace; binary fixed for the comparison; external copies of derived files.

### Actions

Generate three times and save comments.jsonl, declarations.jsonl and snapshot.json outside the source boundary after each generation. Compare exact bytes/hashes for each file. In a facade harness also permute input documents/facts without changing them.

### Expected Invariants

Unchanged source/configuration and binary yield byte-identical canonical comments.jsonl, declarations.jsonl and snapshot.json across generation. Caller input order cannot change successful index identities/output. Timestamps, absolute locations and map order are not emission inputs.

### Allowed Differences

Build duration, generation output filesystem mtimes and scratch-copy locations may vary. This is derived-file determinism, not reproducible binary bytes or permanent IDs after source changes.

### Regression Criteria

Any canonical derived file changes bytes without an input change or input ordering changes successful canonical facts.

### Dependencies

IDX-GEN-001

## IDX-GEN-003 — Canonical snapshot-local IDs and exact Docs resolution

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
CRITICAL

### Purpose

Assign structural identities and resolve parser facts without synthesizing comments.

### Preconditions

A disposable index/syntax facade harness with distinct files, repeated text, reordered inputs, 0/1/several direct ranges and invalid relationship controls.

### Actions

Build and inspect IDs/order/Docs. Insert an earlier fact and rebuild. Try duplicate comment/declaration identities, duplicate Docs, missing/middle/cross-file/overlapping ranges and mismatched coordinates.

### Expected Invariants

Comments order by logical path then physical start/end and remaining coordinates; C IDs begin C000001. Declarations order by path/start/end/kind; D IDs begin D000001 independently. Both widths are at least six digits and are snapshot-local. Docs resolves complete exact same-file ranges to independent existing comment IDs in physical order, with [] for absence and no comment text. Duplicate/unresolved facts or relationships fail the whole Build with no partial index.

### Allowed Differences

IDs may change after source/input changes. Identical text at distinct ranges does not make facts duplicates; declaration identity includes kind. No huge fixture is required merely to reach wider ordinals; canonical ID parsing can be inspected separately.

### Regression Criteria

Text/input order determines IDs, relationships resolve fuzzily/across files, duplicates silently collapse or a failed build returns partial facts.

### Dependencies

SYN-CORE-002

## IDX-STATE-001 — Fingerprint and policy currentness

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
CRITICAL

### Purpose

Decide currentness from effective inputs without rediscovering indexed facts.

### Preconditions

A generated disposable snapshot; identical source tree copy at another root; supported included/excluded and unsupported files; editable policy presentation and mtimes.

### Actions

Inspect status unchanged, then independently edit/add/remove/rename an included source or change policy membership. Restore bytes and alter only mtime, JSON formatting/field/preset order, excluded files or control state. Reuse the snapshot with the identical copied tree.

### Expected Invariants

Currentness compares policy identity and ordered logical path/language/SHA-256 exact-byte fingerprints. Included changes or effective policy changes stale the index; formatting/preset ordering, mtimes, excluded sources and absolute source/workspace identity do not. Indexed comments/declarations/Docs are not reparsed for comparison. Only enabled structural preset selection may inspect structure; without it currentness uses bytes and policy identity.

### Allowed Differences

Tree locations and mtimes may vary. Corrupt derived files are rejected by storage before currentness; a valid derived digest is not itself a freshness input.

### Regression Criteria

An included byte or policy change remains current, excluded/mtime/location/presentation changes stale it, or currentness reparses comment/declaration facts.

### Dependencies

IDX-GEN-001, IGN-RULE-003

## IDX-STATE-002 — Read-only state inspection and refusal

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Keep missing/stale/corrupt inspections separate from regeneration.

### Preconditions

Disposable missing/current/stale/corrupt snapshots and saved full workspace bytes/inventory; an invalid policy or unreadable source control.

### Actions

Run index status in each state; run comments, declarations and show against missing/stale/corrupt states. Compare bytes/inventory before/after. Try operational failures preventing input inspection.

### Expected Invariants

Status reports missing/current/stale/corrupt as successful inspections and creates/modifies nothing. Operational inability to inspect inputs is an error. Comments/declarations/show require a current usable snapshot, fail on missing/stale/corrupt state and emit no partial successful result. None regenerates state; ignore edits do not regenerate it either.

### Allowed Differences

Diagnostics and informational counts vary with state/fixture. Existing unrelated workspace files remain untouched.

### Regression Criteria

A read command regenerates/rewrites state, treats an operational failure as an inspected state or emits successful facts from unusable state.

### Dependencies

IDX-STATE-001, IDX-STORAGE-001

## IDX-STORAGE-001 — Snapshot commit marker and integrity

Status:
ACTIVE

Spec Version:
1

Type:
DURABILITY

Priority:
CRITICAL

### Purpose

Reject incomplete/mixed derived generations rather than loading partial state.

### Preconditions

Two different valid disposable generations saved outside the source tree; controlled copies for tampering and symlinks; outside sentinel; storage facade harness as needed.

### Actions

Inspect publication code as the frozen structural boundary: temporary files, comments first, declarations second, snapshot last. Load sets with absent snapshot, missing derived files, mixed-generation files, altered counts/digests and linked/non-directory committed paths. Exercise a preparation failure.

### Expected Invariants

The derived set lives at `<workspace>/.nodex/index/{comments.jsonl,declarations.jsonl,snapshot.json}`; snapshot is the commit marker with exact digests/counts for both derived files. No snapshot means no committed index. Missing/mismatched/mixed committed data returns no partial index. Directories must be real and committed paths reject links. Preparation is validated before publication; temporary files are used and snapshot publishes last. Generation does not edit ignore.json.

### Allowed Differences

The three renames are not one filesystem transaction: interruption between renames can leave a detectable corrupt set. This spec promises detection, not rollback or always-available old generations.

### Regression Criteria

An incomplete/mixed set is accepted, links are followed, snapshot publishes before derived files or generation edits ignore configuration.

### Dependencies

IDX-GEN-001

## IDX-STORAGE-002 — Strict neutral persisted records

Status:
ACTIVE

Spec Version:
1

Type:
NEGATIVE

Priority:
HIGH

### Purpose

Persist only the canonical structural records and reject malformed shapes.

### Preconditions

A disposable generated snapshot with comments, nameless/documented/undocumented declarations; valid-digest tampered copies to test record validation rather than merely digest rejection.

### Actions

Inspect JSON shapes; mutate required/unknown fields, integer schema, UTF-8, trailing data, JSONL newline/order/ID sequences, ranges, names and Docs. Try legacy doc, absent/null/scalar docs, dangling/cross-file/reversed/duplicate IDs; recompute file digests/counts for deliberate shape controls.

### Expected Invariants

Schema is integer 1. Comments store ID/path/language/range/normalized text; declarations store ID/path/language/kind/names/range/ordered docs arrays, with [] for absence. Snapshot stores policy identity, file digests/counts and source fingerprints. No raw source, context, trees, declaration comment text, absolute paths, timestamps or machine identity are stored. Strict loading rejects malformed/unknown/missing/trailing shapes, bad ordering/IDs and invalid relationships, including legacy singular doc, without a partial index.

### Allowed Differences

Authored normalized comment content may contain arbitrary text including timestamps or path-like prose; the prohibited values are generated identity/metadata, not censorship of source text.

### Regression Criteria

Legacy/invalid/null/duplicate/dangling Docs load successfully, canonical record invariants are bypassed or private/generated identity payload appears.

### Dependencies

IDX-STORAGE-001, IDX-GEN-003

## IDX-QUERY-001 — Compact comment discovery

Status:
ACTIVE

Spec Version:
1

Type:
POSITIVE

Priority:
HIGH

### Purpose

Render indexed comments for discovery without duplicating retrieval metadata.

### Preconditions

A current disposable index with empty, repeated, multiline, directive and Markdown-bearing normalized comment texts; unfiltered commands.

### Actions

Run index comments and compare every block to the loaded indexed comment. Repeat with a current empty comment selection/index; capture stdout bytes.

### Expected Invariants

Each canonical-order block contains only its C ID, logical source-root-relative file path and normalized Text, unchanged apart from necessary block-separating/newline rendering. It adds no physical line metadata, raw source, context or analysis. Zero comments emits zero bytes.

### Allowed Differences

Comment text itself may resemble Markdown/metadata; the renderer does not reinterpret it. IDs depend on the snapshot, not text.

### Regression Criteria

Text is analyzed/rewritten, metadata/context is added, order/IDs change or empty output gains a banner.

### Dependencies

IDX-GEN-003, IDX-STATE-002

## IDX-QUERY-002 — Compact declaration discovery

Status:
ACTIVE

Spec Version:
1

Type:
POSITIVE

Priority:
HIGH

### Purpose

Render neutral declaration facts and direct IDs with explicit absence.

### Preconditions

A current disposable index with multiple names, no names, zero/one/several Docs and empty declaration selection/index.

### Actions

Run index declarations and compare each block to loaded declarations, including docs and names ordering. Capture empty-output bytes.

### Expected Invariants

Each canonical-order block contains D ID, logical file, kind, names and docs. Several names or Docs IDs use comma-space. No names is names: with no value; no Docs is docs: none. No comment text/context or quality judgment is added. Zero declarations emits zero bytes.

### Allowed Differences

Facts/IDs depend on source and snapshot. A real declaration named none remains a name, distinct from empty names and absence of Docs.

### Regression Criteria

Names are invented, IDs merge or reorder, absence is treated as a defect, documentation text is copied or empty output has extra bytes.

### Dependencies

IDX-GEN-003, IDX-STATE-002

## IDX-QUERY-003 — Logical discovery filters

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Select already loaded facts without altering canonical identity or state.

### Preconditions

A current disposable index with sibling subtree names, exact/case-variant kinds/names, multiple and empty names, and absent logical selectors; saved unfiltered output/state.

### Actions

Use comments --file and declarations --file/--kind/--name in varied orders with several and duplicate values. Compare OR within categories, AND across them, exact file or selector + slash descendants, unknown kinds/names and no matches. Try duplicate flags, unsupported options, missing values and unclean/absolute file selectors against even absent state.

### Expected Invariants

Each filter occurs once with one or more consecutive values until the next filter/end. Values deduplicate preserving first occurrence. Files match exact logical paths or descendants only; no filesystem resolution/glob semantics. Kinds and any indexed name match exactly/case-sensitively without aliases; nameless facts fail a supplied name filter. Results retain canonical order, IDs and rendering; no match succeeds with zero bytes. Usage errors occur before opening state; valid filters do not change currentness/persistence/workspace.

### Allowed Differences

Selectors need not exist. Unknown kinds match nothing; same-named facts across languages remain distinct facts.

### Regression Criteria

Filters renumber/sort/rewrite, sibling prefixes leak, aliases/case-folding appear, categories combine incorrectly or malformed filters inspect state.

### Dependencies

IDX-QUERY-001, IDX-QUERY-002

## IDX-QUERY-004 — Current ID retrieval and source reread guard

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
CRITICAL

### Purpose

Retrieve original context only for resolved current snapshot-local identities.

### Preconditions

A current disposable index with C/D IDs across files, repeated requested IDs, nameless/direct-doc declarations and fence-like source; a disposable CLI harness can change bytes between currentness and context reread.

### Actions

Run index show with mixed IDs in varied order and duplicates. Include a malformed/unknown ID after a valid one. Compare physical locations and context bytes to source. In the controlled reread harness change the target source after currentness but before context read.

### Expected Invariants

Show preserves requested order/duplicates. Comment output has logical file, physical comment lines and bounded context, without repeating normalized text separately. Declaration output also has kind/names/ordered Docs IDs or none, without separate documentation text. Currentness is established before context parsing; reread bytes must still match the fingerprint. Any malformed/unknown ID, changed reread or context failure yields no partial successful output. Rendering fences preserve the source payload safely; only a terminating rendering newline may be added.

### Allowed Differences

IDs, physical locations and language-specific context container vary by fixture. Context bounds/byte selection belong to SYN-CORE-003/004; the guard timing may require a controlled harness rather than an unreliable race.

### Regression Criteria

Order/duplicates change, an invalid later ID leaks partial output, stale reread bytes produce context or normalized documentation is repeated as metadata.

### Dependencies

IDX-STATE-002, SYN-CORE-003, SYN-CORE-004
