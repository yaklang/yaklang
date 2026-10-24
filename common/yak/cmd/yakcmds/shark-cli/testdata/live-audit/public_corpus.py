"""Whole-file differential audit. Captures, secrets and outputs MUST stay outside git.

python public_corpus.py --capture FILE --out DIR --tshark EXE --yak EXE
                       [--events EXE] [--keylog FILE] [--reuse-oracle]

No network replay, Decode As overrides, packet stripping or protocol count
normalization. CSV preserves every original frame number. Column labels,
protocol presence, UDP tuples and TCP session partitions are distinct metrics.
"""
import argparse
import collections
import csv
import hashlib
import io
import ipaddress
import json
from pathlib import Path
import subprocess

csv.field_size_limit(32 << 20)
p = argparse.ArgumentParser(description=__doc__)
for key in ("capture", "out", "tshark", "yak"):
    p.add_argument("--" + key, required=True, type=Path)
p.add_argument("--events", type=Path)
p.add_argument("--keylog", type=Path)
p.add_argument("--reuse-oracle", action="store_true")
p.add_argument("--oracle-only", action="store_true")
a = p.parse_args()
for path in (a.capture, a.out, a.keylog):
    if path and any((parent / ".git").exists() for parent in [path.resolve(), *path.resolve().parents]):
        raise SystemExit("Capture, secrets and output directories must be outside any Git checkout")
a.out.mkdir(parents=True, exist_ok=True)

def sha(path):
    h = hashlib.sha256()
    with path.open("rb") as f:
        for block in iter(lambda: f.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()

def run(args, name):
    with (a.out / name).open("wb") as out, (a.out / (name + ".stderr")).open("wb") as err:
        proc = subprocess.run([str(x) for x in args], stdout=out, stderr=err, timeout=1800)
    if proc.returncode:
        raise RuntimeError(f"{name}: exit {proc.returncode}; see saved stderr")
    return a.out / name

# Context fields such as selected suite/SNI are checked at the handshake where
# Wireshark exposes them, not compared to an absent field in every later record.
field_types = {
    "dns.id": "int", "dns.qry.name": "name", "dns.qry.type": "int",
    "tls.record.version": "int", "tls.record.length": "int",
    "tls.handshake.extensions_server_name": "name", "tls.handshake.ciphersuite": "int",
    "http.request.method": "str", "http.response.code": "int", "http2.streamid": "int",
    "ssh.protocol": "str", "ssh.message_code": "int",
    "isakmp.version": "int", "isakmp.exchangetype": "int", "isakmp.flags": "int",
    "isakmp.messageid": "int", "isakmp.length": "int",
    "rip.command": "int", "rip.version": "int", "rip.metric": "int",
    "nbss.type": "int", "nbss.length": "int", "l2tp.flags": "int", "l2tp.length": "int",
    "rpc.xid": "int", "rpc.msgtyp": "int", "rpc.program": "int", "rpc.procedure": "int",
}
fields = ["frame.number", "frame.interface_id", "frame.encap_type", "frame.len", "frame.cap_len",
          "frame.protocols", "_ws.col.Protocol", "_ws.col.Info", "ip.src", "ip.dst", "ipv6.src", "ipv6.dst",
          "tcp.srcport", "tcp.dstport", "udp.srcport", "udp.dstport", "tcp.stream", "udp.stream",
          "tcp.seq_raw", "tcp.ack_raw", "tcp.len", "tcp.flags", "tls.record.content_type",
          "quic.dcid", "quic.scid", "quic.packet_number", "_ws.expert.message", *field_types]
manifest = {"capture_sha256": sha(a.capture), "capture_bytes": a.capture.stat().st_size,
            "tshark_sha256": sha(a.tshark), "fields": fields, "two_pass": True,
            "out_of_order_reassembly": True, "keylog_sha256": sha(a.keylog) if a.keylog else None}
manifest_path = a.out / "oracle-manifest.json"
if not a.reuse_oracle or not manifest_path.exists() or json.loads(manifest_path.read_text()) != manifest:
    args = [a.tshark, "-2", "-n", "-r", a.capture, "-o", "tcp.reassemble_out_of_order:TRUE"]
    if a.keylog:
        args += ["-o", "tls.keylog_file:" + str(a.keylog)]
    args += ["-T", "fields", "-E", "header=y", "-E", "occurrence=a", "-E", "aggregator=|"]
    for field in fields:
        args += ["-e", field]
    run(args, "tshark.tsv")
    manifest_path.write_text(json.dumps(manifest, indent=2), encoding="utf-8")
print("Wireshark complete", flush=True)
if a.oracle_only:
    raise SystemExit(0)
args = [a.yak, "shark", "-r", a.capture, "--json", "--ignore-embedded-secrets"]
if a.keylog:
    args += ["--tls-keylog", a.keylog]
run(args, "yak.jsonl")
print("Yak CLI complete", flush=True)
if a.events:
    run([a.events, "replay", a.capture, a.keylog or "-", "--ignore-embedded-secrets"], "events.jsonl")
    print("Yak SDK complete", flush=True)

with (a.out / "tshark.tsv").open(encoding="utf-8-sig") as f:
    ws = list(csv.DictReader(f, delimiter="\t", quoting=csv.QUOTE_NONE))
with (a.out / "yak.jsonl").open(encoding="utf-8-sig") as f:
    ys = [json.loads(line) for line in f if line.startswith("{")]
assert len(ws) == len(ys), ("packet count", len(ws), len(ys))
events = {}
all_events = {}
if a.events:
    with (a.out / "events.jsonl").open(encoding="utf-8-sig") as f:
        for line in f:
            if not line.startswith("{"):
                continue
            e = json.loads(line)
            all_events[e["id"]] = e
            for ref in e["source_bytes"].get("PacketRefs") or []:
                events.setdefault(ref["Number"], {})[e["id"]] = e
labels = collections.Counter()
presence_ws = collections.Counter()
presence_yak = collections.Counter()
native_status = collections.Counter()
for e in all_events.values():
    native_status[e["protocol"] + "/" + e["status"]] += 1
field_checks = collections.Counter()
field_missing = collections.Counter()
field_mismatches = []
packet_mismatches = collections.Counter()
partitions = {"tcp": collections.defaultdict(set), "udp": collections.defaultdict(set)}
partition_domains = {"tcp": collections.defaultdict(lambda: collections.defaultdict(set)), "udp": collections.defaultdict(lambda: collections.defaultdict(set))}
reverse_partitions = {"tcp": collections.defaultdict(set), "udp": collections.defaultdict(set)}
payload_partitions = {"tcp": collections.defaultdict(set), "udp": collections.defaultdict(set)}
endpoint_checks = collections.Counter()
split = lambda value: [x for x in value.split("|") if x]
canon = lambda v, t: int(str(v), 0) if t == "int" and str(v).startswith("0x") else int(v) if t == "int" else str(v).lower().rstrip(".") if t == "name" else str(v)
with (a.out / "frame-comparison.csv").open("w", encoding="utf-8-sig", newline="") as f:
    w = csv.writer(f)
    w.writerow(["frame", "ws_protocol", "yak_protocol", "equal_label", "ws_stack", "yak_stack", "ws_tcp_session", "yak_tcp_flow_id", "yak_preview_id", "ws_udp_tuple", "yak_udp_tuple", "yak_application_conversation", "ws_info", "yak_info", "native_outcomes"])
    for x, y in zip(ws, ys):
        n = int(x["frame.number"])
        assert n == y["number"]
        label = x["_ws.col.protocol"]
        labels[label + " -> " + y["protocol"]] += 1
        ev = list(events.get(n, {}).values())
        wsp = set(x["frame.protocols"].split(":"))
        yp = set(e["protocol"] for e in ev if e["status"] in ("decoded", "deferred"))
        presence_ws.update(wsp)
        presence_yak.update(yp)
        for key, expected in [("length", int(x["frame.len"])), ("captured_length", int(x["frame.cap_len"]))]:
            if y[key] != expected:
                packet_mismatches[key] += 1
        domain = json.dumps(y.get("tcp_flow_domain") if y.get("tcp_flow_id") else y.get("message_domain") or y.get("capture_domain", {}), sort_keys=True)
        tuple_parts = sorted([(y["source"], y.get("source_port", 0)), (y["destination"], y.get("destination_port", 0))])
        udp_tuple = hashlib.sha256(json.dumps([domain, tuple_parts]).encode()).hexdigest()[:16] if y.get("transport") == "udp" else "0"
        # UDP .stream is a carrier tuple, while conversation_id can identify a
        # QUIC CID session or a logical protocol exchange. Never equate them.
        tokens = x["frame.protocols"].split(":")
        cutoff = next((i for i,t in enumerate(tokens) if t in ("icmp", "icmpv6")), len(tokens))
        outer_tokens = tokens[:cutoff] # Quoted ICMP payload is not another flow.
        for protocol in ("tcp", "udp"):
            if protocol not in outer_tokens and x["frame.protocols"]:
                continue
            stream = split(x[protocol + ".stream"])
            stream = stream[-1] if stream else ""
            native = y.get("tcp_flow_id", 0) if protocol == "tcp" else udp_tuple
            if stream:
                partitions[protocol][stream].add(str(native))
                partition_domains[protocol][stream][domain].add(str(native))
                if str(native) != "0":
                    reverse_partitions[protocol][(domain, str(native))].add(stream)
                # Pure ACKs/orphan control packets need not allocate parser
                # state. Report their association separately, without hiding
                # splits/merges in actual application-bearing segments.
                sizes = split(x["tcp.len"]) if protocol == "tcp" else ["1"]
                if any(int(v) > 0 for v in sizes):
                    payload_partitions[protocol][(stream, domain)].add(str(native))
        ip_kinds = [t for t in outer_tokens if t in ("ip", "ipv6")]
        if ip_kinds:
            prefix = ip_kinds[-1]
            for name, suffix in [("source", "src"), ("destination", "dst")]:
                expected = split(x.get(prefix + "." + suffix, ""))
                if expected:
                    index = max(0, outer_tokens.count(prefix)-1)
                    if index >= len(expected):
                        endpoint_checks[name + "_oracle_unavailable"] += 1
                        continue # A malformed nested IP tree has no address.
                    endpoint_checks[name] += 1
                    try:
                        equal = ipaddress.ip_address(y[name]) == ipaddress.ip_address(expected[index])
                    except ValueError:
                        equal = False
                    if not equal:
                        packet_mismatches[name] += 1
        w.writerow([n, label, y["protocol"], label == y["protocol"], x["frame.protocols"], " > ".join(y.get("protocol_stack", [])), x["tcp.stream"], y.get("tcp_flow_id", 0), y.get("stream_id", 0), x["udp.stream"], udp_tuple, y.get("conversation_id", 0), x["_ws.col.info"], y["info"], ";".join(e["protocol"] + "/" + e["status"] for e in ev)])
        snapshots = [e.get("field_values", {}) for e in ev]
        if not snapshots:
            snapshots = [e.get("field_values", {}) for e in y.get("pdus", [])]
        for field, typ in field_types.items():
            expected = split(x.get(field, ""))
            if not expected:
                continue
            observed = [v.get("exact_value", v["value"]) for s in snapshots for v in s.get("values", []) if v["name"] == field]
            if not observed:
                field_missing[field] += 1
                continue
            want = set(canon(v, typ) for v in expected)
            got = set(canon(v, typ) for v in observed)
            field_checks[field] += 1
            # One frame can contribute to a larger reassembled PDU; compare
            # observed values against the actual contributor's oracle fields.
            if want != got:
                field_mismatches.append({"frame": n, "field": field, "wireshark": sorted(want), "yak": sorted(got)})
with (a.out / "field-mismatches.json").open("w", encoding="utf-8") as f:
    json.dump(field_mismatches, f, indent=2)
summary = {"frames": len(ws), "manifest": manifest, "yak_sha256": sha(a.yak),
           "events_binary_sha256": sha(a.events) if a.events else None,
           "audit_script_sha256": sha(Path(__file__)),
           "ws_labels": dict(collections.Counter(x["_ws.col.protocol"] for x in ws).most_common()),
           "yak_labels": dict(collections.Counter(y["protocol"] for y in ys).most_common()),
           "label_pairs": dict(labels.most_common()), "ws_protocol_presence": dict(presence_ws.most_common()),
           "native_protocol_presence": dict(presence_yak.most_common()), "native_event_status": dict(native_status.most_common()), "events_without_packet_references": sum(not (e["source_bytes"].get("PacketRefs") or []) for e in all_events.values()),
           "field_frames_checked": dict(field_checks), "field_frames_missing": dict(field_missing),
           "field_mismatch_count": len(field_mismatches), "packet_metadata_mismatches": dict(packet_mismatches), "endpoint_frames_checked": dict(endpoint_checks),
           "native_tcp_flow_count": len({y.get("tcp_flow_id") for y in ys if y.get("tcp_flow_id")}),
           "native_tcp_preview_count": len({y.get("stream_id") for y in ys if y.get("stream_id")}),
           "session_partition_scope": "Native TCP core flow IDs (preview IDs are separate) and UDP endpoint tuples; application sessions exported separately. Capture domains may intentionally split a Wireshark stream.",
           "scoped_session_partitions": {protocol: {"oracle_stream_domain_pairs": sum(map(len, parts.values())), "missing_or_split_within_domain": sum(len(ids) != 1 or "0" in ids for domains in parts.values() for ids in domains.values())} for protocol, parts in partition_domains.items()},
           "bidirectional_session_checks": {protocol: {
               "native_partitions_merging_oracle_streams": sum(len(v) > 1 for v in reverse_partitions[protocol].values()),
               "merge_examples": [{"domain": k[0], "native": k[1], "wireshark_streams": sorted(v)} for k,v in reverse_partitions[protocol].items() if len(v) > 1][:30],
               "payload_stream_domain_pairs": len(payload_partitions[protocol]),
               "payload_pairs_missing_or_split": sum(len(v) != 1 or "0" in v for v in payload_partitions[protocol].values())
           } for protocol in partitions},
           "session_partitions": {protocol: {"wireshark_sessions": len(parts), "one_native_session": sum(len(v) == 1 and "0" not in v for v in parts.values()), "missing_or_split": {k: sorted(v) for k, v in parts.items() if len(v) != 1 or "0" in v}} for protocol, parts in partitions.items()},
           "acceptance": "pending: label, coverage, field, reassembly and session differences require investigation"}
(a.out / "summary.json").write_text(json.dumps(summary, indent=2), encoding="utf-8")
with (a.out / "protocol-counts.csv").open("w", encoding="utf-8-sig", newline="") as f:
    writer = csv.writer(f)
    writer.writerow(["column_label", "wireshark_packets", "yak_packets_same_label", "yak_minus_wireshark", "comparison_scope"])
    for label in sorted(summary["ws_labels"].keys() | summary["yak_labels"].keys(), key=lambda label: (-summary["ws_labels"].get(label, 0), label)):
        left, right = summary["ws_labels"].get(label, 0), summary["yak_labels"].get(label, 0)
        writer.writerow([label, left, right, right-left, "raw column labels; aliases, carriers, diagnostics and PDU reassembly require separate review"])
with (a.out / "label-differences.csv").open("w", encoding="utf-8-sig", newline="") as f:
    writer = csv.writer(f)
    writer.writerow(["wireshark_label_to_yak_label", "packets"])
    for pair, count in labels.most_common():
        left, right = pair.split(" -> ", 1)
        if left != right:
            writer.writerow([pair, count])

print(json.dumps({"frames": len(ws), "ws_labels": summary["ws_labels"], "yak_labels": summary["yak_labels"], "field_mismatch_count": len(field_mismatches), "field_frames_checked": dict(field_checks)}, indent=2), flush=True)
