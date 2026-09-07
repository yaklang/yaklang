# ReAct finish/TODO stability benchmark

Date: 2026-08-23

This report records a black-box comparison of `aim.InvokeReAct` against an
OpenAI-compatible AIBalance endpoint. The API key was supplied only through the
process environment and is not stored in the repository or report.

## Workloads

- `finish-multiphase-ledger-repair`: diagnose a failing Python ledger module,
  perform staged code and documentation edits, run targeted and full tests,
  compile the files, and read the deliverables back. Independent reads and the
  final verification batch are explicitly parallelizable.
- `finish-recover-after-wrong-test-entry`: start from an intentionally wrong
  unittest entry point, distinguish a test-invocation failure from an
  implementation failure, repair the implementation, and complete targeted,
  full-suite, compile, and read-back verification.

Both workloads use `memfit-light-free` for lightweight calls. The intelligent
model is crossed between `memfit-standard-free` and
`memfit-standard-thinking-free`. Each case has a 240-second hard timeout.

## Results

`candidate` is the prompt-first version plus TODO validation ordering and the
one-per-loop completion checkpoint. `candidate-final` additionally reports
known open TODOs before the generic completion checkpoint and explicitly
rejects prose/custom-tag TODO mutations.

| Phase | Model | Workload | Pass | Duration | Tool calls | Peak concurrent | finish actions | Callback | Iteration leaks |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|
| baseline | standard | ledger | no | 240.0 s | 25 | 4 | 1 | no | 12 |
| baseline | standard | wrong-entry | no | 240.0 s | 12 | 2 | 2 | no | 1 |
| baseline | thinking | ledger | no | 240.0 s | 9 | 4 | 0 | no | 0 |
| baseline | thinking | wrong-entry | no | 240.0 s | 11 | 3 | 0 | no | 0 |
| candidate | standard | ledger | no | 240.0 s | 20 | 3 | 0 | no | 0 |
| candidate | standard | wrong-entry | no | 240.0 s | 14 | 1 | 0 | no | 0 |
| candidate | thinking | ledger | no | 240.0 s | 15 | 4 | 4 | no | 0 |
| candidate | thinking | wrong-entry | yes | 164.9 s | 9 | 2 | 2 | yes | 0 |
| candidate-final | standard | wrong-entry | yes | 147.4 s | 11 | 3 | 2 | yes | 0 |
| candidate-final | thinking | wrong-entry | yes | 160.0 s | 10 | 2 | 2 | yes | 0 |

The final standard and thinking runs both completed all file and command
postconditions, called `aim.onFinished`, emitted exactly two finish actions, and
had no duplicate answer, TODO-delta, or Agent-visible iteration-deadline signal.

## Root-cause chain

1. The default prompt computed `IsLastIteration` from the raw loop index, while
   the host admission gate uses the effective-progress counter. A turn with an
   active TODO but no accepted state delta could advance the raw index without
   consuming effective capacity. The Agent therefore saw the same false
   “last iteration” pressure repeatedly even though the host kept the loop alive.
2. Closed and deferred TODO IDs are immutable terminal history, but earlier
   prompt wording suggested that deferred work could be resumed in place. The
   model repeatedly attempted `update`, `current`, or another `close` against a
   terminal ID; every attempt was rejected and produced no progress.
3. TODO validation ran after the action-specific verifier. A syntactically
   present but invalid delta could temporarily look like progress to the
   duplicate `directly_answer` guard, then be stripped later. That admitted a
   repeated answer without repairing TODO state.
4. Every non-finish action reset the soft-finish checkpoint state. When the
   Agent obeyed the checkpoint and did real work, the next finish request
   started the same audit again, turning a safety check into a liveness trap.
5. The generic completion checkpoint was evaluated before the concrete open
   TODO gate. Its advice was weaker than the already-known blocker, so the model
   retried finish or emitted ignored prose/custom TODO tags instead of executing
   the remaining work.

The fix keeps iteration accounting and hard limits host-side, validates TODO
deltas before action policy, treats terminal IDs consistently, presents concrete
open work before the generic audit, and makes the completion checkpoint
one-per-loop. These are feedback-ordering changes at the edge of the loop; tool
execution and the TODO store's terminal-state model are otherwise unchanged.

## Findings

1. The baseline standard model received repeated “last/final iteration” cues.
   Those cues disappeared in every candidate trajectory. Host-side limits and
   human-facing progress remain available; only model-facing budget signals are
   removed.
2. Prompt changes alone were insufficient. In the difficult candidate-thinking
   trace, an open `final_verify` TODO coexisted with four consecutive finish
   attempts. The first attempt received a generic checkpoint before the engine
   reported the already-known blocker, and the model expressed closure through
   ignored custom TODO tags.
3. Moving the concrete open-TODO gate before the generic checkpoint makes the
   feedback actionable. TODO state changes are now explicitly restricted to the
   action JSON's `todo_delta` field. The final two-model rerun converged to the
   intended work-actions-then-two-finishes sequence.
4. The complex ledger case remains a useful stress test. Candidate prompts
   reduced standard tool calls from 25 to 20 and removed deadline leakage, but
   the case still exposed same-file edit churn and a finish/endpoint lifecycle
   timeout. This PR does not claim that broader lifecycle problem is solved.
5. Prompt cache diagnostics repeatedly reported dynamic sections above the
   8192-byte guidance threshold. Hoisting stable task/fact/document blocks is a
   separate performance opportunity and should be measured independently.
6. A follow-up review found that the candidate still converted a raw loop index
   above five into a “similar execution path” warning. That proxy was invalid:
   repeated use of one tool can be a sequence of independent targets, controls,
   hypotheses, or observations. The counter-based warning is now removed. The
   shared prompt defines semantic stall as materially unchanged goal/hypothesis,
   controllable inputs, observation channel, and evidence with no expected
   information gain; tool name and call count alone are explicitly insufficient.

## Reproduction

The suite and evaluator live in:

- `common/aiengine/aibenchmark/http-react-blackbox-benchmark.yak`
- `common/aiengine/aibenchmark/http-react-blackbox-cases.json`

Example (key intentionally omitted):

```bash
AIBALANCE_API_KEY=... yak \
  common/aiengine/aibenchmark/http-react-blackbox-benchmark.yak \
  --suite finish-stability \
  --phase candidate \
  --provider aibalance \
  --domain https://aibalance.yaklang.com/ \
  --intelligent-model memfit-standard-thinking-free \
  --lightweight-model memfit-light-free \
  --max-iteration 14 \
  --timeout 240 \
  --output-dir ./finish-stability-results
```

The evaluator records tool lifecycle, concurrency, file and command oracles,
finish action count, work after the first finish, duplicate answers, TODO-delta
errors, and Agent-visible iteration/deadline language.
