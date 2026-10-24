"""Compare complete TLS wire records, independently of GUI fragment attribution.

Usage: tls_record_audit.py CAPTURE OUT TSHARK EVENTS_JSONL [KEYLOG]
Stores raw oracle output outside Git, exports only hashes/counts/frame references.
Does not substitute byte equality for plaintext authentication or field equality.
"""
import base64
import collections
import hashlib
import json
from pathlib import Path
import subprocess
import sys

capture, out, tshark, events = map(Path, sys.argv[1:5])
for path in (capture, out):
    if any((p / ".git").exists() for p in [path.resolve(), *path.resolve().parents]):
        raise SystemExit("Capture and outputs must be outside Git")
out.mkdir(exist_ok=True, parents=True)
args = [tshark, "-2", "-n", "-r", capture, "-o", "tcp.reassemble_out_of_order:TRUE"]
if len(sys.argv) > 5:
    args += ["-o", "tls.keylog_file:" + sys.argv[5]]
# Do not use -j/-J protocol projection: Wireshark nests some TLS trees inside
# carrier items (notably RDPUDP). Projection silently drops those records while
# the display-filter still selects their frames, creating a false exact match.
args += ["-Y", "tls.record", "-T", "json", "-x", "--no-duplicate-keys"]
with (out / "tls-records-oracle.json").open("wb") as f, (out / "tls-records-oracle.stderr").open("wb") as err:
    proc = subprocess.run([str(x) for x in args], stdout=f, stderr=err, timeout=600)
assert proc.returncode == 0, proc.returncode

def valid_record(raw):
    return (len(raw) >= 5 and 20 <= raw[0] <= 24 and raw[1] == 3
            and int.from_bytes(raw[3:5], "big") == len(raw) - 5)

def raw_values(value):
    if isinstance(value, list) and value and isinstance(value[0], str):
        yield bytes.fromhex(value[0])
    elif isinstance(value, list):
        for item in value:
            yield from raw_values(item)

def records(tree):
    if isinstance(tree, dict):
        for name, value in tree.items():
            if name == "tls.record_raw":
                yield from raw_values(value)
            elif not name.endswith("_raw"):
                yield from records(value)
    elif isinstance(tree, list):
        for item in tree:
            yield from records(item)

ws = collections.defaultdict(list)
native = collections.defaultdict(list)
ws_tcp = collections.Counter()
native_tcp = collections.Counter()
carriers = collections.Counter()
native_header_fields_checked = collections.Counter()
native_header_field_differences = []
incomplete_ws = 0
oracle = json.loads((out / "tls-records-oracle.json").read_bytes())
for packet in oracle:
    layers = packet["_source"]["layers"]
    # frame.number is exported as an integer by the raw JSON frame tree.
    frame = layers.get("frame", {}).get("frame.number")
    assert frame is not None, "oracle must preserve the original frame number"
    frame = int(frame)
    stack = layers.get("frame", {}).get("frame.protocols", "").split(":")
    carrier = "RDPUDP" if "rdpudp" in stack else "EAP/RADIUS" if "radius" in stack else "TCP" if "tcp" in stack else "other"
    for raw in records(layers):
        if valid_record(raw):
            digest = hashlib.sha256(raw).hexdigest()
            ws[digest].append({"frame": frame, "type": raw[0], "length": len(raw) - 5, "carrier": carrier})
            carriers[carrier] += 1
            if carrier == "TCP":
                ws_tcp[digest] += 1
        else:
            incomplete_ws += 1
with events.open(encoding="utf-8-sig") as f:
    for line in f:
        if not line.startswith("{"):
            continue
        e = json.loads(line)
        if e["protocol"] != "tls":
            continue
        raw = base64.b64decode(e.get("raw") or "")
        if valid_record(raw):
            digest = hashlib.sha256(raw).hexdigest()
            native[digest].append({"frames": [r["Number"] for r in e["source_bytes"].get("PacketRefs") or []], "flow": e["flow"], "source": e["source"], "destination": e["destination"], "type": raw[0], "length": len(raw) - 5, "status": e["status"]})
            if e["transport"] == "tcp":
                native_tcp[digest] += 1
            values = e.get("field_values", {}).get("values", [])
            for name, expected in (("tls.record.version", int.from_bytes(raw[1:3], "big")), ("tls.record.length", len(raw)-5), ("tls.record.wire_type", raw[0])):
                actual = [v["value"] for v in values if v["name"] == name]
                native_header_fields_checked[name] += 1
                if actual != [expected]:
                    native_header_field_differences.append({"pdu": e["id"], "field": name, "expected": expected, "observed": actual})
shared = ws.keys() & native.keys()
summary = {"wireshark_complete_records": sum(map(len, ws.values())),
           "native_complete_records": sum(map(len, native.values())),
           "byte_identical_record_occurrences": sum(min(len(ws[k]), len(native[k])) for k in shared),
           "wireshark_incomplete_record_views": incomplete_ws,
           "wireshark_carrier_record_counts": dict(carriers),
           "tcp_record_check": {"wireshark": sum(ws_tcp.values()), "native": sum(native_tcp.values()), "byte_identical_occurrences": sum((ws_tcp & native_tcp).values()), "missing_occurrences": sum((ws_tcp-native_tcp).values()), "extra_occurrences": sum((native_tcp-ws_tcp).values())},
           "native_header_fields_checked": dict(native_header_fields_checked),
           "native_header_field_differences": native_header_field_differences,
           "wireshark_unmatched_hashes": {k: v for k, v in ws.items() if k not in native},
           "native_unmatched_hashes": {k: v for k, v in native.items() if k not in ws},
           "occurrence_differences": {k: {"wireshark": ws[k], "native": native[k]} for k in shared if len(ws[k]) != len(native[k])},
           "scope": "Complete TLS record bytes only. Flow partitions, decryption and protocol fields remain separate acceptance checks."}
(out / "tls-record-summary.json").write_text(json.dumps(summary, indent=2), encoding="utf-8")
print(json.dumps({k: v for k, v in summary.items() if not isinstance(v, dict)}, indent=2))
