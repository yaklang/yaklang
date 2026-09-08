# Managed input follow-up: transport cancellation and compatibility

## Contract

- Objective: close the gaps identified while comparing the generic input PR
  with `fix/yaklang/log-analysis-file-size-limit` (5f00560bd). Keep the generic
  manifest/resolver architecture; do not merge the log-specific implementation.
- Class: Complex. This changes asynchronous command consumption, preparation
  lifetime, redelivery and legacy attachment authorization boundaries.
- Sources: existing isolated Yaklang `feat/yaklang/input-resource-workspaces`
  (PR 5015), rebased onto d43ebbe92; existing isolated Legion
  `feat/legion/input-resource-workspaces` (PR 454), a5d6ad1f.
- In scope: bounded asynchronous Bind dispatch, broker acknowledgement renewal,
  cancel/close/rebind delivery during preparation, consumer-loss cleanup,
  ordinary managed-attachment origin/redirect/UTF-8 compatibility, and relevant
  generic file-read/search edge cases found in the previous branch. The real
  final-source run additionally exposed a dropped LiteForge child deadline;
  preserve that deadline through inherited Provider callbacks. Its paired
  Legion fix atomically fails idle Professional Tasks and queues runtime cancel.
- Out of scope: restoring log task/version hardcodes, unrestricted URLs or
  tools, changing attachment quotas, new tasks, deployment, merge or release.
- Publication: update the two existing Draft PRs under the user's existing
  publication authorization. A rebased push uses the recorded exact lease.
- Environment: use only task-owned test brokers and the canonical isolated
  stack for the selected Legion/Yaklang pair. Preserve shared infrastructure,
  task databases/buckets/volumes, the previous reports and private credentials.
- Stop boundary: new unresolved semantic conflicts or external access blockers;
  record partial evidence without broadening authorization or erasing retries.

## Acceptance matrix

| ID | Claim | Evidence | Status |
| --- | --- | --- | --- |
| F-01 | Slow Bind cannot block cancel/close/other control commands | Real JetStream consumer and bounded streaming HTTP; controlled serial-consumer before failure and current-source success | pass |
| F-02 | Preparation longer than AckWait remains one delivery and one engine; bounded workers defer excess binds | Real JetStream metadata and download/driver counters, including deferred fifth Bind settlement | pass |
| F-03 | Consumer loss, cancellation and higher-epoch replacement clean the old preparation; delayed commands cannot damage the replacement | Six transport scenarios and focused manager tests pass under the race detector | pass |
| F-04 | Ordinary attachment downloads use configured origin, reject redirects and keep safe UTF-8 truncation | Focused real HTTP tests and caller propagation tests | pass |
| F-05 | Generic read/search retain valid text, original byte offsets and bounded memory | Unicode/page regression before failure and after success; existing bounded streaming tests | pass |
| F-06 | Cross-owner task creation and correctly formed input download are denied | Real isolated API sessions/node grants; persisted state unchanged | pass (September 7 closure below) |
| F-07 | Main log and same-name input workflows remain correct on final sources | Existing source-impact audit plus affected final-source runtime checks | pass (September 7 closure below) |
| F-08 | Optional LiteForge initialization obeys its child context while the parent remains alive | Both cancellation and deadline fail before the fix and pass after | pass |

## Evidence handling

The earlier issue452-input-20260904 report remains T2/partial and retains all
four setup retries against its original budget of one. This follow-up keeps
that history and does not re-label old fixtures as new evidence. Actual product
changes require new source fingerprints; every reused observation records its
source-impact boundary. The real JetStream harness must enter the production
consumer rather than invoking manager.Cancel directly. Provider-backed analysis
and deterministic transport fault tests are separate evidence surfaces.

The broker cases are now durable Linux tests in
`scannode/legion_input_consumer_integration_test.go`. The final six-case race
invocation passed. Earlier artifact-harness compilation/cleanup/acknowledgement
assertion corrections remain recorded in the evidence history. A one-line
duplicate `sync` import from the refreshed upstream `common/yakgrpc/server.go`
was removed because it prevented the relevant packages from compiling.

The run on 8c924b817 prepared the 1 GiB workspace but failed from inactivity
while optional initialization was waiting on the Provider. Its subsequent
runtime access and manual cancellation/cleanup are retained as failure evidence.
The paired Legion runtime-timeout contract owns F-09 and terminal projection;
F-07 must be rerun only after these affected source changes are frozen.

## September 7 acceptance correction

The final-source 1 GiB Run `aitr_5051d8837d0247699c38d7712e763b39`
completed and cleaned its workspace, but its report missed the seeded tail
event. Its search actions contained legal newlines between the `queries`
colon and array. The streaming JSON parser treated the leading newline as
a completed empty value, so the resolver correctly rejected an empty query.
A deterministic before/after regression covers every JSON value type after
leading LF/CRLF whitespace and the actual search Action consumer. Preserve
missing-comma recovery after a value has started. The full parser suite and
focused Action race checks pass. A separate full-package race timing failure
in the existing 3-second reader-cleanup benchmark is recorded, not hidden.

Cross-owner API denial and zero persisted Runs passed on Legion 6cf72e4a.
The real page two-file run `aitr_ee36a28a184848a599f208c264eba673` passed
on Yaklang e5150857f with both hashes, distinct paths, both report markers,
and cleanup. The large-file tail criterion remains pending the parser fix's
frozen-source runtime check; a completed status alone is not acceptance.

## September 7 functional acceptance closure

F-06 and F-07 passed. The corrected 1 GiB Run
`aitr_1dfd7fb7eced4d20a50d8bf34ba8ce29` on 30bc7fe3f read the final 424
bytes after whole-file streaming searches, persisted the expected tail code,
count and reason, and cleaned the workspace. VmHWM was 329728 KiB; maximum
prompt size was 117444 bytes. The earlier unsuccessful report remains retained.
Real owner isolation returned own 200 / foreign 404 / foreign create 422 with
zero Runs. The same-name page Run on e5150857f passed both distinct file hashes,
actual access, report markers and cleanup. The owning Legion issue452 plan
records the complete IR matrix and explicit non-log fixture boundary.

The conflict-free rebase preserves all input/parser/Provider patches; the only
range-diff change drops the already-upstream duplicate-import repair. Focused
input, real JetStream, Action, logging and MCP client race checks pass after
rebase. The full parser race timing assertion also fails on prior source
(8.44 s vs the existing 3 s limit, no goroutine leak); full non-race and focused
race checks pass. Do not claim the entire parser race package passed.

The parser-fix acceptance passed at that source. Later compatibility fixes
require their own affected runtime acceptance; CI is a separate handoff gate.
Historical evidence stays partial because its original retry budget was
exceeded and some temporary follow-up artifacts no longer exist. This does not
relabel those attempts as clean runs or claim deployed/production acceptance.

## Final compatibility review: search continuation

The legacy log Focus passes `offset` and consumes `complete`, `next_path`,
`next_offset`, `results` and `byte_offset`. The generic bridge previously
ignored the cursor, so a result-limited query could repeatedly return the
first page. A real Bind/resolver capability regression fails before this
correction and passes for both source.search and input.search afterwards.

SearchFrom validates exact-file cursors, aligns UTF-8 offsets, retains absolute
byte positions and access ranges, and returns a progressing continuation after
a result limit. The legacy bridge uses results while the generic tool keeps
matches, avoiding duplicate snippet payloads. Resumed results omit unknown
absolute line numbers. Three-page, invalid-range, directory-cursor, UTF-8,
file-transition and EOF checks pass under race alongside existing boundaries.
The affected real large-file Run is repeated on this frozen source; its final
evidence and CI state are recorded with the paired PR acceptance bundle.

## Bounded search work within a Focus action

The 5cbcba904 runtime Run `aitr_c8768a76e6ac4445a57617e2207f62a2`
failed while one search action performed multiple full-file scans: the Yak
Focus handler reached its existing 30-second deadline. Its failed terminal
state, partial access ranges and successful cleanup remain retained.

SearchPage now caps one call at 128 MiB and a four-second processing budget,
returning complete=false and a progressing cursor. Parent cancellation still
fails closed. A smaller requested max_scan_bytes is supported down to 64 KiB.
Continuation retains query overlap at byte-budget boundaries. A real capability
regression proves the former ignored budget and the corrected cross-page
match; managed-input and resolver boundary race checks pass. No timeout was
extended and no incomplete scan is labelled complete. The final Provider Run
and CI are separate handoff gates in the acceptance bundle.

## Retaining observed input evidence across Focus actions

Run `aitr_93654fdcc09b481eb7bff720b5cd4835` completed on 3942fe8ff,
with bounded searches, nonzero continuation, actual tail access and cleanup.
Its report nevertheless retracted the observed tail event. The legacy Focus
retains read metadata but exposes raw feedback for only the next action;
later actions could no longer substantiate an earlier observation. This Run
is a content-acceptance failure, not a successful analysis.

The generic managed-input capability now records successful requested reads
in the existing session evidence store. Each observation contains manifest,
workspace, logical path, file digest, read range and at most 8 KiB of original
head/tail bytes. Omitted bytes are explicit; UTF-8 boundaries and absolute
offsets remain accurate. Contents are labelled untrusted data, not instructions
or proof of whole-file analysis. No credential, download URL or host path is
recorded. Stable identities deduplicate the generic and compatibility aliases.
The existing session evidence retention budget applies; this is a bounded
working context, not a permanent archive of every byte ever read.

A non-log real Bind/resolver regression fails before the change and verifies
retention after later actions, bounded excerpts, original UTF-8 byte positions,
retry deduplication and no evidence update after a denied read. Final Provider
content acceptance and the final commit CI remain pending.


## Session Manager and capability correction (historical checkpoint)

Earlier real Runs selected a host node directly. They prove the observed input
functionality only, not Session Manager container isolation. Container acceptance
and current CI must pass before restoring merge readiness.

The paired Legion service now creates targetless attachment execution Sessions
on create, recovery and retry. Only the manager's leased-container transaction
can bind managed input; host targets remain rejected even with input capability.

Capability review: runtime containers receive enrollment and service environment
values, so arbitrary filesystem/script tools would bypass the immutable input
resolver and expose runtime material. Retain scoped file tools and no dynamic
MCP/Forge loading. This policy is per managed-input Run, not a global node limit.

Remove blanket child-Agent denial: buildSubAgentInvoker uses ConvertConfigToOptions
and inherits the exact finite tool manager and LegionResultRuntime. A regression
exercises that production constructor and verifies read_file object identity,
managed-input policy, MCP denial, and rejection of bash. Existing child strategy
rules still prevent recursive delegation. Plan execution remains disabled because
its custom planning loop includes EnhanceKnowledgeGetterEx outside the finite
file-tool manager; Docker alone does not authorize that extra input source.


## Capability policy follow-up (2026-09-08)

Complex: this changes an execution permission boundary within Yaklang. Reuse
the existing isolated worktree and paired Legion branch; no protocol, Session
Manager, filesystem authorization or global node capability changes. Replace
AI-core attachment detection with an inherited generic action policy supplied
by the node. Restore planning and task coordination while keeping tool lookup
and non-tool side effects within the existing authorization boundary. Do not
merge or release. Existing task-scoped branch publication authorization applies.

| Acceptance | Evidence | Status |
|---|---|---|
| AI core has no managed-input special case | Source review and generic policy regressions | pass |
| Policy applies to prompt and execution, including children | Production constructor and handler tests | pass |
| Planning uses scoped tools; independent knowledge loading denied | Planning loop and resolver integration | pass |
| Existing managed attachment flow remains correct | Real Bind/resolver/planning runtime integration, with a deterministic model callback | pass (T1) |

Own only task worktree files and task-specific temporary test artifacts. Reuse
existing synthetic fixtures and approved Provider destination if a real stack
is needed. Retain all earlier evidence and do not claim old T2 covers changed
policy behavior. Update the node README policy description with final behavior.

The focused four-package race group passed. Planning now uses the configured
read tool schema and checks unsuccessful tool results before recording planning
evidence. A denied path previously appeared as completed; the regression proves
that it fails and does not change the accumulated evidence. Task-level explicit
Plan/Detached choices and ordinary loops without a policy retain their behavior.

The runtime fixture uses real Bind, HTTP download, resolver, ReAct constructor,
planning loop and tool invocation. Only the tool-reason model callback is a
deterministic local fixture; no external Provider was contacted. This is T1
integration evidence, not a new product T2 claim. Prior container T2 remains
source-bound: input resolution, Session Manager and contracts are unchanged;
the log-analysis Focus explicitly disables planning independently. No live
stack or repeated 1 GiB Provider run is needed to verify this action-policy
change. Dynamic Skills/Forge/MCP loaders remain excluded until their independent
resources are authorized; this follow-up does not claim they were restored.
