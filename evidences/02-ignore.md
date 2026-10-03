# Ignore

## Purpose

Compile and maintain generic workspace exclusion policy.

## Scope

Strict ignore documents, exact manual matching, compiled identity and concrete preset storage/edits. Go preset meanings belong to SYN-GO-005; the syntax registry supplies selectors.

## Group Invariants

No optional exclusion is enabled by default. Invalid policy never partially applies. Dependencies on Project do not transfer policy ownership.

All fixtures and observations follow [the manual-validation reference law](README.md). Nodex collects, indexes, localizes and retrieves; human/LLM analyzes.

## Test Index

| ID | Name | Type | Priority | Spec | Status |
| --- | --- | --- | --- | --- | --- |
| IGN-RULE-001 | Complete strict ignore document | NEGATIVE | HIGH | 1 | ACTIVE |
| IGN-RULE-002 | Exact ordered path matching and sealed ancestors | BOUNDARY | HIGH | 1 | ACTIVE |
| IGN-RULE-003 | Compiled policy identity | IDEMPOTENCY | HIGH | 1 | ACTIVE |
| IGN-PRESET-001 | Concrete preset persistence and selector validation | NEGATIVE | HIGH | 1 | ACTIVE |
| IGN-PRESET-002 | Safe idempotent policy edits | IDEMPOTENCY | HIGH | 1 | ACTIVE |

## Test Specifications

## IGN-RULE-001 — Complete strict ignore document

Status:
ACTIVE

Spec Version:
1

Type:
NEGATIVE

Priority:
HIGH

### Purpose

Accept only a complete schema-1 policy with ordered manual patterns.

### Preconditions

Disposable workspace; copies of valid and malformed ignore documents; current binary.

### Actions

Use ignore list and generation with absent config, valid arrays, reordered JSON fields and malformed cases: invalid UTF-8, wrong/non-integer schema, missing/null arrays, unknown fields, trailing data, non-string/empty entries, duplicate presets and malformed patterns.

### Expected Invariants

Missing config means zero optional exclusions and creates nothing. A valid document has integer schema 1, presets and exclude arrays. Manual patterns retain order and duplicates; presets are a lexically canonical set. Any invalid entry rejects the entire document, never a partially compiled policy. Known identifier validation is owned by IGN-PRESET-001.

### Allowed Differences

JSON formatting and field order may vary; diagnostic wording is not frozen.

### Regression Criteria

Invalid config proceeds with partial policy, manual order/duplicates are discarded, or a read creates config.

### Dependencies

PRJ-ROOT-003

## IGN-RULE-002 — Exact ordered path matching and sealed ancestors

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Apply generic manual exclusions without hidden matching conventions.

### Preconditions

A disposable policy/discovery fixture with root/nested names, Unicode, case variants, dotfiles and traversable/sealed directories.

### Actions

Exercise bare names, leading/internal slash anchoring, unescaped trailing directory slash, *, ?, whole-element **, character classes and escapes. Compare vendor/ plus !vendor/patched.go with vendor/* plus that negation. Reverse conflicting rules and retain duplicates.

### Expected Invariants

Last matching rule decides; ! negates. Bare names match any depth; slash-containing patterns anchor at root; directory-only patterns require a directory. * stays in one element, ? consumes one rune and ** recurses only as a whole element. Excluded ancestors seal subtrees. Candidates are not cleaned, case-folded or Unicode-normalized; dotfiles have no special matching treatment.

### Allowed Differences

Authored inventories may differ; expected match tables must be established from these rules before invocation.

### Regression Criteria

Negation reaches a sealed ancestor, rules reorder, wildcards cross forbidden boundaries or matching silently normalizes candidates.

### Dependencies

IGN-RULE-001, PRJ-PATH-001

## IGN-RULE-003 — Compiled policy identity

Status:
ACTIVE

Spec Version:
1

Type:
IDEMPOTENCY

Priority:
HIGH

### Purpose

Identify effective policy rather than JSON presentation.

### Preconditions

A disposable ignore facade harness or snapshot policy-identity inspection; equivalent and distinct policies.

### Actions

Compare identities for whitespace/field-order/preset-order variations and equivalent compiled patterns such as a/b versus /a/b and a*b versus a**b. Change concrete presets, rule order/duplicates, negation or directory-only behavior.

### Expected Invariants

Identity is a deterministic SHA-256 digest of the canonical preset set and ordered compiled manual rules. Presentation changes and equivalent compiled rules preserve identity; changes to the represented preset set or compiled rule sequence change identity. Manual duplicates remain represented.

### Allowed Differences

This law does not freeze private hash-encoding bytes or equate every logically equivalent matcher; identity follows compiled representation.

### Regression Criteria

JSON formatting changes identity, represented rule order is lost, or preset membership is absent from identity.

### Dependencies

IGN-RULE-001

## IGN-PRESET-001 — Concrete preset persistence and selector validation

Status:
ACTIVE

Spec Version:
1

Type:
NEGATIVE

Priority:
HIGH

### Purpose

Persist concrete exclusions while command aggregates remain expansion tokens.

### Preconditions

Disposable workspace; binary preset catalog; no config and then config with ordered duplicate manual rules.

### Actions

Run ignore presets, enable go:all, list, disable go:all, and attempt unknown selectors. Supply ignore documents containing go:all, unknown identifiers or duplicate concrete identifiers before generation/status.

### Expected Invariants

No preset is enabled by default. CLI selectors expand to known concrete IDs before storage; go:all is never persisted. Unknown selectors and aggregate/unknown/duplicate persisted IDs are rejected before generation/currentness proceeds. Generic Ignore stores IDs; language catalog and meanings remain Syntax-owned.

### Allowed Differences

Catalog expansion follows the running binary; Go meaning is specified by SYN-GO-005 rather than duplicated here.

### Regression Criteria

An aggregate persists, unsupported policy proceeds, or a language-specific meaning moves into generic matching.

### Dependencies

IGN-RULE-001

## IGN-PRESET-002 — Safe idempotent policy edits

Status:
ACTIVE

Spec Version:
1

Type:
IDEMPOTENCY

Priority:
HIGH

### Purpose

Edit enabled presets without disturbing manual policy or derived state.

### Preconditions

Disposable workspace with saved ignore/index bytes; unmanaged outside sentinel; link/non-directory controls.

### Actions

Enable a concrete preset twice, disable it twice, and disable an absent preset with no config. Compare document bytes/mtime on no-op edits and index bytes throughout. Attempt edits through .nodex or ignore.json symlinks and a publication failure in a disposable harness.

### Expected Invariants

Edits preserve manual order and duplicates, validate complete policy, and publish using a temporary file and rename. Identity-preserving edits do not rewrite or create config. Index is not regenerated or rewritten. Control links and invalid types are rejected without changing their targets; failed pre-publication writes expose no partial document.

### Allowed Differences

Timestamps vary on actual edits; timestamp resolution must be adequate to distinguish a rewrite. Crash atomicity across unrelated files is not claimed.

### Regression Criteria

A no-op writes state, manual rules change, links are followed, or a failed preparation publishes partial config.

### Dependencies

IGN-PRESET-001
