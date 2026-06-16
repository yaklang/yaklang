# Memfit AI Agent Eval Harness

Headless evaluation framework for Memfit AI Agent + LLM on whitebox vulnerability detection tasks.

## Architecture

```
eval-harness (Go)
  │
  ├─ harness/client.go     gRPC client wrapper
  ├─ harness/runner.go     Task execution & event collection
  ├─ harness/evaluator.go  Metric computation (Recall, FPR, F1, etc.)
  ├─ harness/reporter.go   JSON/Markdown report generation
  │
  ├─ cases/                Ground truth definitions
  │   └── ground_truth/    Per-CVE JSON files
  │
  ├─ cmd/eval/main.go      CLI entry point
  │
  └─ results/              Evaluation output (gitignored)
```

## Prerequisites

1. **yaklang gRPC server running headlessly**:
   ```bash
   yak grpc --port 8087
   ```

2. **AI Provider configured** (Minmax M3 via Aibalance, or any compatible model).
   Configure via Memfit UI or programmatically via `SetAIGlobalConfig` gRPC call.

3. **Go 1.22+** (same as yaklang_engine).

## Usage

### Run single evaluation

```bash
cd yaklang_engine/eval
go run ./cmd/eval \
  --model "minmax-m3" \
  --case cases/ground_truth/cve-2024-xxxx.json \
  --output results/ \
  --review yolo \
  --max-iter 30
```

### Run reproducibility test (multiple runs)

```bash
go run ./cmd/eval \
  --model "minmax-m3" \
  --case cases/ground_truth/cve-2024-xxxx.json \
  --runs 5 \
  --output results/
```

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--model` | (required) | AI model name |
| `--service` | (empty, uses default) | AI provider service name |
| `--case` | (required) | Path to ground truth JSON |
| `--output` | `results/` | Output directory |
| `--max-iter` | `30` | Max ReAct iterations |
| `--review` | `yolo` | Review policy: `yolo`/`manual`/`ai` |
| `--addr` | `127.0.0.1:8087` | gRPC server address |
| `--runs` | `1` | Number of runs |

## Ground Truth Format

Create a JSON file in `cases/ground_truth/`:

```json
{
  "cve_id": "CVE-2024-12345",
  "project_url": "https://github.com/vuln-project/vuln-app",
  "commit_hash": "abc1234",
  "description": "SQL injection in user search",
  "vulns": [
    {
      "id": "VULN-001",
      "type": "sqli",
      "file": "src/main/java/com/example/UserDao.java",
      "line": 42,
      "description": "SQL injection via string concatenation",
      "keywords": ["sql injection", "UserDao", "concatenat"]
    }
  ]
}
```

## Metrics

| Metric | Formula | Description |
|--------|---------|-------------|
| **Recall** | TP / (TP + FN) | % of ground-truth vulns found |
| **Precision** | TP / (TP + FP) | % of reported vulns that are real |
| **F1 Score** | 2·P·R / (P+R) | Harmonic mean |
| **FPR** | FP / (FP + TN) | False positive rate |
| **Reasoning Quality** | heuristic | Based on thought/error ratio |
| **Duration** | wall clock | Task completion time |

## Output

Each run produces:
- `results/<cve_id>_<timestamp>.json` — Full metrics + vuln matches
- `results/<cve_id>_<timestamp>.md` — Human-readable summary
- `results/logs/<cve_id>_<run>.zip` — Full AI session logs (checkpoints, timeline, memory, events)

## Extending

### Adding a new CVE case

1. Create `cases/ground_truth/cve-YYYY-NNNNN.json` with the ground truth
2. Clone the vulnerable project version and note the commit hash
3. Run the eval harness

### Custom evaluation logic

Modify `harness/evaluator.go` to change matching heuristics or add new metrics.
