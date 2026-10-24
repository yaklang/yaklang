"""Export only aggregate public-corpus evidence; never copy packet/key contents.

Usage: summarize_public_corpus.py EXTERNAL_AUDIT_ROOT REPORT_DIRECTORY
The root must contain weber-final and sharon-final from public_corpus.py.
"""
import collections
import csv
import json
from pathlib import Path
import sys

root, out = map(Path, sys.argv[1:3])
if any((p / ".git").exists() for p in [root.resolve(), *root.resolve().parents]):
    raise SystemExit("Raw audit inputs must remain outside Git")
out.mkdir(parents=True, exist_ok=True)
keys = ("frames", "manifest", "yak_sha256", "events_binary_sha256", "audit_script_sha256",
        "ws_labels", "yak_labels", "ws_protocol_presence", "native_protocol_presence",
        "native_event_status", "events_without_packet_references", "field_frames_checked",
        "field_frames_missing", "field_mismatch_count", "packet_metadata_mismatches",
        "endpoint_frames_checked", "native_tcp_flow_count", "native_tcp_preview_count",
        "scoped_session_partitions", "session_partition_scope", "acceptance")
summaries = {}
for name in ("weber", "sharon"):
    directory = root / (name + "-final")
    summary = json.loads((directory / "summary.json").read_text(encoding="utf8"))
    aggregate = {key: summary[key] for key in keys}
    aggregate["bidirectional_session_checks"] = {
        proto: {key: value for key, value in checks.items() if key != "merge_examples"}
        for proto, checks in summary["bidirectional_session_checks"].items()
    }
    differences = json.loads((directory / "field-mismatches.json").read_text(encoding="utf8"))
    aggregate["field_difference_counts"] = dict(collections.Counter(d["field"] for d in differences))
    (out / (name + "-summary.json")).write_text(json.dumps(aggregate, ensure_ascii=False, indent=2)+"\n", encoding="utf8")
    # Reconstruct whitelisted CSV columns from aggregate counts, rather than
    # copying an arbitrary file from a directory containing raw packet bytes.
    with (out / (name + "-protocol-counts.csv")).open("w", encoding="utf-8-sig", newline="") as f:
        w = csv.writer(f)
        w.writerow(["column_label", "wireshark_packets", "yak_packets_same_label", "yak_minus_wireshark"])
        for label in sorted(summary["ws_labels"].keys() | summary["yak_labels"].keys(), key=lambda k: (-summary["ws_labels"].get(k, 0), k)):
            left, right = summary["ws_labels"].get(label, 0), summary["yak_labels"].get(label, 0)
            w.writerow([label, left, right, right-left])
    with (out / (name + "-label-differences.csv")).open("w", encoding="utf-8-sig", newline="") as f:
        w = csv.writer(f)
        w.writerow(["wireshark_label_to_yak_label", "packets"])
        for pair, count in summary["label_pairs"].items():
            left, right = pair.split(" -> ", 1)
            if left != right:
                w.writerow([pair, count])
    summaries[name] = aggregate

tls = json.loads((root / "weber-final" / "tls-record-summary.json").read_text())
tls_summary = {key: tls[key] for key in ("wireshark_complete_records", "native_complete_records",
    "byte_identical_record_occurrences", "wireshark_incomplete_record_views", "wireshark_carrier_record_counts",
    "tcp_record_check", "native_header_fields_checked", "native_header_field_differences", "scope")}
tls_summary["missing_record_occurrences"] = tls["wireshark_complete_records"] - tls["byte_identical_record_occurrences"]
tls_summary["extra_record_occurrences"] = tls["native_complete_records"] - tls["byte_identical_record_occurrences"]
(out / "weber-tls-records.json").write_text(json.dumps(tls_summary, indent=2)+"\n", encoding="utf8")

lines = ["<!-- Generated aggregate evidence; no packet bytes or secrets. -->", "",
    "| 统计口径 | Weber | Sharon fuzz |", "|---|---:|---:|"]
rows = [
    ("输入帧数", lambda s: s["frames"]),
    ("原始 Protocol 列标签种数", lambda s: len(s["ws_labels"])),
    ("捕获长度/线上长度差异", lambda s: s["packet_metadata_mismatches"].get("length",0)+s["packet_metadata_mismatches"].get("captured_length",0)),
    ("TCP stream×捕获域分组", lambda s: s["scoped_session_partitions"]["tcp"]["oracle_stream_domain_pairs"]),
    ("TCP 域内缺关联/拆分分组", lambda s: s["scoped_session_partitions"]["tcp"]["missing_or_split_within_domain"]),
    ("TCP 合并不同 WS stream 的原生分组", lambda s: s["bidirectional_session_checks"]["tcp"]["native_partitions_merging_oracle_streams"]),
    ("UDP stream×捕获域分组", lambda s: s["scoped_session_partitions"]["udp"]["oracle_stream_domain_pairs"]),
    ("UDP 域内缺关联/拆分分组", lambda s: s["scoped_session_partitions"]["udp"]["missing_or_split_within_domain"]),
    ("UDP 合并不同 WS stream 的原生分组", lambda s: s["bidirectional_session_checks"]["udp"]["native_partitions_merging_oracle_streams"]),
    ("字段值集合逐帧差异项", lambda s: s["field_mismatch_count"]),
    ("源地址显示/观察层差异", lambda s: s["packet_metadata_mismatches"].get("source",0)),
    ("目标地址显示/观察层差异", lambda s: s["packet_metadata_mismatches"].get("destination",0)),
]
for label, fn in rows:
    lines.append(f"| {label} | {fn(summaries['weber']):,} | {fn(summaries['sharon']):,} |")
lines += ["", "以下按 Weber 的 Wireshark Protocol 列报文数排序；保留大小写和别名。",
          "Yak 一列只计算完全同名的列标签；不能据此直接判断遗漏或误识别。", "",
          "| 排名 | WS 原始标签 | WS 帧数 | Yak 完全同名帧数 | 差值 |", "|---:|---|---:|---:|---:|"]
for rank, (label, count) in enumerate(list(summaries["weber"]["ws_labels"].items())[:30], 1):
    right = summaries["weber"]["yak_labels"].get(label, 0)
    lines.append(f"| {rank} | {label.replace('|', '&#124;')} | {count:,} | {right:,} | {right-count:+,} |")
(out / "counts.md").write_text("\n".join(lines)+"\n", encoding="utf8")
print(json.dumps({name: {"frames": s["frames"], "labels": len(s["ws_labels"])} for name,s in summaries.items()}))
