# AI traffic security audit follow-up

## Contract

Class: Complex, because the findings concern local evidence persistence and
security boundaries. This is a bounded follow-up to Yaklang PR #5268.

- Objective: audit the four path-operation findings and the unchecked metadata
  marker deletion reported on the traffic spool implementation.
- Scope: the collector's local directory, atomic writes, cleanup error handling,
  focused regression tests and the traffic protocol reference.
- Out of scope: changing the wire protocol, platform authorization, quota
  semantics, shared runtime environments or the security scanner's rules.
- Ownership: the existing `feat/yaklang/ai-traffic-capture` worktree; no services
  or external test targets are needed.
- Publication: update the existing Draft PR under the task's push authorization.
  No merge, release or deployment is requested.

## Acceptance and evidence

| Requirement | Evidence |
| --- | --- |
| Remote session identities cannot select local paths | Production call-chain audit and hostile-identity regression |
| Writes stay within the configured spool directory | Basename/containment validation; traversal and absolute-path regressions |
| Cleanup failure is observable without claiming evidence loss | Explicit error handling and independent review of receipt ordering |
| Durable upload, restart and drain semantics remain intact | Existing focused `TestAITraffic` and `TestResilienceAITraffic` checks |
| Findings are reconciled with the actual fix | Independent review, PR discussion replies and the next scan results |

The local root is operator-controlled `YAKIT_HOME` configuration. The constructor
accepts no directory override. Atomic writes use only
validated direct-child names; remote identities remain hashed. This work does
not claim protection against a concurrent process with the same filesystem
permissions replacing parent directories. Focused T1 evidence is appropriate
for these local filesystem and error-handling claims.
