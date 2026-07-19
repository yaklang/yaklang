# Yak Agent Harbor Benchmark v1

This directory contains the first Harbor evaluation slice for comparing two
Yak Agent builds on the same tasks, model, limits, and verifier logic.

## Quick start: smoke tests (no Docker/AI required)

```bash
# Structural smoke test — validates task definitions, oracle syntax, verifier logic
python3 benchmarks/harbor/scripts/smoke_test.py

# Go-level gateway smoke test — validates HTTP API surface (requires `yak` binary)
go test ./common/yakgrpc/aihttp/tests/ -run TestBenchmark -v -count=1
```

These tests run in seconds and catch structural issues before any
expensive Docker-based benchmark run.

## Local run (no Harbor, no Docker)

`scripts/run_local.py` runs the four `yak-agent-v1` tasks entirely on the
host. Two yak backends are available:

- **`--backend grpc` (default):** starts `yak grpc` and drives the agent via
  raw gRPC (`StartAIReAct` bidi stream). No HTTP gateway involved — config is
  seeded via `SetAIGlobalConfig`, and the full trace can be exported via
  `ExportAILogs` after each run. This is the recommended path.
- **`--backend http`:** starts `yak ai-http-gateway` and talks REST+SSE (the
  same flow the Harbor `YakAgent` uses). Useful as a fallback.

OpenCode is invoked as a local binary (the macOS Mach-O build works — nothing
is uploaded into a container). Challenge servers (for the three
security/recovery tasks) are started as plain `python3` subprocesses on free
host ports; the instruction and verifier paths are rewritten so scoring is
unchanged.

### One-time setup for the gRPC backend

Generate the Python gRPC stubs from the yak proto (self-contained, no
external imports):

```bash
pip3 install grpcio grpcio-tools protobuf   # if not already installed
mkdir -p benchmarks/harbor/agents/_grpc_stubs
python3 -m grpc_tools.protoc -Icommon/yakgrpc \
  --python_out=benchmarks/harbor/agents/_grpc_stubs \
  --grpc_python_out=benchmarks/harbor/agents/_grpc_stubs \
  common/yakgrpc/yakgrpc.proto
```

The stubs are gitignored (they are a build artifact). If you skip this step,
`--backend grpc` exits with a message pointing you here; `--backend http`
still works without it.

### The three things most people want

1. **Run the local yak engine on a task and save the result:**
   ```bash
   python3 benchmarks/harbor/scripts/run_local.py yak \
     --task direct-incident-summary --label base
   # add --backend http to use the HTTP gateway instead of raw gRPC
   ```

2. **Re-run with a freshly built yak engine and compare:**
   ```bash
   bash benchmarks/harbor/scripts/run_local.sh paired direct-incident-summary
   # reads YAK_BASE_BINARY / YAK_CANDIDATE_BINARY; writes verdict.json
   ```

3. **Compare against local OpenCode on the same task:**
   ```bash
   bash benchmarks/harbor/scripts/run_local.sh yak direct-incident-summary
   bash benchmarks/harbor/scripts/run_local.sh opencode direct-incident-summary
   python3 benchmarks/harbor/scripts/run_local.py compare \
     benchmarks/harbor/results/local/<stamp>/yak.jsonl \
     benchmarks/harbor/results/local/<stamp>/opencode.jsonl
   ```

`run_local.py` reuses `compare_results.py` for the verdict and emits JSONL
with the same fields as `harbor_results_to_jsonl.py`, so Harbor-produced and
locally-produced results are directly comparable. Each run leaves a sandbox
at `/tmp/yakbench/` with the agent's trajectory (`logs/trajectory.jsonl`),
the exported trace zip (`logs/trace.zip`, gRPC backend only), and the
verifier score (`logs/verifier/reward.json`). See `run_local.py --help` for
per-subcommand flags (`--attempts`, `--timeout`, `--max-iterations`).

### Troubleshooting: agent produces no tool calls / reward 0

The yak agent's tool set (`do_http_request`, `write_file`, `read_file`, ...)
is **not** hardcoded in the binary — these are `.yak` scripts synced into the
**profile database** (`ai_yak_tools` table) when the yak home is first
initialized. At run time, `StartAIReAct` queries that table to build the
agent's tool list.

`run_local.py` therefore uses the **default yak home** (`~/yakit-projects`)
rather than a throwaway temp dir, because a fresh empty home has an empty
`ai_yak_tools` table and the agent silently loses all yak-script tools (it
can still read/write files via the Go-builtin fs tools, but cannot make HTTP
requests, run yak scripts, etc.). If you see `tools=0` in the output or the
agent "can't find" tools it should have:

1. Check that the default home is initialized — run any yak command once
   (`yak version`) so the post-init hook populates the DB.
2. Don't override `--yak-home` to a fresh directory unless you've initialized
   it. If you must isolate, copy or symlink the default home's
   `yakit-profile-plugin.db` first.
3. Verify the tools are present:
   ```bash
   sqlite3 ~/yakit-projects/yakit-profile-plugin.db \
     "SELECT count(*) FROM ai_yak_tools"   # expect ~80
   ```

## What is included

- `scripts/run_local.py`: **local runner** (no Harbor/Docker). Drives yak via
  raw gRPC (`--backend grpc`, default) or HTTP gateway (`--backend http`),
  and runs OpenCode as a local binary. Emits JSONL directly consumable by
  `compare_results.py`. See "Local run" above.
- `agents/yak_agent.py`: a Harbor custom agent that uploads a selected `yak`
  binary and the model config, starts `ai-http-gateway`, submits the task, and
  records SSE events.
- `agents/gateway_runner.py`: the in-container REST/SSE client. Supports two
  execution modes:
  - `react` (default): submits via `POST /agent/run/{run_id}` backed by `StartAIReAct` gRPC.
  - `forgetask`: submits via Forge-based workflow backed by `StartAITask` gRPC.
  Seeds the tiered-ai-config via HTTP API (`POST /agent/setting/aiconfig`) and
  the simple setting (`POST /agent/setting`) as a fallback.
- `agents/opencode_agent.py`: a Harbor adapter that uploads a pinned Linux
  OpenCode binary instead of installing the latest npm package.
- `agents/opencode_runner.py`: runs `opencode run --format=json`, writes the
  raw event stream and produces the same efficiency summary fields as Yak.
- `datasets/yak-agent-v1`: four deterministic Harbor tasks (two smoke-level,
  two security-level).
- `scripts/smoke_test.py`: fast structural validation without Docker or Harbor.
- `scripts/smoke_gateway_test.py`: starts a local `yak ai-http-gateway` and
  validates the HTTP API flow (requires a `yak` binary, no AI provider needed).
- `scripts/validate_tasks.py`: fast structural validation without Harbor.
- `scripts/validate_oracle_noop.sh`: three clean oracle and noop runs per task.
- `scripts/build_yak_at_ref.sh`: builds a Linux binary from a Git ref via a
  manylinux container (CGO + gzip_embed), without switching the checkout.
- `scripts/gen_ai_config_yaml.py`: emits the tiered-ai-config YAML from
  `YAK_AI_*` env vars (secrets stay out of git).
- `scripts/run_paired.sh`: runs interleaved base/candidate jobs, then converts
  and compares the results into a single verdict.
- `scripts/run_opencode.sh`: runs OpenCode against the exact same four tasks
  and converts its results to comparison-ready JSONL.
- `scripts/harbor_results_to_jsonl.py`: converts Harbor job output into the
  flat JSONL `compare_results.py` consumes.
- `scripts/compare_results.py`: paired reward and efficiency comparison.

## Yak AI Agent task reception methods

The Yak AI Agent receives tasks through **7 distinct entry mechanisms**:

| # | Method | Protocol | Primary use case |
|---|--------|----------|-----------------|
| 1 | `StartAIReAct` (gRPC) | Bidirectional stream | Main reactive AI loop — used by HTTP gateway |
| 2 | `StartAITask` (gRPC) | Bidirectional stream | Forge-based task execution with triage |
| 3 | `StartAITriage` (gRPC) | Bidirectional stream | Intent recognition / classification |
| 4 | **HTTP REST/SSE Gateway** | `yak ai-http-gateway` → proxies to #1 | REST API at `/agent/*` |
| 5 | Yak Scripting (`aiagent`) | `aiagent.ExecuteForge()` | In-process from Yak scripts |
| 6 | IM Control | WeChat/DingTalk/Lark → #1 | IM bot integration |
| 7 | Vulinbox WebSocket | WebSocket at `/_/ws/agent` | Remote vuln testing agents |

The benchmark currently tests methods **#4** (HTTP Gateway → `StartAIReAct`)
and can optionally test **#2** (`StartAITask`) via `--mode forgetask`.

## Prerequisites

Install Harbor and make sure Docker is running:

```bash
uv tool install harbor==0.6.5
docker info
```

Harbor 0.6.5 is required; `run_paired.sh` asserts the version.

The custom agent runs the Yak HTTP gateway inside each Linux task container,
so the uploaded `yak` must be a **Linux** binary (not the macOS host build).
See "Build two Yak Agent versions" below.

### Model configuration (required for real runs)

The gateway resolves the upstream AI provider from a **tiered config**
persisted in the profile DB.  Two paths are available:

**Path 1 (full, recommended):** `POST /agent/setting/aiconfig` with a full
`AIGlobalConfig` payload including `Provider.APIKey` and `Provider.Domain`.
This is seeded by `gateway_runner.py`'s `seed_ai_config()` at container startup.

**Path 2 (simple):** `POST /agent/setting` with `AIService` + `AIModelName`.
This triggers `applySettingToRuntime` but does NOT carry API credentials —
it relies on provider keys pre-configured elsewhere in the profile DB.

Without either path, `AIService=openai` silently falls back to the built-in
free `memfit-*-free` model, making any benchmark reward meaningless.

Export the required environment variables:

```bash
export YAK_AI_TYPE=openai                       # provider type (default openai)
export YAK_AI_API_KEY=...                        # provider API key
export YAK_AI_DOMAIN=api.openai.com              # host; gateway appends /v1/chat/completions
export YAK_AI_MODEL=gpt-5.2-2025-12-11           # exact model id, pinned for base + candidate
```

Then generate the YAML (written to `benchmarks/harbor/ai-config.yaml`,
gitignored, mode 0600):

```bash
python3 benchmarks/harbor/scripts/gen_ai_config_yaml.py
```

Do not add credentials to tasks, images, result files, or Git.

## Validate the task set

### Smoke test (no Docker, no Harbor, no AI — seconds)

```bash
python3 benchmarks/harbor/scripts/smoke_test.py
```

This checks structural integrity, oracle syntax, and verifier logic for all
four tasks instantly.

### Gateway smoke test (needs `yak` binary, no AI provider)

```bash
YAK_BINARY_PATH=/path/to/yak python3 benchmarks/harbor/scripts/smoke_gateway_test.py
```

Starts a local `yak ai-http-gateway` and validates the full HTTP API flow
(session → SSE → run → events).  The agent will fail at the AI call stage
since no provider is configured — that's expected.

### Go integration tests

```bash
go test ./common/yakgrpc/aihttp/tests/ -run TestBenchmark -v -count=1
```

15 tests covering: session CRUD, setting CRUD, SSE content type, run submission,
CORS headers, cancel, AIInputEvent/AIOutputEvent/AIGlobalConfig JSON field
compatibility with the Python client.

### Structural validation (no Docker)

```bash
python3 benchmarks/harbor/scripts/validate_tasks.py
```

### Oracle/noop validator (requires Docker)

```bash
bash benchmarks/harbor/scripts/validate_oracle_noop.sh
```

Expected result: every oracle reward is `1.0`; every noop reward is `0.0`.

## Build two Yak Agent versions

The build runs in a `manylinux2014_aarch64` container so the output is a
Linux arm64 binary with CGO (pcap) and the `gzip_embed` resources baked in,
matching a release build. Docker must be running and `aarch64` is the default
target (override `YAK_BENCHMARK_GOARCH` for x86 hosts).

```bash
bash benchmarks/harbor/scripts/build_yak_at_ref.sh \
  main /tmp/yak-agent-benchmark/main/yak

bash benchmarks/harbor/scripts/build_yak_at_ref.sh \
  HEAD /tmp/yak-agent-benchmark/candidate/yak
```

The script uses a temporary Git worktree and leaves the current branch
untouched. It builds `linux/arm64` with CGO enabled and the `gzip_embed` tag,
matching the CI release build (`.github/scripts/build-linux-manylinux.sh`).
Override `YAK_BENCHMARK_GOARCH` (and `YAK_BENCHMARK_MANYLINUX_IMAGE`) when the
Docker daemon uses another architecture. A self-check runs the binary inside
the manylinux image and fails the build if it does not execute cleanly.

## Run a paired comparison

```bash
export YAK_BASE_BINARY=/tmp/yak-agent-benchmark/main/yak
export YAK_CANDIDATE_BINARY=/tmp/yak-agent-benchmark/candidate/yak
export YAK_AI_SERVICE=openai
export YAK_AI_MODEL=gpt-5.2-2025-12-11
export YAK_AI_CONFIG_FILE=$(pwd)/benchmarks/harbor/ai-config.yaml
export YAK_BENCHMARK_ATTEMPTS=5

bash benchmarks/harbor/scripts/run_paired.sh
```

The script interleaves base and candidate runs for each attempt (odd:
base→candidate, even: candidate→base) to reduce model-service time drift.
Each `harbor run` writes its job directory under the output root, records the
job name in `base-jobs.txt` / `candidate-jobs.txt`, and a failed job is logged
as `FAILED:` without aborting the matrix. After all runs it converts trial
results to JSONL (`harbor_results_to_jsonl.py`) and produces a paired verdict
(`compare_results.py` → `verdict.json`).

For a direct Harbor run, the custom agent uses `--agent-import-path` (not
`-a`, which only accepts Harbor's built-in agent names), and per-run settings
are passed via repeated `--agent-env` flags:

```bash
harbor run \
  -p benchmarks/harbor/datasets/yak-agent-v1 \
  --agent-import-path benchmarks.harbor.agents.yak_agent:YakAgent \
  -m "${YAK_AI_SERVICE}/${YAK_AI_MODEL}" \
  --agent-env "YAK_BINARY_PATH=/absolute/path/to/yak" \
  --agent-env "YAK_AI_CONFIG_FILE=/absolute/path/to/ai-config.yaml" \
  --force-build -o /tmp/yak-harbor-jobs -y
```

Harbor CLI flags can change between releases. `run_paired.sh` pins the version
and fails with a clear message if `harbor --version` is not 0.6.5.

## Decision rule

The main score is the verifier's `reward` value. Efficiency fields are derived
from `/logs/agent/benchmark-summary.json`.

- Improvement: paired mean reward increases and no critical security task
  regresses.
- Efficiency improvement: reward is non-inferior within `0.01`, while median
  duration, model events, or tool events improves by at least `10%`.
- Regression: any critical task loses more than `0.05`, or total mean reward
  loses more than `0.02`.

Three attempts are suitable for the first smoke run. Use at least five attempts
before treating a small difference as a reliable improvement.

## Run the same tasks with OpenCode

The adapter requires a Linux OpenCode binary matching the task container
architecture. The normal binary under `~/.opencode/bin/opencode` on macOS is a
Mach-O executable and cannot run inside Harbor's Linux container.

Use the same provider, exact model ID, AI config, attempt count, and task set as
the Yak run:

```bash
export OPENCODE_BINARY_PATH=/absolute/path/to/linux/opencode
export OPENCODE_MODEL=deepseek/deepseek-v4-flash
export OPENCODE_AI_CONFIG_FILE=$(pwd)/benchmarks/harbor/ai-config.yaml
export OPENCODE_BENCHMARK_ATTEMPTS=5

bash benchmarks/harbor/scripts/run_opencode.sh
```

The AI config is the same credential file generated by
`gen_ai_config_yaml.py`. The OpenCode runner converts its first
`intelligent_configs` entry into a private in-container `opencode.json` and
checks that its provider/model exactly matches `OPENCODE_MODEL`.

Each successful run produces:

```text
<job>/<trial>/agent/opencode.txt
<job>/<trial>/agent/final.txt
<job>/<trial>/agent/trajectory.json
<job>/<trial>/agent/benchmark-summary.json
<output>/opencode.jsonl
```

`opencode.jsonl` uses the same reward, duration, model-event, tool-event, and
token fields as the Yak JSONL files, so it can be passed directly to
`compare_results.py`:

```bash
python3 benchmarks/harbor/scripts/compare_results.py \
  /path/to/yak.jsonl /path/to/opencode.jsonl \
  --output /tmp/yak-vs-opencode.json
```
