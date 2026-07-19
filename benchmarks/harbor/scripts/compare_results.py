#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import statistics
from collections import defaultdict
from pathlib import Path


def load(path: Path) -> dict[tuple[str, int], dict]:
    records: dict[tuple[str, int], dict] = {}
    for line_number, line in enumerate(path.read_text().splitlines(), 1):
        if not line.strip():
            continue
        record = json.loads(line)
        task = str(record["task"])
        attempt = int(record["attempt"])
        record["reward"] = float(record["reward"])
        records[(task, attempt)] = record
    if not records:
        raise ValueError(f"{path} contains no records")
    return records


def mean(values: list[float]) -> float:
    return statistics.fmean(values) if values else 0.0


def percent_change(base: float, candidate: float) -> float | None:
    if base == 0:
        return None
    return (candidate - base) / base * 100.0


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Compare normalized paired Harbor result JSONL files. "
        "Produce the JSONL inputs with harbor_results_to_jsonl.py."
    )
    parser.add_argument("base", type=Path)
    parser.add_argument("candidate", type=Path)
    parser.add_argument("--critical-prefix", default="security-")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()

    base = load(args.base)
    candidate = load(args.candidate)
    pairs = sorted(base.keys() & candidate.keys())
    if not pairs:
        raise ValueError("base and candidate have no matching task/attempt pairs")

    by_task: dict[str, list[float]] = defaultdict(list)
    reward_deltas: list[float] = []
    for key in pairs:
        delta = candidate[key]["reward"] - base[key]["reward"]
        reward_deltas.append(delta)
        by_task[key[0]].append(delta)

    metric_summary: dict[str, dict] = {}
    for metric in ("duration_sec", "tool_event_count", "model_event_count"):
        base_values = [
            float(base[key][metric])
            for key in pairs
            if metric in base[key] and metric in candidate[key]
        ]
        candidate_values = [
            float(candidate[key][metric])
            for key in pairs
            if metric in base[key] and metric in candidate[key]
        ]
        if base_values:
            base_mean = mean(base_values)
            candidate_mean = mean(candidate_values)
            metric_summary[metric] = {
                "base_mean": round(base_mean, 4),
                "candidate_mean": round(candidate_mean, 4),
                "change_percent": percent_change(base_mean, candidate_mean),
            }

    task_deltas = {task: round(mean(values), 4) for task, values in by_task.items()}
    critical_regressions = {
        task: delta
        for task, delta in task_deltas.items()
        if task.startswith(args.critical_prefix) and delta < -0.05
    }
    mean_delta = mean(reward_deltas)
    verdict = "no-material-change"
    if critical_regressions or mean_delta < -0.02:
        verdict = "regression"
    elif mean_delta > 0:
        verdict = "improvement"
    elif mean_delta >= -0.01 and any(
        (item.get("change_percent") or 0) <= -10 for item in metric_summary.values()
    ):
        verdict = "efficiency-improvement"

    report = {
        "paired_trials": len(pairs),
        "base_mean_reward": round(mean([base[key]["reward"] for key in pairs]), 4),
        "candidate_mean_reward": round(
            mean([candidate[key]["reward"] for key in pairs]), 4
        ),
        "paired_mean_reward_delta": round(mean_delta, 4),
        "task_reward_deltas": task_deltas,
        "critical_regressions": critical_regressions,
        "efficiency": metric_summary,
        "verdict": verdict,
    }
    rendered = json.dumps(report, indent=2, sort_keys=True)
    print(rendered)
    if args.output:
        args.output.write_text(rendered + "\n")
    return 1 if verdict == "regression" else 0


if __name__ == "__main__":
    raise SystemExit(main())

