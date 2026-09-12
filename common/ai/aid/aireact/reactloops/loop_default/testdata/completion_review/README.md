# Completion review regression lab

This lab runs the real `aim.InvokeReAct` entry point with `yak -c`. It uses
synthetic local JSON files, separate work directories and separate `YAKIT_HOME`
databases. No API key is stored in this directory; supply `REACT_LAB_API_KEY` in
the invoking process. Main, quality and speed callbacks are explicitly configured
for `deepseek-v4.1-flash` through the supplied OpenAI-compatible gateway.

## What is checked

`prepare.ps1` creates a production release with three enabled routes, a disabled
route, a migrated path and a nested override. The final export value is available
only from the last include (`timeout=120`, `proof=EXPORT-FINAL-174`), not its
parent (`90`, `EXPORT-963`). `verify.ps1` checks the actual output file against an
independent oracle, including route coverage, final values, valid source paths and
every file contributing a final value (including the deepest include).
The oracle is outside the fixture directory and is not part of the model prompt.

`analyze.ps1` extracts canonical TODO updates and timeline events. Inspect the
trajectory as well as the output: a passing result does not establish that every
independent target received its own TODO before execution, nor that a review's
assertion about coverage is true.

## Run

Build both executables from separate checkouts; the recorded baseline is
`d9edffca6` (main fetched on 2026-09-12). In PowerShell, with the API key already
in the process environment:

```powershell
go build -o C:/temp/yak-current.exe ./common/yak/cmd/yak.go
$lab = './common/ai/aid/aireact/reactloops/loop_default/testdata/completion_review'
& "$lab/run.ps1" -Binary C:/temp/yak-current.exe -RunDirectory C:/temp/react-natural -Mode natural
& "$lab/run.ps1" -Binary C:/temp/yak-current.exe -RunDirectory C:/temp/react-replay -Mode replay
& "$lab/run.ps1" -Binary C:/temp/yak-current.exe -RunDirectory C:/temp/react-greeting -Mode greeting
& "$lab/analyze.ps1" -RunDirectory C:/temp/react-replay
```

Each run directory must be new. `run.ps1` passes the contents of `run.yak` or
`replay.yak` as `yak -c` code. It restores its environment changes on return.
The runner has a 900-second timeout and an 80-work-iteration ceiling, with no
minimum iteration quota. The ceiling is a safety bound, not the success criterion.
The baseline replay is expected to fail `verify.ps1`; that is the regression being
demonstrated, even though the old engine emitted task success.

## Natural execution versus injected failure

Natural mode leaves every decision to the real model. It checks end-result
correctness and normal liveness, and should not be described as a reproduction if
the baseline also succeeds.

Replay mode deliberately replaces the first three primary decisions with a known
bad trajectory: read the index/registry and primary route, write a checkout-only
report while incorrectly closing the broad `release` TODO, then request finish.
The reads and write execute through real tool handlers. Every subsequent primary
decision uses the live model. This tests whether the host leaves an opportunity
to discover and repair omitted work; it is fault injection, not a natural-model
failure-rate measurement. The script identifies primary calls by the runtime's
caller label, not by searching potentially replayed prompt text.

## Observations (2026-09-12/13, Windows/amd64, Go 1.22.12)

| Case | main | Changed engine |
| --- | --- | --- |
| Natural three-route audit | Correct three-route result | Correct result in the final run and both development runs |
| Injected partial delivery + premature finish | Reports task success with only checkout; oracle fails | Blocks, continues with the live model, resolves invoice and export, rewrites and reads back the report; oracle passes |
| Recheck after repair in injected run | No completion checkpoint | Two checkpoints: before repair and after new work |
| Simple greeting | Existing behavior preserved by regression tests | One primary decision; ends after answer with no finish or completion checkpoint |

The natural baseline and final run each used seven primary decisions. The injected
baseline used three (all scripted); the repaired run used nine (three scripted and
six live decisions), with two completion checkpoints. These counts describe the
recorded traces and are not requirements imposed on the agent.

The normal runs still sometimes group independent routes into one TODO, and the
review may overstate that each discovered object was registered. These results
support the host gate and recovery behavior, not a claim that prompts or three
non-empty review fields prove semantic completeness. No statistical improvement
rate is claimed from this small sample. Tool count and response length are not
depth metrics: the output oracle and the dependency/source chain determine success.

An early replay harness version used prompt-text matching and a noncanonical
scalar parameter field; it did not inject all three decisions and was discarded.
Only runs logging all three `LAB_SEEDED_DECISION` markers qualify as the injected
comparison. Raw logs are local artifacts; do not commit credentials or provider
request dumps.

## Automated coverage

`completion_review_test.go` covers empty-list finish, task/evidence/TODO state
changes, invalid sidecars and malformed audits. The default-loop execution tests
cover discovery followed by actual work and a second checkpoint, plus bounded
validation failure when the model keeps producing malformed finish actions.

On Windows the relevant package suites can be run as follows:

```powershell
go test ./common/ai/aid/aireact/reactloops ./common/ai/aid/aireact/reactloops/loop_default ./common/ai/aid/aireact/reactloops/reactloopstests -skip 'TestParseWorkspaceAttachedContext|TestInitWorkspaceAttachedContext' -count=1 -timeout=3m
go test -race ./common/ai/aid/aireact/reactloops ./common/ai/aid/aireact/reactloops/loop_default -run 'TestCompletionReview|TestCompletionCheckpoint|TestMalformedFinish|TestFinish' -count=1 -timeout=2m
```

The skipped workspace-context tests contain six existing Windows failures caused
by `/tmp/...` expectations versus normalized backslash paths. The same failures
were reproduced on the clean baseline checkout. Do not skip them on platforms
where the path expectations hold; they are unrelated to completion review.
