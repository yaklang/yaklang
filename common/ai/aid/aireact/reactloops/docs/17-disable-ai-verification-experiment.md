# Disable AI verification experiment

## Question

Measure whether removing AI-backed satisfaction verification improves cache
behavior and task execution, while retaining deterministic completion:

- `finish` is the only explicit action that terminates a ReAct loop;
- `finish` is rejected while the current task owns an active TODO;
- TODO state is updated by `next_movements` on any action or by
  `adjust_todolist`;
- no automatic, explicit, watchdog, or specialized action path may issue a
  verification prompt.

## Reproduction

Both runs used the repository engine directly:

```bash
go run common/yak/cmd/yak.go \
  common/ai/aid/aicache/cachebench/run_react.yak \
  --input '进行主机体检' \
  --ai-type aibalance \
  --max-iteration 15 \
  --max-duration 1200 \
  --stall-timeout 180 \
  --fatal-error-threshold 0 \
  --output-dir <output>
```

The experiment run additionally used `--disable-ai-verification`. Each run had
an isolated persistent session. No prompt-count cutoff was configured.

Baseline report: `/tmp/yaklang-noverify-experiment/baseline/cachebench-20260718-140055.json`.
Experiment report: `/tmp/yaklang-noverify-experiment/after-fixed/cachebench-20260718-143239.json`.

## Verification attribution

Exact prompt-schema attribution found six baseline prompts whose schema had
`"const": "verify-satisfaction"`, at request sequences 31, 69, 95, 129, 168,
and 198. All six were routed to the intelligent model.

| metric | baseline verification | disabled |
| --- | ---: | ---: |
| calls | 6 | 0 |
| prompt tokens | 319,325 | 0 |
| cached tokens | 169,695 | 0 |
| uncached prompt tokens | 149,630 | 0 |
| upstream token hit | 53.14% | n/a |

Verification represented 14.63% of baseline intelligent calls and 19.42% of
baseline intelligent prompt tokens.

## Equal-call comparison

The complete runs reached different execution depths, so the primary causal
comparison uses the first 206 calls from each run. This is the complete baseline
sample size.

| metric, first 206 calls | baseline | disabled | change |
| --- | ---: | ---: | ---: |
| intelligent calls | 41 | 43 | +2 |
| all prompt tokens | 4,402,305 | 4,130,241 | -6.18% |
| intelligent prompt tokens | 1,644,573 | 1,431,068 | -12.98% |
| upstream token hit, all | 29.17% | 26.56% | -2.60 pp |
| upstream token hit, intelligent | 49.97% | 48.90% | -1.07 pp |
| client LCP hit, all | 33.39% | 43.75% | +10.36 pp |
| client LCP hit, intelligent | 29.45% | 32.44% | +2.99 pp |
| dynamic distinct hashes | 166 | 159 | -7 |
| timeline-open distinct hashes | 52 | 49 | -3 |
| semi-dynamic-2 distinct hashes | 11 | 9 | -2 |

Disabling verification therefore reduced high-cost prompt volume and structural
prefix drift. It did not improve the upstream token-hit ratio in the equal-call
window. The provider cache result is affected by request/model mix and cannot be
inferred from client LCP alone.

## Complete-run outcome

The baseline stopped after 645 seconds because no usage callback arrived for
180 seconds. It captured 206 calls, six task result summaries, 16 tool-call
artifacts, one direct answer, and one explicit finish.

The disabled run reached the 1200-second hard cap with 376 calls and no
verification prompt. It produced all six task result summaries, 38 tool-call
artifacts, five direct answers, and six explicit finishes. Its final metrics
were:

- intelligent calls: 76;
- intelligent upstream token hit: 55.74%;
- intelligent client LCP hit: 26.85%;
- all-tier upstream token hit: 30.30%;
- all-tier client LCP hit: 33.59%.

The disabled run progressed through Timeline archival that had stalled the
baseline and emitted the final child-task answer. It still did not terminate
naturally: Timeline fork/embedding cleanup encountered an upstream 502 and the
outer invocation remained alive until the hard cap.

## Findings

1. AI verification is removable without losing the deterministic TODO safety
   gate. Real child tasks closed TODOs through ordinary action
   `next_movements`, then used `finish`.
2. The switch must be inherited by every child Config. Without propagation in
   `ConvertConfigToOptions`, nested blueprint loops silently re-enable
   verification.
3. The old Timeline Open prompt incorrectly said that only verification could
   mutate TODO state. That conflicts with the global `next_movements` runtime
   and makes verification-free completion unreliable.
4. Removing verification alone does not produce a 90% upstream cache hit rate.
   The remaining dominant drift is in `dynamic` and `timeline-open`, and the
   remaining tail-latency risk is Timeline archive/embedding cleanup.
5. Verification-free subtasks emitted more user-visible `directly_answer`
   actions before `finish`. A follow-up experiment should suppress child-task
   chat emission and let the coordinator synthesize one final answer.

## Next experiments

1. Make child-task completion use `finish` directly when its result summary is
   already persisted; reserve `directly_answer` for the root task.
2. Decouple Timeline archive/embedding cleanup from the synchronous task exit
   path and bound it with its own timeout.
3. Compare equal-task-stage checkpoints in addition to equal-call windows, so
   extra useful progress is separated from extra overhead.
4. Continue moving stable task/instruction material ahead of volatile Timeline
   content; removing verification reduces one behavior family but does not fix
   dynamic-section growth.
