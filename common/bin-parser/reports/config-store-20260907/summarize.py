#!/usr/bin/env python3
"""Validate matched config-store measurements and print summaries (stdlib only).

All measured runs stay in the result. Percentiles are summarized per run, not
pooled. Pilot, race and correctness durations are excluded from performance.
"""
import json
import re
import statistics
from pathlib import Path

ROOT = Path(__file__).resolve().parent


def stats(values):
    return {"median": statistics.median(values), "min": min(values), "max": max(values)}


def blocks(filename):
    text = (ROOT / filename).read_text()
    assert "truncated output" not in text
    starts = list(re.finditer(r"^RUN (.+)$", text, re.M))
    for i, start in enumerate(starts):
        end = starts[i + 1].start() if i + 1 < len(starts) else len(text)
        block = text[start.end():end]
        assert len(re.findall(r"^EXIT 0$", block, re.M)) == 1
        yield dict(part.split("=", 1) for part in start[1].split()), block, text[:start.start()].count("\n") + 1


def benchmark_summary():
    groups = {}
    for filename in ("application-benchmarks.txt", "envelope-benchmarks.txt"):
        run_count = 0
        for header, block, line in blocks(filename):
            run_count += 1
            measurements = re.findall(r"^BenchmarkCurrentCorpus.*$", block, re.M)
            assert len(measurements) == (2 if filename.startswith("application") else 1)
            for measurement in measurements:
                parts = measurement.split()
                name = parts[0].split("-")[0].removeprefix("BenchmarkCurrentCorpus")
                metrics = {parts[i + 1]: float(parts[i]) for i in range(2, len(parts), 2)}
                workers = int(header["cpu"])
                assert metrics["workers"] == workers
                expected = (58533, 8993088) if name == "Envelopes" else (11, 1421)
                assert (metrics["messages/op"], metrics["input-B/op"]) == expected
                assert metrics["ns/op"] > 0 and int(parts[1]) > 0
                row = {
                    "repetition": int(header["repetition"]),
                    "ns_per_op": metrics["ns/op"], "bytes_per_op": metrics["B/op"],
                    "allocs_per_op": metrics["allocs/op"],
                    "messages_per_second": metrics["messages/op"] * 1e9 / metrics["ns/op"],
                    "input_mbps": metrics["input-B/op"] * 8000 / metrics["ns/op"],
                    "source": {"file": filename, "run_line": line},
                }
                key = (name, workers, header["variant"])
                groups.setdefault(key, []).append(row)
        assert run_count == 18
    assert len(groups) == 18
    result = []
    for (name, workers, variant), rows in sorted(groups.items()):
        assert sorted(row["repetition"] for row in rows) == [1, 2, 3]
        item = {"workload": name, "workers": workers, "variant": variant, "n": len(rows)}
        for field in ("ns_per_op", "bytes_per_op", "allocs_per_op", "messages_per_second", "input_mbps"):
            item[field] = stats([row[field] for row in rows])
        item["sources"] = [row["source"] for row in rows]
        result.append(item)
    comparisons = []
    for name in ("Envelopes", "ApplicationFields", "ApplicationJSON"):
        for workers in (1, 4, 10):
            pair = {row["variant"]: row for row in result if row["workload"] == name and row["workers"] == workers}
            old, new = pair["baseline"], pair["compact"]
            comparisons.append({
                "workload": name, "workers": workers,
                "throughput_gain_percent": (old["ns_per_op"]["median"] / new["ns_per_op"]["median"] - 1) * 100,
                "allocation_bytes_reduction_percent": (1 - new["bytes_per_op"]["median"] / old["bytes_per_op"]["median"]) * 100,
                "allocation_count_reduction_percent": (1 - new["allocs_per_op"]["median"] / old["allocs_per_op"]["median"]) * 100,
            })
    return result, comparisons


def replay_summary():
    groups = {}
    totals = {"runs": 0, "scheduled_seconds": 0, "offered": 0, "offered_bytes": 0, "dropped": 0}
    for header, block, line in blocks("replay-comparison.txt"):
        measurements = re.findall(r"CURRENT_CORPUS_REPLAY (.+)", block)
        assert len(measurements) == 1
        row = json.loads(measurements[0])
        assert row["workload"] == "known-profile-packet-and-application-json"
        assert (row["cycle_packets"], row["cycle_applications"], row["cycle_packet_bytes"], row["cycle_json_bytes"]) == (38, 11, 3897, 33820)
        assert row["workers"] == int(header["cpu"]) == 4
        assert row["requested_mbps"] == int(header["mbps"])
        assert row["window_seconds"] == int(header["duration"].removesuffix("s"))
        assert row["queue_capacity"] == 64
        assert row["offered"] == row["completed"] + row["dropped"] == 73137
        assert row["offered_bytes"] == row["completed_bytes"] + row["dropped_bytes"] == 7499881
        assert row["completed_in_window"] <= row["completed"]
        assert row["completed_in_window_bytes"] <= row["completed_bytes"]
        assert row["parse_errors"] == 0 and len(row["by_record"]) == 38
        for field in ("offered", "completed", "dropped", "parse_errors"):
            assert sum(record[field] for record in row["by_record"].values()) == row[field]
        for record in row["by_record"].values():
            assert record["offered"] == record["completed"] + record["dropped"]
        assert abs(row["offered_bytes"] * 8 / row["window_seconds"] / 1e6 - row["offered_mbps"]) < 1e-9
        assert abs(row["completed_in_window_bytes"] * 8 / row["window_seconds"] / 1e6 - row["completed_in_window_mbps"]) < 1e-9
        process = re.search(r"([\d.]+) real\s+([\d.]+) user\s+([\d.]+) sys", block)
        rss = re.search(r"(\d+)\s+maximum resident set size", block)
        assert process and rss
        row["process_cpu_equivalents"] = (float(process[2]) + float(process[3])) / float(process[1])
        row["process_peak_rss_mib"] = int(rss[1]) / 2**20
        row["drop_percent"] = row["dropped"] * 100 / row["offered"]
        row["total_p99_ms"] = row["scheduled_total_latency"]["p99_ms"]
        row["dispatch_max_ms"] = row["dispatch_lateness"]["p100_ms"]
        row["repetition"] = int(header["repetition"])
        row["source"] = {"file": "replay-comparison.txt", "run_line": line}
        key = (header["variant"], row["requested_mbps"], row["window_seconds"])
        groups.setdefault(key, []).append(row)
        totals["runs"] += 1
        totals["scheduled_seconds"] += row["window_seconds"]
        for field in ("offered", "offered_bytes", "dropped"):
            totals[field] += row[field]
    assert totals["runs"] == 12 and len(groups) == 4
    result = []
    for (variant, rate, duration), rows in sorted(groups.items()):
        assert sorted(row["repetition"] for row in rows) == [1, 2, 3]
        item = {"variant": variant, "workers": 4, "requested_mbps": rate, "window_seconds": duration, "n": 3,
                "zero_drop_runs": sum(row["dropped"] == 0 for row in rows)}
        for field in ("dropped", "drop_percent", "completed_in_window_mbps", "total_p99_ms", "dispatch_max_ms", "allocated_bytes", "gc_count", "process_cpu_equivalents", "process_peak_rss_mib"):
            item[field] = stats([row[field] for row in rows])
        item["sources"] = [row["source"] for row in rows]
        result.append(item)
    return result, totals


def digest_summary():
    records = {}
    for variant in ("baseline", "compact"):
        text = (ROOT / f"{variant}-export-digest.txt").read_text()
        assert "truncated output" not in text
        rows = re.findall(r"CURRENT_EXPORT_DIGEST workload=(\w+) worker=(\d+) records=(\d+) sha256=([0-9a-f]{64})", text)
        assert len(rows) == 16
        for workload in ("envelopes", "application"):
            for worker in range(4):
                selected = [row for row in rows if row[:2] == (workload, str(worker))]
                assert len(selected) == 2 and selected[0] == selected[1]
                expected = ([14634, 14633, 14633, 14633] if workload == "envelopes" else [3, 3, 3, 2])[worker]
                assert int(selected[0][2]) == expected
        records[variant] = sorted(rows)
    assert records["baseline"] == records["compact"]
    return {"equal": True, "repeats_per_binary": 2, "envelopes_per_repeat": 58533, "applications_per_repeat": 11,
            "partitions": [{"workload": row[0], "worker": int(row[1]), "records": int(row[2]), "sha256": row[3]}
                           for row in sorted(set(records["baseline"]))]}


if __name__ == "__main__":
    benchmarks, comparisons = benchmark_summary()
    replay, totals = replay_summary()
    print(json.dumps({
        "aggregation": "median and min-max over all 3 runs per configuration; no excluded formal runs; replay percentiles are per-run",
        "benchmark_groups": benchmarks, "benchmark_comparisons": comparisons,
        "replay_groups": replay, "replay_totals": totals,
        "export_equivalence": digest_summary(),
    }, ensure_ascii=False, indent=2))
