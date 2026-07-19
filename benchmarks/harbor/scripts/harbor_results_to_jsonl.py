#!/usr/bin/env python3
"""Convert Harbor 0.6.5 job output into the flat JSONL compare_results.py needs.

Harbor writes one directory per job and one subdir per trial:
    <output_root>/<job-name>/<task-short>__<id>/result.json
Each trial result.json has:
    task_name = "yak-agent-v1/<task>"          -> we strip the prefix
    verifier_result.rewards.{outcome,evidence,format,reward}
    agent_execution.started_at / verifier.finished_at  (ISO-8601)
    <trial>/agent/benchmark-summary.json  (only on successful YakAgent runs,
        written by gateway_runner.py; carries tool/model event counts)

compare_results.py joins base vs candidate on (task, attempt). Harbor trial
names carry no attempt index, so we assign attempts 1..N per task, ordered by
trial started_at across all input job dirs. This is deterministic as long as
base and candidate jobs were interleaved in wall-clock order (run_paired.sh
does this).

Usage:
    harbor_results_to_jsonl.py <job-dir>... --label base --output base.jsonl
"""
from __future__ import annotations

import argparse
import datetime as dt
import json
import sys
from pathlib import Path

TASK_PREFIX = "yak-agent-v1/"


def parse_iso(value: str | None) -> dt.datetime | None:
    if not value:
        return None
    # Harbor emits e.g. "2026-07-18T06:39:05.630996Z"; fromisoformat wants +00:00
    cleaned = value.replace("Z", "+00:00")
    try:
        return dt.datetime.fromisoformat(cleaned)
    except ValueError:
        return None


def load_trial(trial_dir: Path, job_name: str) -> dict | None:
    result_file = trial_dir / "result.json"
    if not result_file.is_file():
        return None
    try:
        data = json.loads(result_file.read_text())
    except json.JSONDecodeError as exc:
        print(f"warn: could not parse {result_file}: {exc}", file=sys.stderr)
        return None

    task_name = str(data.get("task_name") or "")
    task = task_name[len(TASK_PREFIX):] if task_name.startswith(TASK_PREFIX) else task_name
    if not task:
        return None

    rewards = (data.get("verifier_result") or {}).get("rewards") or {}
    verifier_result = data.get("verifier_result")
    errored = verifier_result is None or data.get("exception_info") is not None

    # duration = verifier.finished_at - agent_execution.started_at (agent+verify
    # wall time). Falls back to trial-level started_at/finished_at.
    started = (
        parse_iso((data.get("agent_execution") or {}).get("started_at"))
        or parse_iso(data.get("started_at"))
    )
    verifier_finished = parse_iso((data.get("verifier") or {}).get("finished_at"))
    finished = verifier_finished or parse_iso(data.get("finished_at"))
    duration_sec = (
        round((finished - started).total_seconds(), 3)
        if started and finished and finished > started
        else None
    )

    record: dict = {
        "task": task,
        "reward": float(rewards.get("reward") or 0.0),
        "outcome": float(rewards.get("outcome") or 0.0),
        "evidence": float(rewards.get("evidence") or 0.0),
        "format": float(rewards.get("format") or 0.0),
        "errored": errored,
        "trial_name": data.get("trial_name") or trial_dir.name,
        "job_name": job_name,
        "started_at": data.get("started_at"),
    }
    if duration_sec is not None:
        record["duration_sec"] = duration_sec

    # Optional efficiency metrics from gateway_runner.py's benchmark-summary.
    summary_file = trial_dir / "agent" / "benchmark-summary.json"
    if summary_file.is_file():
        try:
            summary = json.loads(summary_file.read_text())
            for key in ("tool_event_count", "model_event_count", "event_count",
                        "final_text_chars"):
                if key in summary:
                    record[key] = int(summary[key])
            # Token consumption
            token = summary.get("token")
            if isinstance(token, dict):
                record["input_tokens"] = int(token.get("input", 0))
                record["output_tokens"] = int(token.get("output", 0))
                record["cache_hit_tokens"] = int(token.get("cache_hit", 0))
                tier = token.get("tier")
                if isinstance(tier, dict):
                    record["tier_tokens"] = tier
            # Duration from agent (more precise than verifier wall time)
            ds = summary.get("duration_sec")
            if isinstance(ds, (int, float)):
                record["agent_duration_sec"] = round(float(ds), 1)
        except (json.JSONDecodeError, TypeError):
            pass

    return record


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("job_dirs", nargs="+", type=Path, help="Harbor job directories")
    parser.add_argument("--label", default="run", help="label stamped on each record")
    parser.add_argument(
        "--output", "-o", type=Path, required=True, help="output JSONL path"
    )
    args = parser.parse_args()

    records: list[dict] = []
    for job_dir in args.job_dirs:
        if not job_dir.is_dir():
            print(f"warn: skipping non-directory: {job_dir}", file=sys.stderr)
            continue
        job_name = job_dir.name
        for trial_dir in sorted(job_dir.iterdir()):
            if not trial_dir.is_dir():
                continue
            record = load_trial(trial_dir, job_name)
            if record is not None:
                record["label"] = args.label
                records.append(record)

    if not records:
        print("no trial records found", file=sys.stderr)
        return 1

    # Assign per-task attempt indices by started_at order.
    records.sort(key=lambda r: (r["task"], r["started_at"] or ""))
    counters: dict[str, int] = {}
    for record in records:
        task = record["task"]
        counters[task] = counters.get(task, 0) + 1
        record["attempt"] = counters[task]

    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open("w") as fh:
        for record in records:
            # Reorder for readability: task, attempt, reward first.
            ordered = {
                k: record[k]
                for k in (
                    "task",
                    "attempt",
                    "reward",
                    "outcome",
                    "evidence",
                    "format",
                    "duration_sec",
                    "agent_duration_sec",
                    "tool_event_count",
                    "model_event_count",
                    "event_count",
                    "final_text_chars",
                    "input_tokens",
                    "output_tokens",
                    "cache_hit_tokens",
                    "errored",
                    "label",
                    "trial_name",
                    "job_name",
                    "started_at",
                )
                if k in record
            }
            fh.write(json.dumps(ordered, sort_keys=True) + "\n")

    errored = sum(1 for r in records if r.get("errored"))
    print(
        f"wrote {len(records)} records to {args.output} "
        f"({errored} errored -> reward 0.0)",
        file=sys.stderr,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
