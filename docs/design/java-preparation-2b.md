# Java source preparation — 2B acceptance ledger

Last verified: 2026-10-22T22:14:37+08:00. Source worktrees: `fix-ssa-unit-plan-perf` (yaklang) and `/home/wlz/Developer/work/javajive-coremail-fix`.

## Acceptance and current result

**NOT ACCEPTED:** coremail baseline finished with exit 1; ekp has not been run. Do not substitute a selected-file diagnosis for either full-project run.

* Both input trees are immutable. Retain every jar/war/class/JSP input; Java must pass `java2ssa.FrontendRaw` without the compatibility normalizer.
* Distinguish materialization/read errors, decompiler refusal comments, true Java syntax errors, descriptor units, ownership suppression and method stubs. A syntax-valid stub is NOT reconstructed behavior.
* Source-ownership suppression requires a proved owning source unit and consistent emitted output. No generic “unexpressible” marker is admitted: absence of a known strategy is not an impossibility proof.
* Per project: fresh run/YAKIT_HOME/TMPDIR under `/mnt/data/scan-outputs/`; GOGC=100, GOMEMLIMIT=12GiB, existing run parallelism=16. Six profiles (CPU/heap/allocs/goroutine/mutex/block), debug2 goroutines, exit, phases, peak RSS and minimum available memory.
* 20 minutes means end-to-end preparation plus Java verification for one whole project. Also report process wall time including profile serialization. Never infer phase wall time by summing overlapping worker durations.

## Baseline

`/mnt/data/scan-outputs/coremail-java-recheck-20261022-2008/verification.json`

| Metric | Measured |
|---|---:|
| files / checked | 41765 / 41765 |
| materialization errors | 0 |
| generic failures | 249 |
| failure-comment units | 206 |
| empty package-info units | 43 |
| true FrontendRaw syntax errors | 0 among submitted units; failure-comment units were rejected before parsing |
| stub methods / files | 1634 / 1285 |
| ownership-suppressed units | 2842 (existing markers, not newly proved in this iteration) |
| prepare / verify | 1539.7s / 758.2s |
| process wall | 2695.2s |
| peak RSS | 12151MiB (sampling) |
| final heap inuse | 1195442176 bytes |

Binary SHA256: `d2beb87f1558c752a4da93db75ca115e59a5e3c9c71966f476d3c3c4add172ce`. The old preparation metric combines archive reads, decompilation, JSP translation, source storage and Java validation. These subphases were NOT individually timed, so no invented split is supplied. Roughly 397s between preparation+verification and total wall includes GC/report/profiles; attribution awaits profiles.

## Issue register (register BEFORE new RED test/fix)

| ID | First recorded | Symptom / evidence | Root cause / certainty | Status / regression / commit |
|---|---|---|---|---|
| J2B-001 | 2026-10-22T21:02+08:00 | 43 empty package-info units; CXF XmlSchema lost | Confirmed early descriptor return omitted annotation attributes | Prior RED/GREEN, unpublished; repository fixture and annotation-identity roundtrip to strengthen |
| J2B-002 | 2026-10-22T21:12+08:00 | wmsvr a/l static owner a.k refused | Confirmed package/type collision; keep same-package simple type and foreign type qualified | Prior RED/GREEN + javac; import order and runtime equivalence to strengthen before commit |
| J2B-003 | 2026-10-22T22:14:37+08:00 | 206 refusal comments have no reason in verification.json | Confirmed verifier discards decompiler reason; actual failure families unknown until replay | OPEN: full path/hash/reason inventory first; no suppression allowed |
| J2B-004 | 2026-10-22T22:14:37+08:00 | constructor prefix stubs 851+181 WARN lines | Proof failures observed, not yet determined to be decompiler bug vs missing evidence | OPEN: original bytecode/effect-order fixture required; WARN counts != unique final stub counts |
| J2B-005 | 2026-10-22T22:14:37+08:00 | inherited member-name proof 630 WARN lines | Dependency namespace partially fixed; remaining missing parents not inventoried | OPEN: enumerate missing owners before changing proof |
| J2B-006 | 2026-10-22T22:14:37+08:00 | c:forEach missing items (91) / var (3) logged | Translator currently rejects numeric/anonymous iteration; specification and concrete pages to verify | OPEN: RED fixtures must preserve loop body and bounds, not swallow tags |
| J2B-007 | 2026-10-22T22:14:37+08:00 | 38.3m preparation+verification, 44.9m wall | No separate phase timer; possible repeated Java parsing; profile output cost observed | OPEN: instrument phases and inspect profiles before optimization |
| J2B-008 | 2026-10-22T22:14:37+08:00 | incorrect assertion that @Deprecated() is illegal and package annotations cannot see imports | Confirmed incorrect by JLS §§7.5,9.7 | OPEN: correct comments/tests without losing annotation preservation |
| J2B-009 | 2026-10-22T22:14:37+08:00 | local replace and uncommitted javajive fixes | Confirmed git status | UPDATED 22:28:12+08:00 by user: keep filesystem replace UNCOMMITTED; finish logical local javajive commits and clean tree; NO PUSH or remote pseudo-version change until separate authorization |
| J2B-010 | 2026-10-22T22:14:37+08:00 | enum constant-specific enclosing identity refused | No valid impossibility proof; must inspect stripped attributes and ownership before changing gate | OPEN: no suppression or fabricated bodies |
| J2B-014 | 2026-10-22T22:50:00+08:00 | `not-package-info.java` accepted as descriptor | Confirmed suffix check matches non-descriptor filename; RED negative fixture reports only 3 errors instead of 4 | OPEN: require exact basename; preserve descriptor role in cache key |
| J2B-013 | 2026-10-22T22:43:33+08:00 | older enum test requires mandated MethodParameters absent from javac17 fixture | JDK17 gate fails fixture precondition; CI pins JDK21 | JDK21 toolchain downloaded and checksum verified; re-gate pending |
| J2B-012 | 2026-10-22T22:40:53+08:00 | verification resets per file and reparses identical generated bytes from duplicate archives | Potential repeated work; sharing syntax results is safe only for identical bytes AND the same descriptor-path role; no AST retention | OPEN: injected verifier call-count RED test, duplicate failure propagation and package-info-vs-ordinary negative fixture required |
| J2B-011 | 2026-10-22T22:14:37+08:00 (J2B-003 inventory refinement) | 117 backing factory/initializer refusals; representative CXF values() is flags 0x0019 | Confirmed exact-0x0009 check rejects older public static final compiler form; remaining members of bucket need replay | RED enum-final-red.log → GREEN enum-final-green.log; accept ONLY 0x0009/0x0019, retain original producer/CFG proof; integration fixture pending |
| J2B-020 | 2026-10-22T23:40+08:00 | 43 hidden enum name/ordinal refusals; representative `cN` stores field `a` but passes `"OK"` to `Enum.<init>`, and the values array is private synthetic `k` rather than `$VALUES` | Confirmed by javap and by a synthetic class-file rename. JLS 8.9.1 / `Enum` constructor use one identifier for the field and the name string; JLS 8.1.4 forbids `extends Enum`. A static alias is legal (JLS 8.9.2). The values-array spelling is not source-level once the clone protocol and constant packet match | GREEN `TestHiddenEnumNameKeepsAliasAndName` (name, ordinal, payload, alias, valueOf, clone). Keyword and duplicate strings still refused. Replay of these 43 paths: 40 emitted, 3 still `constant-specific enclosing identity`. Not project acceptance |
| J2B-021 | 2026-10-23T00:20+08:00 | 3 constant-specific enclosing-identity refusals (`es`, `mt/dq`, `ez`). Bodies are final ACC_ENUM classes such as `et` extending the enum, with no InnerClasses or EnclosingMethod. Each `<init>(String,int)` passes null to the enum's synthetic `<init>(String,int,et)` | Javac 8 uses that bridge and names the first body as the marker. The obfuscator renamed `Owner$N` and deleted the enclosing rows. Source can still express the body as a constant block. A contradictory enclosing row is not accepted by this certificate | GREEN `TestRenamedEnumConstantBodyFoldsIntoConstant`: renamed fixture recompiles and keeps comparisons, names, ordinals, valueOf and values clone. `TestRenamedEnumConstantBodyRejectsNonEnumSubclass` still refuses. Replay of the 3 classes emitted, stubs=0. Not project acceptance |

### Targeted replay inventory (not acceptance)

249 original failure paths replayed using the current local source. 45 emitted (43 descriptors + 2 formerly refused classes), 204 still refused. Mutually exclusive refusal buckets: 117 backing factory/initializer, 43 hidden enum name/ordinal, 15 static binding, 3 enum enclosing identity, 23 enclosing method scope, 2 enum constructor origin, 1 backing declaration. No `FrontendRaw` acceptance is inferred from `emitted`.

Evidence: `/mnt/data/scan-outputs/java-2b-20261022-2207/inventory/inventory.jsonl`; contains every path, actual reason, output SHA256 and elapsed time; `sources/` retains the exact diagnosed output. The earlier progress message's “124/overlap” and “14 enclosing” figures were incorrect: the JSON bucket counts above are authoritative, not an estimate.

## Sources verified

Verified online at 2026-10-22T22:14:37+08:00:

* [Oracle JLS 7.5](https://docs.oracle.com/javase/specs/jls/se17/html/jls-7.html#jls-7.5): import scope includes annotations on the package declaration; textual ordering does not prevent that lookup.
* [Oracle JLS 9.7.1–9.7.2](https://docs.oracle.com/javase/specs/jls/se17/html/jls-9.html#jls-9.7.2): marker `@T` is shorthand for legal normal `@T()`.
* [Go module pseudo-versions](https://go.dev/ref/mod#pseudo-versions), [replace](https://go.dev/ref/mod#go-mod-file-replace): resolve a pushed commit using Go tooling, not a hand-made timestamp. Fork-only publication may need a remote path replacement; a filesystem replacement is not acceptable final evidence.

## Evidence paths

2B diagnostic/test evidence root: `/mnt/data/scan-outputs/java-2b-20261022-2207/`. The original run remains unchanged. Publish only synthetic fixtures, not proprietary application classes, in javajive tests.
