"""Compare normalized transport observations to original-frame TShark raw views.

Captures, native JSONL, the full oracle and output directory must stay outside
Git. Exported review rows contain frame numbers and hashes, never packet bytes.
PIM Null-Register dummy addresses are checked separately from traffic endpoints.
"""
import argparse
import collections
import hashlib
import ipaddress
import json
from pathlib import Path
import subprocess

p = argparse.ArgumentParser(description=__doc__)
for key in ("capture", "yak-jsonl", "tshark", "out"):
    p.add_argument("--" + key, required=True, type=Path)
p.add_argument("--keylog", type=Path)
p.add_argument("--review-pim-null-registers", action="store_true", help="also check bounded PIM Null-Register dummy headers (separate from transport admission)")
a = p.parse_args()
for path in (a.capture, a.yak_jsonl, a.out, a.keylog):
    if path and any((parent / ".git").exists() for parent in [path.resolve(), *path.resolve().parents]):
        raise SystemExit("capture, secrets and audit output must remain outside Git")
a.out.mkdir(parents=True, exist_ok=True)


def sha(path):
    h = hashlib.sha256()
    with path.open("rb") as f:
        for part in iter(lambda: f.read(1 << 20), b""):
            h.update(part)
    return h.hexdigest()


def many(value):
    return value if isinstance(value, list) else [value] if value is not None else []


def last(layers, name):
    return many(layers.get(name))[-1]


def header(layers, name):
    raw = layers[name + "_raw"]
    if not isinstance(raw[0], str):
        raw = raw[-1]
    return bytes.fromhex(raw[0])


def addr(value):
    return str(ipaddress.ip_address(value))


def csum(wire):
    if len(wire) % 2:
        wire += b"\0"
    total = sum(int.from_bytes(wire[i:i + 2], "big") for i in range(0, len(wire), 2))
    while total >> 16:
        total = (total & 65535) + (total >> 16)
    return (~total) & 65535


rows, views = {}, {}
with a.yak_jsonl.open(encoding="utf-8-sig") as f:
    for line in f:
        row = json.loads(line)
        rows[row["number"]] = row
        if row.get("transport_view"):
            views[row["number"]] = row["transport_view"]
if not views:
    raise SystemExit("no native normalized transport observations to verify")
selected = "frame.number in {" + ",".join(map(str, views)) + "}"
args = [a.tshark, "-2", "-n", "-r", a.capture, "-o", "tcp.reassemble_out_of_order:TRUE"]
if a.keylog:
    args += ["-o", "tls.keylog_file:" + str(a.keylog)]
predicate = "(" + selected + ")"
if a.review_pim_null_registers:
    predicate += " || pim.register_flag.null_register == 1"
args += ["-Y", predicate, "-T", "json", "-x", "--no-duplicate-keys"]
oracle = a.out / "oracle.json"
with oracle.open("wb") as out, (a.out / "oracle.stderr").open("wb") as err:
    subprocess.run([str(x) for x in args], stdout=out, stderr=err, timeout=1800, check=True)
checks, missing, failures, reviewed, nulls = collections.Counter(), [], [], [], []
seen = set()
for packet in json.loads(oracle.read_text(encoding="utf-8-sig")):
    layers = packet["_source"]["layers"]
    frame = last(layers, "frame")
    number = int(frame["frame.number"])
    row = rows[number]
    if number in views:
        seen.add(number)
        v = views[number]
        protocol = v["protocol"]
        if protocol not in layers or protocol + "_raw" not in layers:
            missing.append({"frame": number, "reason": "oracle lacks selected transport raw view"})
            continue
        t = last(layers, protocol)
        wire = header(layers, protocol)
        tokens = frame["frame.protocols"].split(":")
        transport_at = len(tokens) - 1 - tokens[::-1].index(protocol)
        family = next(token for token in reversed(tokens[:transport_at]) if token in ("ip", "ipv6"))
        network = last(layers, family)
        expected = {"header_sha256": hashlib.sha256(wire[:v["header_length"]]).hexdigest(),
                    "source": addr(network[family + ".src"]), "destination": addr(network[family + ".dst"]),
                    "source_port": int(t[protocol + ".srcport"]), "destination_port": int(t[protocol + ".dstport"]),
                    "header_length": int(t["tcp.hdr_len"]) if protocol == "tcp" else 8}
        if protocol == "tcp":
            expected.update(sequence=int(t["tcp.seq_raw"]), acknowledgment=int(t["tcp.ack_raw"]),
                            tcp_flags=int(t["tcp.flags"], 0), payload_length=int(t["tcp.len"]))
        for field, want in expected.items():
            got = v.get(field, 0)
            if field in ("source", "destination"):
                got = addr(got)
            checks[field] += 1
            if got != want:
                failures.append({"frame": number, "field": field})
        current_refs = [ref for ref in v["packet_refs"] if ref["Number"] == number]
        checks["current_frame_reference"] += 1
        if not any(ref["Domain"]["Interface"] == int(frame["frame.interface_id"]) and ref["Domain"]["Section"] == int(frame["frame.section_number"]) - 1 for ref in current_refs):
            failures.append({"frame": number, "field": "current_frame_reference"})
        checks["progressive_observed_prefix"] += 1
        stack = row.get("protocol_stack", [])
        if stack[:len(v["layers"])] != v["layers"]:
            failures.append({"frame": number, "field": "progressive_observed_prefix"})
        # The selected header already terminates at TCP/UDP. Appending another
        # inferred IP/transport pair is a presentation bug, even when fields agree.
        checks["no_fabricated_network_suffix"] += 1
        if any(name in ("IPv4", "IPv6", "TCP", "UDP") for name in stack[len(v["layers"]):]):
            failures.append({"frame": number, "field": "no_fabricated_network_suffix"})
        reviewed.append({"frame": number, "protocol": protocol, "header_sha256": v["header_sha256"],
                         "contributing_frames": [ref["Number"] for ref in v["packet_refs"]],
                         "preview_associated": bool(row.get("stream_id")), "layers_complete": v["layers_complete"]})
    if a.review_pim_null_registers and "pim" in layers:
        pim = last(layers, "pim")
        if pim.get("pim.type") != "1":
            continue
        wire = header(layers, "pim")
        if len(wire) < 8 or wire[0] != 0x21 or not wire[4] & 0x40:
            raise AssertionError("filtered Null-Register lacks its complete wire header")
        # RFC 7761 section 4.9.3: these inner addresses name (S,G), they are
        # not a separately transmitted inner datagram or a TCP/UDP connection.
        dummy = wire[8:]
        family = "ip" if dummy[0] >> 4 == 4 else "ipv6"
        ips = many(layers[family])
        outer = ips[0]
        dummy_diagnostic = "none"
        if family == "ip":
            assert len(dummy) >= 20 and dummy[0] == 0x45 and int.from_bytes(dummy[2:4], "big") == 20 and dummy[9] == 103
            if int.from_bytes(dummy[10:12], "big"):
                assert csum(dummy[:20]) == 0
            pseudo = b""
            source, destination = addr(dummy[12:16]), addr(dummy[16:20])
        else:
            assert len(dummy) >= 40 and dummy[0] >> 4 == 6
            if len(dummy) < 44 or dummy[6] != 103:
                dummy_diagnostic = "incomplete/nonconforming IPv6 dummy PIM envelope; addresses only"
            pseudo = ipaddress.ip_address(outer["ipv6.src"]).packed + ipaddress.ip_address(outer["ipv6.dst"]).packed + (8).to_bytes(4, "big") + b"\0\0\0\x67"
            source, destination = addr(dummy[8:24]), addr(dummy[24:40])
        assert csum(pseudo + wire[:8]) == 0
        assert ipaddress.ip_address(destination).is_multicast
        assert addr(row["source"]) == addr(outer[family + ".src"])
        assert addr(row["destination"]) == addr(outer[family + ".dst"])
        assert addr(ips[-1][family + ".src"]) == source and addr(ips[-1][family + ".dst"]) == destination
        assert not row.get("tcp_flow_id") and not row.get("stream_id") and not row.get("transport_view")
        nulls.append({"frame": number, "family": family, "pim_header_sha256": hashlib.sha256(wire[:8]).hexdigest(),
                      "dummy_header_sha256": hashlib.sha256(dummy).hexdigest(), "dummy_diagnostic": dummy_diagnostic,
                      "assessment": "control-plane (S,G), outer traffic endpoints retained; native PIM fields still pending"})
missing += [{"frame": n, "reason": "oracle did not export requested frame"} for n in views.keys() - seen]
result = {"capture_sha256": sha(a.capture), "native_jsonl_sha256": sha(a.yak_jsonl),
          "tshark_sha256": sha(a.tshark), "oracle_sha256": sha(oracle), "script_sha256": sha(Path(__file__)),
          "observations": len(views), "checks": dict(checks), "failures": failures, "missing": missing,
          "review_pim_null_registers": a.review_pim_null_registers,
          "reviewed": reviewed, "pim_null_registers": nulls,
          "scope": "original frame numbers, exact normalized TCP/UDP header bytes and typed fields; TCP payload admission and native PIM control decoding are separate"}
(a.out / "summary.json").write_text(json.dumps(result, indent=2), encoding="utf-8")
print(json.dumps({key: result[key] for key in ("observations", "checks", "failures", "missing")}))
if failures:
    raise SystemExit(1)
