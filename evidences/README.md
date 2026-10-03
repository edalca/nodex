# Canonical manual Product validation

`evidences/` contains tracked canonical manual Product test specifications. A specification defines a current Nodex behavioral law independently of any execution, repository fixture, timestamp, result file or development commit. Canonical specifications contain neither execution timestamps nor results.

Nodex collects, indexes, localizes and retrieves. Human/LLM analyzes. This suite validates physical collection, structural relationships, source localization, derived state and public retrieval/delivery. It does not make Nodex responsible for syntax correction, compilation, type checking, symbol resolution, documentation quality/correctness/staleness, recommendations, rewriting, semantic interpretation or LLM analysis. Go parser rejection is its extraction boundary; ECMAScript parsing does not certify syntax validity. A successful extraction is never a compiler correctness claim.

## Territories and reference direction

| File | Group | Responsibility |
| --- | --- | --- |
| [00-environment.md](00-environment.md) | ENV | Public CLI grammar, state-independent runtime inspection and supported build boundaries |
| [01-project.md](01-project.md) | PRJ | Source-root/workspace selection, filesystem discovery and confined logical reads |
| [02-ignore.md](02-ignore.md) | IGN | Generic manual exclusions, strict policy identity and concrete preset storage/edits |
| [03-syntax.md](03-syntax.md) | SYN | Neutral and language-specific structural extraction, direct Docs, source context and language preset meanings |
| [04-index.md](04-index.md) | IDX | Derived generation/IDs, persistence integrity, input currentness and discovery/retrieval queries |
| [05-skills.md](05-skills.md) | SKL | Canonical embedded Skill identity, destinations and managed installation/removal |

There is one canonical owner for each behavioral law. A wrong Go direct Docs relationship belongs to Syntax; nondeterministic `comments.jsonl` generation belongs to Index; a file excluded incorrectly by generic ignore rules belongs to Ignore; a logical root/path escape belongs to Project; a Skill installer overwrite belongs to Skills. A Go language preset's classification belongs to Syntax, while storage of its concrete identifier belongs to Ignore.

An execution may reveal a law through another subsystem. Dependencies may cross groups but do not transfer ownership or duplicate prerequisite specification prose. Dependency targets must exist and the specification dependency graph must remain acyclic. Use dependencies only when they clarify an actual prerequisite; each body remains understandable on its own.

The reference direction is:

```text
concrete execution evidence
    -> Test ID + Spec Version
    -> canonical specification
```

Executions never define, replace or duplicate normative specification bodies. External repositories are execution fixtures, not territories. No repository name, URL or exact external revision is frozen into a canonical law.

## Four distinct records

| Record | Authority and location |
| --- | --- |
| Canonical manual Product specifications | Tracked laws under `evidences/`; current expected behavior, independent of execution history |
| Concrete manual execution evidence | Ignored local records under `evidences/runs/`; actual commands/observations compared to Test ID + Spec Version |
| Automated test/CI output | Automated validation receipts produced by test/CI tooling; neither canonical manual law nor a manual execution merely because tests pass |
| Engineering provenance | `.outputs/`: engineering investigation, research spikes, implementation provenance, audits, build receipts and commit preparation reports |

`.outputs/` remains separate. Do not migrate old reports into this suite or retroactively convert implementation reports into manual execution evidence. Repository evidence/engineering directory names do not become Nodex exclusions in analyzed source projects; only independent Product selection laws determine that behavior.

## Test identity, version and status

Canonical IDs have the form `<GROUP>-<SUBJECT>-<NNN>`: uppercase group and stable subject followed by a three-digit ordinal starting at `001`. Numbers are continuous independently within each exact prefix in this initial baseline. There is no global counter. For example, `SYN-CORE-001`, `SYN-CORE-002`, `SYN-GO-001`, `SYN-GO-002` and `SYN-ECM-001`, `SYN-ECM-002` use independent sequences.

Current subjects are ENV-CLI, ENV-BUILD, PRJ-ROOT, PRJ-PATH, IGN-RULE, IGN-PRESET, SYN-CORE, SYN-GO, SYN-ECM, IDX-GEN, IDX-STATE, IDX-QUERY, IDX-STORAGE, SKL-PKG and SKL-INSTALL. Subjects represent stable responsibilities rather than one-test implementation details.

Syntax families grow inside the Syntax group. `SYN-CORE-*` owns common structural laws; `SYN-GO-*` specializes Go; `SYN-ECM-*` owns current shared JS/JSX/TS/TSX adapter laws, exercising forms only in applicable dialects. Do not duplicate CORE laws in every language. Introduce `SYN-PY-*` or another `SYN-<LANG>-*` only when that language actually exists as a supported/researched Product boundary. Python is deliberately omitted. Do not split ECMAScript into JS/JSX/TS/TSX or speculative ECM-TS subjects merely because four identities exist.

Every initial specification has Spec Version `1`. Before stable Nodex `1.0.0`, an intentional explicit specification rebaseline is possible when architecture demands it; casual renumbering is forbidden. After stable `1.0.0`, published IDs are permanent and never reused. Material change to the same law increments its Spec Version; a genuinely removed law remains `RETIRED`; a new law receives the next unused number for its exact prefix. Execution never changes IDs or Spec Version.

`ACTIVE` is specification status: a current observable Product law capable of PASS under valid prerequisites. It is distinct from execution conclusions PASS/FAIL/BLOCKED. A missing fixture can block an execution without changing the specification's ACTIVE status. This baseline contains only ACTIVE specifications.

Every numbered group has Purpose, Scope, Group Invariants, Test Index and Test Specifications. Every index row corresponds to exactly one body; ID, name, type, priority, integer Spec Version and status must agree. Bodies use the fixed Status / Spec Version / Type / Priority fields and Purpose, Preconditions, Actions, Expected Invariants, Allowed Differences, Regression Criteria and Dependencies sections. Dependency absence is `NONE`. Types are POSITIVE, NEGATIVE, SECURITY, DURABILITY, IDEMPOTENCY or BOUNDARY; priorities are CRITICAL, HIGH, MEDIUM or LOW. The vocabularies are categories, not quotas.

## Manual procedure and fixture independence

Select a law, read its dependencies, author or select source with independently expected physical facts, and verify its prerequisites before operating on a disposable copy/workspace. Record exact commands, exit status, stdout/stderr, source inventory and relevant bytes/hashes. Derive expected ranges from original bytes and structural ownership, not from dumping Nodex output as the oracle. A real repository and a small authored fixture can execute the same law; identify coverage and unexercised subcases honestly. A subset does not prove the whole specification.

Public CLI is the preferred observation path. Laws about public facade values not exposed by CLI (Raw ranges, exact malformed relationships, direct reads or controlled reread timing) can use a small, recorded Go harness in a disposable copy of the matching Nodex module. Keep its source/output in execution evidence, import only `internal/project`, `internal/ignore`, `internal/syntax`, `internal/index` or `internal/skill` facades as needed, and do not alter Product implementation or access private language packages. Running automated tests alone does not constitute manual execution of a law.

Deliberately frozen architectural boundaries, such as static composition and snapshot publication order, may require read-only source inspection as specified alongside observable controls. Record that inspection; do not claim general semantic quality from an import check. Fault/tamper fixtures and isolated home installs must not mutate the actual repository or user's installations. No external campaign is implied by creating these files.

## Execution location and filenames

Concrete executions live only under ignored `evidences/runs/<version>-<group>/`. `<version>` is the Nodex source Product version from the current version constant/CLI, not a development commit or build timestamp. `<group>` is exactly one of `environment`, `project`, `ignore`, `syntax`, `index` or `skills`. Changing development commit while Product version is unchanged does not create a new version/group directory. Every concrete execution still records the exact Nodex commit exercised, even when binary metadata is `unknown`.

The tracked repository `.gitignore` rule covers `evidences/runs/` in every clone. Do not create this directory or placeholders until the first manual execution.

Each concrete execution creates `<TEST-ID>-<YYYYMMDDHHMMSS>.md` using `America/Tegucigalpa` time. For example, `SYN-GO-003-20261003143022.md` or `IDX-GEN-002-20261003143310.md`. A rerun creates a new file; never overwrite an earlier record. If the timestamp collides, wait for another second. Execution records are immutable once written. Corrections require a separately timestamped amendment referencing the original evidence file; do not edit historical evidence.

## Required execution record

Each execution contains these fields/sections (the Expected Invariants are referenced and compared, never copied as a normative body):

```text
Test ID
Spec Version
Canonical Specification Path
Product Version
Group
Exact Product Commit
Execution Timestamp
Timezone

Verified Preconditions
Actual Execution
Observed Result
Comparison With Expected Invariants
Differences
Conclusion: PASS | FAIL | BLOCKED
```

Canonical Specification Path identifies the owning tracked group file and Test ID heading. Record tools/environment and exact fixture bytes or recoverable identities needed to evaluate the observations. If an external repository is involved, add an execution-only `External Fixture` section containing Repository URL, exact revision, local source root, starting source status and relevant source inventory. This section is unnecessary for only local authored fixtures. Public URLs and public commit SHAs are allowed; they do not become specification content.

PASS means the observed execution proves the expected invariants. FAIL means an expected invariant is violated. BLOCKED means a legitimate prerequisite external to the operation under test is unavailable. BLOCKED must not disguise a Product failure, an unimplemented Product operation required by the same law or a test-authoring gap. Document the missing prerequisite and stop claiming coverage it prevented.

## Cumulative group state

Each `evidences/runs/<version>-<group>/` may contain `RUN.md`. It is mutable cumulative execution state, while individual execution records remain immutable. It records at minimum Product Version, Group, overall status, first and latest execution timestamps, Product commit(s) exercised, latest result per Test ID, latest evidence-file path and whether group execution is complete or in progress.

Its index includes Test ID and Spec Version as well as latest result/evidence path. It points to canonical laws and evidence; it must not duplicate normative bodies. Completion requires coverage of all applicable ACTIVE group laws and their required subcases; unavailable prerequisites remain visible rather than being counted as passes.

## Sensitive evidence

These workflows generally require no secrets. Never record access tokens, passwords, SSH private keys, cookies, private repository credentials or authorization headers accidentally used in repository acquisition or surrounding tools. Use `<REDACTED>` or safe symbolic facts. Redact before writing immutable evidence, including captured command arguments/output. Public repository URLs and public commit SHAs are safe fixture identities.
