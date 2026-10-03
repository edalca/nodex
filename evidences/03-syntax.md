# Syntax

## Purpose

Collect and localize structural source facts without semantic analysis.

## Scope

SYN-CORE-* owns the language-neutral structural contract; SYN-GO-* adds Go-specific extraction/association/exclusion behavior; SYN-ECM-* adds behavior shared by the current JS/JSX/TS/TSX adapter, with applicable dialect controls.

Known limitation outside ACTIVE normative invariants: the current TS/TSX combination `class C { static "quoted-name"() {} }` can expose the interior name without quotes. SYN-ECM-006 excludes static quoted-method combinations in TS/TSX from its raw-spelling guarantee. This observation is not an allowed regression for the covered forms, a repair request or a universal name-correctness claim.

## Group Invariants

Facts use exact physical ranges and direct parser relationships. Empty Docs is structural absence. SYN-CORE laws are not repeated per language. New SYN-PY-* or SYN-<LANG>-* subjects are introduced only when the language actually exists as a supported/researched Product boundary and adds/specializes behavior beyond CORE. No Python tests exist here. The current ECMAScript adapter stays SYN-ECM, without speculative JS/JSX/TS/TSX or ECM-TS subfamilies.

All fixtures and observations follow [the manual-validation reference law](README.md). Nodex collects, indexes, localizes and retrieves; human/LLM analyzes.

## Test Index

| ID | Name | Type | Priority | Spec | Status |
| --- | --- | --- | --- | --- | --- |
| SYN-CORE-001 | Physical comment facts and coordinates | BOUNDARY | CRITICAL | 1 | ACTIVE |
| SYN-CORE-002 | Structural declarations and direct Docs vocabulary | BOUNDARY | CRITICAL | 1 | ACTIVE |
| SYN-CORE-003 | Exact-range bounded comment context | BOUNDARY | HIGH | 1 | ACTIVE |
| SYN-CORE-004 | Declaration-anchored original context | BOUNDARY | HIGH | 1 | ACTIVE |
| SYN-CORE-005 | Static complete language composition | BOUNDARY | HIGH | 1 | ACTIVE |
| SYN-GO-001 | Go recognition and parse boundary | BOUNDARY | HIGH | 1 | ACTIVE |
| SYN-GO-002 | AST comment groups and directive-preserving normalization | POSITIVE | HIGH | 1 | ACTIVE |
| SYN-GO-003 | Go structural declaration inventory | POSITIVE | HIGH | 1 | ACTIVE |
| SYN-GO-004 | Only AST-own direct documentation | BOUNDARY | CRITICAL | 1 | ACTIVE |
| SYN-GO-005 | Optional Go language preset meanings | BOUNDARY | HIGH | 1 | ACTIVE |
| SYN-ECM-001 | Configured ECMAScript dialect boundary | BOUNDARY | HIGH | 1 | ACTIVE |
| SYN-ECM-002 | Parser comments and lexical traps | BOUNDARY | CRITICAL | 1 | ACTIVE |
| SYN-ECM-003 | Ordered structural leading JSDoc | BOUNDARY | CRITICAL | 1 | ACTIVE |
| SYN-ECM-004 | Decorator and independent nested/signature slots | BOUNDARY | HIGH | 1 | ACTIVE |
| SYN-ECM-005 | Neutral ECMAScript declaration forms and extents | POSITIVE | HIGH | 1 | ACTIVE |
| SYN-ECM-006 | Bounded raw structural names | BOUNDARY | HIGH | 1 | ACTIVE |
| SYN-ECM-007 | True versus static constructor classification | BOUNDARY | HIGH | 1 | ACTIVE |
| SYN-ECM-008 | Member-owned terminators through bounded trivia | BOUNDARY | HIGH | 1 | ACTIVE |
| SYN-ECM-009 | ECMAScript line terminators in attachment | BOUNDARY | HIGH | 1 | ACTIVE |
| SYN-ECM-010 | Conservative unsafe-recovery facts | BOUNDARY | CRITICAL | 1 | ACTIVE |

## Test Specifications

## SYN-CORE-001 — Physical comment facts and coordinates

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
CRITICAL

### Purpose

Localize language-defined comment units by source bytes rather than text identity.

### Preconditions

Authored source for each configured language with repeated text, directives, UTF-8, LF and CRLF; a disposable syntax facade harness exposing Document values.

### Actions

Parse caller-supplied bytes, including a logical path that does not exist. Independently compute positions from bytes and compare Raw to source[Start.Offset:End.Offset]. Inspect normalized directive text and repeated physical units.

### Expected Invariants

Comments are independent language-defined physical units in source order. Raw equals the original half-open byte slice; normalized Text preserves directive content. Offsets are zero-based bytes; lines and byte columns are one-based; only LF advances physical lines. Source directives do not relocate positions. Equal text at different ranges remains independent. Successful facts come from the supplied bytes without filesystem reads.

### Allowed Differences

Unit grouping and normalization details specialize by language; this does not require one fact per lexical token for Go. Arbitrarily broken ECMAScript input has the recovery limits in SYN-ECM-010.

### Regression Criteria

Raw or coordinates differ from source, text equality merges units, directive content is dropped, or a path is opened instead of using supplied bytes.

### Dependencies

NONE

## SYN-CORE-002 — Structural declarations and direct Docs vocabulary

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
CRITICAL

### Purpose

Expose declaration facts and direct relationships without judging documentation.

### Preconditions

A disposable syntax facade harness; parser-clean supported-language fixtures containing named/nameless and documented/undocumented declarations.

### Actions

Parse each fixture once and inspect both comment and declaration collections. For each Docs range find the exact same-parse comment and compare physical ordering; inspect declarations with no direct docs.

### Expected Invariants

A declaration exposes structural Kind, Names and half-open physical Range. Docs is a non-nil ordered 0..N collection of distinct direct parser-owned comment ranges from the same parse. Comments remain separate facts rather than concatenated documentation. Absence is an empty collection and does not suppress a declaration or imply a requirement to document it. Syntax assigns no snapshot IDs or quality/meaning judgments.

### Allowed Differences

Language-specific kinds, cardinality and association rules specialize this vocabulary; no universal name-correctness claim covers the recorded ECMAScript limitation.

### Regression Criteria

Docs uses text/proximity instead of direct range identity, merges comments, invents IDs or turns absence into a quality judgment.

### Dependencies

SYN-CORE-001

## SYN-CORE-003 — Exact-range bounded comment context

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Retrieve structural context as a physical original-source slice.

### Preconditions

A syntax facade harness; supported-language fixtures with identical comments, direct/contained/unattached comments, an overlong comment, long lines and large containers.

### Actions

Call Context for exact parsed ranges. Compare returned Text and Range to the original bytes. Try partial, shifted and coordinate-inconsistent ranges. Measure physical lines and bytes added outside the comment anchor.

### Expected Invariants

Range identity selects exactly one comment, never comment text. Nonmembers/malformed ranges fail without a successful snippet. A successful snippet is contiguous original bytes with matching physical coordinates, no reformatting, inserted ellipsis or syntax tree. It uses a language-selected structural container, at most 40 physical lines and 8192 extra bytes outside the comment anchor. An overlong comment retains its beginning and only fitting prefix lines; comment bytes themselves are exempt from the extra-byte budget.

### Allowed Differences

Enclosing-container choice is language-specific. A whole comment need not fit the line cap; very long anchor lines can exceed total byte length without violating the extra-byte budget.

### Regression Criteria

Text is reconstructed, the wrong identical comment is selected, bounds are exceeded outside the exempt anchor or truncation loses the anchor beginning.

### Dependencies

SYN-CORE-001

## SYN-CORE-004 — Declaration-anchored original context

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Keep a declaration identifiable when its body exceeds retrieval limits.

### Preconditions

A syntax facade harness; small and large supported declarations, including Unicode/CRLF and a first line longer than the extra-byte budget.

### Actions

Call DeclarationContext for exact declaration ranges and compare bytes. Attempt partial/shifted/nonmember ranges. For large declarations inspect start and line/extra-byte bounds.

### Expected Invariants

Only an exact declaration range selects a result. The snippet is an original-byte contiguous prefix of that declaration, excluding leading documentation outside its range. If it fits, the whole declaration is returned; otherwise its starting physical line is retained with fitting following content. Limits are 40 physical lines and 8192 bytes outside the first-line anchor. Invalid/nonmember ranges fail without context.

### Allowed Differences

Anchor bytes are exempt; a long first line may exceed the total byte budget. Language declaration extents vary according to their own laws.

### Regression Criteria

Context loses the declaration start, prepends documentation outside its range, reformats bytes or accepts an inexact identity.

### Dependencies

SYN-CORE-002

## SYN-CORE-005 — Static complete language composition

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Keep language recognition, extraction, context and preset ownership in one deliberately frozen structural boundary.

### Preconditions

Current source and architecture tests; read-only review of the syntax registry, language constructors, facade and imports; a disposable facade harness.

### Actions

Inspect internal/syntax/languages.go and complete capability contracts. Compare path recognition and catalogs through the facade. Review existing controlled duplicate/overlap/composition tests without adding or executing a future language campaign.

### Expected Invariants

One explicit static registry composes complete language capability objects for recognition, parse, both contexts and presets. The facade exposes neutral values; parser trees/dependencies stay private to their language adapter. Identities/selectors are unique and overlapping recognition is rejected rather than selecting by registration order. Unsupported paths return no partial document, regardless of contents. There is no self-registration or independent preset registry.

### Allowed Differences

Private parser mechanisms and dependency revisions are not frozen by this law. A future actual language may extend the same contract; manual source review supplements mechanical import checks.

### Regression Criteria

A capability is bypassed by facade language dispatch, an alternate registry exists, a parser object crosses the facade, or ambiguity silently selects a winner.

### Dependencies

NONE

## SYN-GO-001 — Go recognition and parse boundary

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Select Go from the exact suffix and reject malformed Go without semantic gating.

### Preconditions

Authored .go and uppercase/unsupported paths; malformed and syntactically parseable Go with unresolved imports/names, build tags and platform filenames; facade harness.

### Actions

Recognize paths independently of contents. Parse malformed source containing earlier comments; parse source with unresolved identifiers/imports and alternate build constraints.

### Expected Invariants

Only case-sensitive .go selects Go. Parse rejects malformed Go with no partial document and physical diagnostics. It does not type-check, resolve imports or symbols, or select source by build constraints/platform suffix. Those files remain ordinary supported source unless independently excluded.

### Allowed Differences

Diagnostic text can vary; parser acceptance is not a compilation or type-correctness certificate.

### Regression Criteria

Uppercase/other suffix selects Go, malformed source publishes partial facts, or semantic/build selection prevents structural extraction.

### Dependencies

SYN-CORE-005

## SYN-GO-002 — AST comment groups and directive-preserving normalization

Status:
ACTIVE

Spec Version:
1

Type:
POSITIVE

Priority:
HIGH

### Purpose

Preserve Go comment units according to parser grouping.

### Preconditions

Parser-clean Go with adjacent line/block comments, detached groups, trailing comments, directives and CRLF; facade harness.

### Actions

Compare Parse comments with a comment-preserving Go AST observation and independently authored raw spans. Inspect normalized text for //go:generate, //go:build and //line content.

### Expected Invariants

Each AST CommentGroup is one unit, including groups unrelated to declarations. Raw spans the first through last physical member; normalization strips delimiters, preserves group member order and directive content, and normalizes CRLF in block text without changing Raw. Adjacent members in one group are not separate Go facts.

### Allowed Differences

Group membership follows Go parser structure, not an ECMAScript one-block rule; prose has no quality expectation.

### Regression Criteria

A Go group splits or disappears merely for lacking documentation ownership, directives vanish or raw CRLF bytes change.

### Dependencies

SYN-CORE-001, SYN-GO-001

## SYN-GO-003 — Go structural declaration inventory

Status:
ACTIVE

Spec Version:
1

Type:
POSITIVE

Priority:
HIGH

### Purpose

Collect supported Go declaration forms without inferring semantic entities.

### Preconditions

Parser-clean Go with package, functions/methods, parenthesized and standalone const/var/type forms, struct/interface fields, embedded fields and imports; authored expected facts.

### Actions

Parse through the facade and compare each kind, source-ordered name list and exact range to its authored syntax. Include nested declarations and field-versus-parameter controls.

### Expected Invariants

Indexed forms are package, function, method, const-group, var-group, type-group, const, var, type and field. Parenthesized groups and each spec are independent declarations; unparenthesized forms produce one fact. Struct/interface fields are included; parameters and imports are not declarations. Names follow declared identifiers; unnamed embedded fields have no invented name. Ranges identify the original declaration syntax.

### Allowed Differences

Names and inventories vary with authored source. Go legality beyond parser acceptance and symbol interpretation are outside scope.

### Regression Criteria

Imports/parameters become declarations, group/spec identities collapse, names are invented or source extents cease to identify their syntax.

### Dependencies

SYN-GO-001, SYN-CORE-002

## SYN-GO-004 — Only AST-own direct documentation

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
CRITICAL

### Purpose

Keep Go documentation association owned by the exact parser node.

### Preconditions

Parser-clean Go covering all declaration forms with own Doc, no Doc, detached nearby comments, group/member docs and trailing struct/interface comments; authored AST ownership.

### Actions

Inspect each declaration Docs against the node own Doc field. Put documentation on a group alone, a member alone and both. Keep trailing Field.Comment and comments separated by blank lines.

### Expected Invariants

Go Docs is empty or exactly one range: the node own AST Doc group. Group docs are not copied to children and child docs are not copied to groups. Trailing Field.Comment is a physical comment without a direct Doc relationship. Detached nearby comments do not attach by proximity. Undocumented nodes still appear.

### Allowed Differences

A group can contain several lexical comments while being one physical unit; this does not enlarge direct Docs cardinality.

### Regression Criteria

Docs propagates between nodes, uses trailing Comment/proximity or contains more than one Go comment-group range.

### Dependencies

SYN-GO-003, SYN-CORE-002

## SYN-GO-005 — Optional Go language preset meanings

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Exclude Go source only through the selected language-owned preset meaning.

### Preconditions

Authored inventory with _test.go, vendor elements/lookalikes, genuine generated headers before package, loose generated-text controls, platform/build-tag files and other supported languages.

### Actions

Compare selection with no presets and each of go:tests, go:vendor and go:generated enabled. Inspect go:all expansion through syntax. Use malformed bodies after a valid generated package-header control.

### Expected Invariants

go:tests matches Go basenames ending _test.go; go:vendor matches Go files beneath a vendor path element; go:generated follows the structural Go generated convention (a matching // Code generated ... DO NOT EDIT. line in leading comments before package), not a loose text search. Generated classification can stop at package and need not parse the body. go:all expands to those three concrete IDs. These meanings do not exclude other languages; platform/build-tag source remains ordinary without an applicable exclusion.

### Allowed Differences

No preset is enabled by Syntax itself. Generic persistence/edit rules stay with Ignore. Unparseable leading structure need not classify as generated.

### Regression Criteria

Loose text or path lookalikes trigger a preset, other-language files are excluded by Go presets, or platform/build constraints become implicit policy.

### Dependencies

SYN-GO-001

## SYN-ECM-001 — Configured ECMAScript dialect boundary

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Recognize the four configured identities without content guessing or implicit presets.

### Preconditions

A facade harness; extension matrix and parser-clean dialect-appropriate fixtures; binary grammar-override environment control.

### Actions

Recognize .js/.jsx/.ts/.tsx and uppercase/alternate extensions. Parse examples under each identity, inspect its catalog and repeat in a fresh process with GOTREESITTER_GRAMMARGEN_BLOB_DIR pointing to an empty directory.

### Expected Invariants

Exactly .js, .jsx, .ts and .tsx select javascript, jsx, typescript and tsx respectively, case-sensitively. JS/JSX use one fixed embedded JavaScript grammar; TS and TSX use distinct fixed embedded grammars. Contents/runtime filesystem blobs do not choose grammar. This family provides no presets or implicit node_modules/dist/generated exclusions.

### Allowed Differences

Grammar internals/pins may evolve deliberately; success never certifies dialect validity. Generic manual exclusions remain possible.

### Regression Criteria

An alternate extension/content guess is accepted, runtime blob override changes capability or an ECMAScript-specific implicit exclusion appears.

### Dependencies

SYN-CORE-005

## SYN-ECM-002 — Parser comments and lexical traps

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
CRITICAL

### Purpose

Collect physical ECMAScript comments rather than delimiter-like text.

### Preconditions

Parser-clean dialect-appropriate sources with line/block/JSDoc/hashbang units, directives, repeated blocks, strings, regexes, template raw/expression text and JSX text/attributes/expressions.

### Actions

Parse all applicable dialects; compare authored physical units and raw spans. Contrast delimiter-like string/regex/template/JSX text with genuine comments in expression containers; inspect normalized block interiors and tags.

### Expected Invariants

Each parser-recognized comment/hashbang is an independent physical unit; distinct identical blocks stay separate. Comment-like text in lexical literal/JSX raw slots creates no fake fact; genuine recognized expression comments remain. Normalization removes delimiters and one ASCII space after line/hashbang delimiters, preserves block interiors/stars/tags/directives and converts block CRLF only in Text.

### Allowed Differences

Arbitrarily broken input may hide comments under SYN-ECM-010; this law uses clean authored lexical controls and does not infer comment meaning.

### Regression Criteria

Literal text becomes a comment, independent blocks merge or normalized text interprets/removes directives or tags.

### Dependencies

SYN-CORE-001, SYN-ECM-001

## SYN-ECM-003 — Ordered structural leading JSDoc

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
CRITICAL

### Purpose

Associate eligible blocks within one bounded leading-trivia slot.

### Preconditions

Clean authored ECMAScript with source-initial trivia/BOM, export/default/ambient/modifier carriers, several JSDoc blocks, ordinary intervening comments, blank lines, /**/, /***/ and syntax barriers.

### Actions

Parse and inspect direct ranges for each declaration. Put eligible blocks before carriers, between ordinary trivia and behind unrelated syntax. Contrast trailing multiline blocks with an outside-comment line break before a block.

### Expected Invariants

All eligible /** blocks except exactly /**/ in the declaration leading interval are separate ordered Docs. Only whitespace and parser-recognized comments may intervene. The preceding syntax slot bounds the interval; after syntax a line break outside comments must precede a block for it to lead. Blank lines/ordinary comments do not break an otherwise eligible run. Carriers/modifiers start the declaration extent; docs precede them. There is no nearest-block rule or tag interpretation.

### Allowed Differences

Dialect-appropriate carriers differ. Line-terminator specialization is owned by SYN-ECM-009.

### Regression Criteria

Eligible multiple blocks collapse, /**/ attaches, syntax barriers are crossed or a break inside a trailing block establishes leading status.

### Dependencies

SYN-CORE-002, SYN-ECM-002

## SYN-ECM-004 — Decorator and independent nested/signature slots

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Keep each declaration documentation slot independent through wrappers and nesting.

### Preconditions

Clean decorated class/method/field examples in applicable dialects; nested declarations; TS/TSX two overload signatures plus implementation, each with independently authored documentation.

### Actions

Place a block before the first decorator and another after decorators. Compare outer/nested Docs. Parse signatures with distinct leading A/B blocks and an undocumented implementation.

### Expected Invariants

Before-decorator eligible docs can attach; after-decorator blocks remain physical without directly documenting that declaration. Decorator tokens belong to the declaration extent. Nested declarations and overload signatures retain independent slots; container/signature docs never propagate or merge by name. The implementation has no docs unless its own slot supplies them.

### Allowed Differences

Overload forms apply to TS/TSX; the shared family law does not require unsupported syntax in JS/JSX or judge decorator legality.

### Regression Criteria

After-decorator comments attach to that owner, docs propagate to nested/implementation nodes or same-name signatures collapse.

### Dependencies

SYN-ECM-003

## SYN-ECM-005 — Neutral ECMAScript declaration forms and extents

Status:
ACTIVE

Spec Version:
1

Type:
POSITIVE

Priority:
HIGH

### Purpose

Report syntactic declaration families with their original physical extents.

### Preconditions

Authored clean dialect-appropriate inventory spanning functions/generators, classes/members, variables, objects, anonymous default exports and TS-family interfaces/signatures, aliases, enums, namespaces/modules and ambient forms.

### Actions

Compare parsed facts to independently authored fragments/kinds. Include export/default/declare/modifiers and trailing comment extras; inspect nested declarations and declaration contexts.

### Expected Invariants

Kinds are function, class, method, constructor, property, accessor, const, let, var, interface, type-alias, enum, enum-member and namespace where the dialect recognizes those forms. Generators/async/overloads are function, getters/setters accessor, modules with bodies namespace; kinds do not classify runtime values. Extents include declaration-owned wrappers/modifiers and exclude leading docs and trailing unrelated comment extras. No parser node type is exposed as a public kind.

### Allowed Differences

Only applicable syntax families are exercised per dialect. Constructor classification and member terminator specialization have their own canonical owners.

### Regression Criteria

Kinds become semantic/runtime classifications or raw CST names, wrappers vanish from extents or unrelated trailing comments enlarge them.

### Dependencies

SYN-CORE-002, SYN-ECM-001

## SYN-ECM-006 — Bounded raw structural names

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Preserve guaranteed structural spellings without interpreting bindings as symbols.

### Preconditions

Clean authored identifiers/escaped/private/numeric and non-static quoted properties; destructuring with renamed/computed keys, defaults, rest and types; computed and anonymous declarations. Exclude TS/TSX static quoted-method combinations from normative checks.

### Actions

Compare names to authored source spelling and binding order. Separate renamed keys/default initializers/type annotations from binding children. Inspect computed properties (including ["literal"]) and anonymous default exports.

### Expected Invariants

For these covered forms, names retain raw identifier/escape/private/numeric/non-static literal-quote spelling. Destructuring follows binding children in source order, excluding keys, initializers and type annotations. Computed property names and anonymous declarations have empty names rather than evaluated or invented identifiers. Multiple declarators keep ordered names within one variable declaration.

### Allowed Differences

The TS/TSX static quoted-method combination is outside this ACTIVE guarantee; the group Scope records the current limitation. No decoding, normalization, symbol resolution or universal name correctness is promised.

### Regression Criteria

A covered name changes source spelling, binding names include initializer/key/type identifiers, computed names are evaluated or anonymous names are invented.

### Dependencies

SYN-ECM-005

## SYN-ECM-007 — True versus static constructor classification

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Distinguish constructor syntax from a static method with that spelling.

### Preconditions

Clean class member fixtures in all four dialects with constructor() {}, static constructor() {}, decorated static constructor, static get/set constructor and computed [constructor] controls.

### Actions

Inspect kind/name facts and direct docs for each independently authored member.

### Expected Invariants

A true non-static class constructor is constructor with no invented binding name. static constructor is method with name constructor, including when decorated. Static getters/setters remain accessor; computed constructor expressions do not become true constructors. Physical ranges and direct leading docs remain attached to the actual slot.

### Allowed Differences

No runtime constructor or class-validity interpretation is inferred; the known static quoted-name combination is not part of this classification control.

### Regression Criteria

Static constructor is classified as constructor or loses its literal name, or accessors/computed names become true constructors.

### Dependencies

SYN-ECM-005, SYN-ECM-004

## SYN-ECM-008 — Member-owned terminators through bounded trivia

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Keep owned member delimiters in the original declaration extent without crossing slots.

### Preconditions

Clean class fields and TS/TSX property/method/abstract signatures, plus object pairs and enum members; touching/space/newline/comment/absent delimiter variants and following-member controls.

### Actions

Place ; or an applicable typed-member comma after whitespace and recognized comments. Compare declaration ranges and contexts to authored fragments. Remove the delimiter or insert another member before it. Inspect object-pair/enum commas.

### Expected Invariants

An owned field/signature delimiter remains in the range through intervening whitespace/comments within the same member slot. Other syntax, a following member or body boundary stops ownership. Without the owned delimiter, trailing trivia is not appended. Comments stay independent and do not acquire Docs merely by lying inside a range. Object-pair and enum-member separator commas remain excluded.

### Allowed Differences

Only current field/property/method-signature families receive this extent specialization; punctuation is not synthesized.

### Regression Criteria

Trivia drops an owned terminator, an absent terminator is invented, another slot is absorbed or pair/enum separators enter member extents.

### Dependencies

SYN-ECM-005, SYN-CORE-004

## SYN-ECM-009 — ECMAScript line terminators in attachment

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
HIGH

### Purpose

Honor language line-break attachment without relocating physical coordinates.

### Preconditions

Clean fixtures in all four dialects with actual CR, LF, CRLF, U+2028 and U+2029 bytes; preceding work(); syntax and eligible blocks.

### Actions

For each separator put a break before a block, only after it, on both sides or inside a trailing block. Include multiple leading blocks and ordinary comments. Independently recompute LF-based coordinates.

### Expected Invariants

CR/LF/CRLF/LS/PS in outside-comment whitespace before an eligible block establish leading separation. A break only after a trailing block or inside comments does not make that block leading. LS/PS consume UTF-8 byte columns without advancing physical LF lines; original context preserves them.

### Allowed Differences

Physical offsets change with separator byte length; these are measured from fixture bytes rather than displayed Unicode columns.

### Regression Criteria

LS/PS fail to establish leading separation, comment interiors establish it, or physical positions switch to language/UTF-16 line conventions.

### Dependencies

SYN-ECM-003, SYN-CORE-001

## SYN-ECM-010 — Conservative unsafe-recovery facts

Status:
ACTIVE

Spec Version:
1

Type:
BOUNDARY

Priority:
CRITICAL

### Purpose

Prevent uncertain declarations and relationships from surviving detected unsafe recovery.

### Preconditions

Authored known unsafe-recovery controls: class C { broken((x) {} followed by a method JSDoc/member; and /** before-broken */ followed by const broken = ; and function later() {}. Include intact neighboring declarations and clean controls.

### Actions

Parse applicable JS/JSX and TS/TSX controls, inspect comments/declarations and then generate a local index. Compare to clean controls without treating parser acceptance as validity.

### Expected Invariants

Detected unsafe recovery suppresses every declaration and hence every Docs relationship in that file, including intact neighbors. Confidently observed physical comments may remain; the authored critical controls retain their actual blocks. No replacement declaration or reassigned Docs is guessed. Operational inability to produce source structure returns an error with no partial document.

### Allowed Differences

Arbitrary broken input may hide some physical comments, and undetected recovery is not universally certified. This is fact safety, not a syntax/type/semantic validator.

### Regression Criteria

A known unsafe control publishes a declaration/Docs, guesses an owner, or claims language validity from parser-clean structure.

### Dependencies

SYN-ECM-002, SYN-CORE-002
