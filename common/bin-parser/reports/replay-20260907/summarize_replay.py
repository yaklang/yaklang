#!/usr/bin/env python3
"""Validate and summarize repeated bounded-replay logs; stdlib only, stdout.

No outlier removal, no pooling percentiles, no live-link capacity inference.
Separate duration / worker / input-rate configurations are never combined.
"""
import json
import statistics
import sys
from pathlib import Path


def summary(paths):
    groups = {}
    runs = []
    for path in paths:
        for lineno, line in enumerate(Path(path).read_text().splitlines(), 1):
            if "CURRENT_CORPUS_REPLAY " not in line:
                continue
            row = json.loads(line.split("CURRENT_CORPUS_REPLAY ", 1)[1])
            row["source"] = {"file": Path(path).name, "line": lineno}
            assert row["workload"] == "known-profile-packet-and-application-json"
            assert (row["cycle_packets"], row["cycle_applications"], row["cycle_packet_bytes"], row["cycle_json_bytes"]) == (38, 11, 3897, 33820)
            assert row["queue_capacity"] == 64
            assert row["offered"] == row["completed"] + row["dropped"]
            assert row["offered_bytes"] == row["completed_bytes"] + row["dropped_bytes"]
            assert row["completed_in_window"] <= row["completed"]
            assert row["completed_in_window_bytes"] <= row["completed_bytes"]
            assert row["parse_errors"] == 0, "Do not classify parse failures as capacity results"
            assert len(row["by_record"]) == 38
            for field in ("offered", "completed", "dropped", "parse_errors"):
                assert sum(record[field] for record in row["by_record"].values()) == row[field]
            for record in row["by_record"].values():
                assert record["offered"] == record["completed"] + record["dropped"]
            assert abs(row["offered_bytes"] * 8 / row["window_seconds"] / 1e6 - row["offered_mbps"]) < 1e-9
            assert abs(row["completed_in_window_bytes"] * 8 / row["window_seconds"] / 1e6 - row["completed_in_window_mbps"]) < 1e-9
            row["drop_percent"] = row["dropped"] * 100 / row["offered"]
            row["drain_ms"] = (row["drained_seconds"] - row["window_seconds"]) * 1000
            key = (row["workers"], row["requested_mbps"], row["window_seconds"])
            groups.setdefault(key, []).append(row)
            runs.append(row)
    assert runs, "No replay measurements found"

    def values(rows, *keys):
        numbers = []
        for row in rows:
            value = row
            for key in keys:
                value = value[key]
            numbers.append(value)
        return {"median": statistics.median(numbers), "min": min(numbers), "max": max(numbers)}

    results = []
    for (workers, rate, duration), rows in sorted(groups.items()):
        assert len(rows) == 3, f"Expected three repeats: {(workers, rate, duration)} has {len(rows)}"
        results.append({
            "workers": workers, "requested_mbps": rate, "window_seconds": duration, "n": len(rows),
            "zero_drop_runs": sum(row["dropped"] == 0 for row in rows),
            "drop_percent": values(rows, "drop_percent"),
            "completed_in_window_mbps": values(rows, "completed_in_window_mbps"),
            "scheduled_total_p99_ms": values(rows, "scheduled_total_latency", "p99_ms"),
            "scheduled_total_max_ms": values(rows, "scheduled_total_latency", "p100_ms"),
            "dispatch_p99_ms": values(rows, "dispatch_lateness", "p99_ms"),
            "dispatch_max_ms": values(rows, "dispatch_lateness", "p100_ms"),
            "queue_p99_ms": values(rows, "queue_latency", "p99_ms"),
            "queue_observed_max": values(rows, "observed_queue_max"),
            "drain_ms": values(rows, "drain_ms"),
            "allocated_bytes": values(rows, "allocated_bytes"),
            "gc_count": values(rows, "gc_count"),
            "end_heap_bytes": values(rows, "end_heap_bytes"),
            "sources": [row["source"] for row in rows],
        })
    return {"aggregation": "median and min-max over 3 runs per configuration; percentiles remain per-run; no excluded runs", "groups": results, "runs": runs}


if __name__ == "__main__":
    args = sys.argv[1:]
    summary_only = "--summary-only" in args
    if summary_only:
        args.remove("--summary-only")
    if not args:
        raise SystemExit("usage: summarize_replay.py [--summary-only] LOG [LOG ...]")
    result = summary(args)
    if summary_only:
        del result["runs"]  # complete measurements remain in linked raw logs
    print(json.dumps(result, ensure_ascii=False, indent=2))
