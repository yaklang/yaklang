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
| F-06 | Cross-owner task creation and correctly formed input download are denied | Real isolated API sessions/node grants; persisted state unchanged | pending |
| F-07 | Main log and same-name input workflows remain correct on final sources | Existing source-impact audit plus affected final-source runtime checks | pending |
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
