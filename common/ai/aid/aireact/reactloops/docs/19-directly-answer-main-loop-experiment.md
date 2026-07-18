# DirectlyAnswer main-loop convergence experiment

## Question

Can every answer emitted while ReAct is running use the ordinary
`directly_answer` action instead of starting a second, standalone AI call with
the `# Direct Answer Decision` prompt family?

The desired lifecycle is:

1. a legacy caller requests `DirectlyAnswer` while a synchronous loop is live;
2. the request and bounded reference material are appended to Timeline as
   `stage_summary_request`;
3. the current finalizer resumes the same main loop;
4. the next ordinary decision emits `directly_answer`, including evidence and
   `next_movements` for completed TODOs;
5. `directly_answer` emits the stage summary and continues; only an explicit
   `finish` may terminate the task.

This keeps one action schema and one prompt family in the hot path. It also
preserves the existing rule that a user-visible answer is not equivalent to
task completion.

## Implementation boundary

The behavior is opt-in through `aim.directlyAnswerViaMainLoop(true)` or
`aicommon.WithDirectlyAnswerViaMainLoop(true)`. The option is inherited by
child configs.

| situation | behavior |
| --- | --- |
| synchronous task, live decision/action phase | write one Timeline request and return a typed delegation sentinel |
| normal finalization with a pending request | skip terminal callbacks and resume the same main loop |
| main-loop `directly_answer` | emit answer, close the pending request, continue execution |
| second request while one is pending | keep the standalone fallback to avoid a resume loop |
| initialization failure, hard error, max-iteration exit | keep the standalone fallback because the main loop cannot safely resume |
| asynchronous plan/blueprint handoff | do not summarize the parent; the downstream runner owns the result |

The async rule is important. An early version delegated the parent summary to
Timeline after `require_ai_blueprint`, but that parent loop had already handed
off control and could never consume the request. The final implementation skips
that premature summary instead of either delegating it or starting a standalone
answer call.

## Reproduction

Runs used the repository engine directly and disabled AI verification:

```bash
go run common/yak/cmd/yak.go \
  common/ai/aid/aicache/cachebench/run_react.yak \
  --input '进行主机体检' \
  --ai-type aibalance \
  --max-iteration 15 \
  --max-duration 1200 \
  --stall-timeout 600 \
  --fatal-error-threshold 0 \
  --disable-ai-verification \
  --directly-answer-via-main-loop \
  --output-dir <output>
```

No prompt-count cutoff was configured for the complete runs. The corrected run
was allowed to reach the full 1200-second cap and captured 416 calls.

Reports:

- verification-disabled baseline:
  `/tmp/yaklang-noverify-experiment/after-fixed/cachebench-20260718-143239.json`;
- first convergence run, containing the stale async Timeline request:
  `/tmp/yaklang-noverify-experiment/directly-main-loop/cachebench-20260718-152700.json`;
- corrected complete run:
  `/tmp/yaklang-noverify-experiment/directly-main-loop-fixed/cachebench-20260718-154820.json`;
- final-code 40-prompt lifecycle smoke test:
  `/tmp/yaklang-noverify-experiment/directly-main-loop-final-smoke/cachebench-20260718-155013.json`.

## Results

| complete-run metric | standalone baseline | corrected convergence |
| --- | ---: | ---: |
| duration limit | 1200 s | 1200 s |
| total calls | 376 | 416 |
| intelligent calls | 76 | 74 |
| intelligent prompt tokens | 3,875,533 | 4,557,643 |
| intelligent upstream token hit | 55.74% | 55.06% |
| intelligent client LCP hit | 26.85% | 24.00% |
| all-tier upstream token hit | 30.30% | 30.19% |
| all-tier client LCP hit | 33.59% | 29.10% |
| high-static distinct hashes | 2 | 2 |
| timeline-open distinct hashes | 83 | 83 |
| dynamic distinct hashes | 291 | 323 |
| standalone `# Direct Answer Decision` prompts | 1 | 1 |
| delegated stage-summary requests | 0 | 0 |

The single remaining standalone answer in the corrected complete run was the
outer asynchronous blueprint handoff. That executable included the synchronous
delegation guard but predated the final “skip async post-summary” fix. A smoke
test compiled from the final code captured 41 calls and contained zero
standalone Direct Answer Decision prompts and zero stale stage-summary requests.

The first convergence run reached 213 calls before its 180-second stall guard
fired. It measured 57.37% intelligent upstream hit, but the sample is not a
valid improvement claim: it stopped at a different task depth and contained an
unconsumable stage-summary request created by the async-parent bug.

## Interpretation

The control-flow convergence is valid, but this host-health task does not
exercise it often. Its user-visible direct answers were already ordinary
main-loop `directly_answer` actions. Only one call used the standalone prompt,
and that call belonged to an async handoff that should not have summarized at
all. Eliminating one call cannot materially change a 4.6-million-token
intelligent sample, and the observed 55.74% to 55.06% difference is run-path
noise rather than evidence of a regression or gain.

This experiment therefore supports keeping the bridge as a guarded compatibility
path for tool interruptions and specialized synchronous finalizers. It does not
support enabling it as a cache-rate headline optimization until a workload with
frequent programmatic `DirectlyAnswer` calls demonstrates a causal gain.

## Actual bottleneck exposed by the run

The late run spent substantial wall time synchronously compressing Timeline and
expanding a single archive operation to nearly 200 indexed chunks. Prompt sizes
then reached roughly 790-899 KB while only about 7 KB remained aligned in many
lightweight calls. The corrected report shows:

- `dynamic`: 323 distinct hashes over 413 uses, minimum reuse 6%;
- `timeline-open`: 83 distinct hashes over 413 uses, minimum reuse 14%;
- 17 `lcp_hit_but_upstream_miss` records late in the run;
- intelligent upstream hit remained about 55%, far from 90%.

The next cache work should prioritize bounding and asynchronously persisting
Timeline archives, preventing raw tool output from multiplying archive chunks,
and moving stable task/TODO facts ahead of volatile Timeline Open content. Those
changes affect hundreds of calls; DirectlyAnswer convergence affected one call
in this workload.

## Follow-up experiment

Use a synthetic workload that deliberately triggers programmatic direct answers
from synchronous tool interruption and specialized-loop finalization. Compare
equal action checkpoints with the flag off and on, and assert all three:

1. standalone Direct Answer Decision prompt count decreases to zero;
2. the same evidence and TODO transitions appear in the main-loop answer;
3. total main-loop iterations do not increase enough to offset the removed call.
