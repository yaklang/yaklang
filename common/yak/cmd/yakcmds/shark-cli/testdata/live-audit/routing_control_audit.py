"""Exact original-frame routing-control audit. No label normalization or Decode As.

Inputs are a full-file TShark -2 -n -T json -x --no-duplicate-keys oracle,
native SDK JSONL, and the original capture. Captures/raw exports stay outside Git.
"""
import argparse
import base64
import collections
import hashlib
import ipaddress
import json
from pathlib import Path


def sha(path):
    h = hashlib.sha256()
    with path.open("rb") as f:
        for b in iter(lambda: f.read(1 << 20), b""):
            h.update(b)
    return h.hexdigest()


def walk(value):
    if isinstance(value, dict):
        yield value
        for v in value.values():
            yield from walk(v)
    elif isinstance(value, list):
        for v in value:
            yield from walk(v)


def oracle_values(tree, field):
    for row in walk(tree):
        if field in row:
            v = row[field]
            yield from v if isinstance(v, list) else [v]


def canonical(v):
    if isinstance(v, bool):
        return int(v)
    if isinstance(v, (int, float)):
        return int(v)
    if v.startswith("0x") or v.isdecimal():
        return int(v, 16 if v.startswith("0x") else 10)
    try:
        return str(ipaddress.ip_address(v))
    except ValueError:
        return v.lower()


def bfd_rejection_reasons(raw):
    # Independent RFC 5880 section 6.8.6 receive preconditions. These are
    # explicit original-oracle differences, not successful parity checks.
    if len(raw)<24: return ["fewer than 24 BFD control octets"]
    version,flags,mult,length=raw[:4]
    reasons=[]
    if version>>5!=1: reasons.append("outside selected BFDv1 Control version")
    if length<24 or length>len(raw): reasons.append("declared control length outside captured message")
    if not mult: reasons.append("zero Detect Multiplier")
    if not int.from_bytes(raw[4:8],"big"): reasons.append("zero My Discriminator")
    if flags&0x30==0x30: reasons.append("simultaneous Poll and Final")
    return reasons


def audit(oracle, events, capture, out, expected_weak):
    for p in (oracle, events, capture, out):
        if any((x / ".git").exists() for x in [p.resolve(), *p.resolve().parents]):
            raise ValueError("Raw evidence must stay outside Git")
    by_frame = collections.defaultdict(list)
    native = collections.Counter()
    registered = set()
    with events.open(encoding="utf-8-sig") as f:
        for line in f:
            e = json.loads(line)
            if e.get("protocol") not in ("hsrp", "glbp", "bfd"):
                continue
            native[e["protocol"] + "/" + e["status"]] += 1
            if e["status"] != "decoded":
                continue
            for ref in e["source_bytes"].get("PacketRefs") or []:
                by_frame[(ref["Number"], e["protocol"])].append(e)
            registered.update(v["name"] for v in e["field_values"]["values"])
    counts, checks = collections.Counter(), collections.Counter()
    failures, refused, proof, incomplete_views, outside_ingress, rejected_views = [], [], [], [], [], []
    for record in json.loads(oracle.read_text(encoding="utf-8-sig")):
        layers = record["_source"]["layers"]
        frame = int(layers["frame"]["frame.number"])
        for proto in ("hsrp", "glbp", "bfd"):
            if proto not in layers:
                continue
            counts[proto] += 1
            if "udp" not in layers:
                outside_ingress.append({"frame":frame,"protocol":proto,"reason":"No UDP ingress; native routing profile covers UDP control only", "wireshark_stack":layers["frame"]["frame.protocols"]})
                if by_frame[(frame,proto)]: failures.append({"frame":frame,"protocol":proto,"reason":"unexpected UDP native PDU without UDP layer"})
                continue
            if proto + "_raw" not in layers:
                incomplete_views.append({"frame":frame,"protocol":proto,"reason":"Wireshark exposes no complete protocol raw range; preserved as a diagnostic view"})
                if by_frame[(frame,proto)]:
                    failures.append({"frame":frame,"protocol":proto,"reason":"native decoded against incomplete oracle view; investigate independently"})
                continue
            raw = bytes.fromhex(layers[proto + "_raw"][0])
            matches = [e for e in by_frame[(frame, proto)] if base64.b64decode(e["raw"]) == raw]
            if not matches:
                # This tiny context-only layout has no version/sender identity
                # of its own. The exact original frame list stays explicit.
                if proto == "hsrp" and raw[:2] == b"\x02\x04" and len(raw) == 6 and frame in expected_weak:
                    refused.append(frame)
                    continue
                reasons=bfd_rejection_reasons(raw) if proto=="bfd" else []
                if reasons and not by_frame[(frame,proto)]:
                    rejected_views.append({"frame":frame,"protocol":proto,"wire_sha256":hashlib.sha256(raw).hexdigest(),"reasons":reasons})
                    continue
                failures.append({"frame": frame, "protocol": proto, "reason": "no exact native PDU"})
                continue
            if len(matches) != 1:
                failures.append({"frame": frame, "protocol": proto, "reason": "duplicate native PDU"})
                continue
            e = matches[0]
            checks[proto + ".wire"] += 1
            checked = []
            values = collections.defaultdict(list)
            for v in e["field_values"]["values"]:
                values[v["name"]].append(v.get("exact_value", v["value"]))
            relevant = [f for f in registered if f.startswith(proto + ".") or proto == "hsrp" and f.startswith("hsrp2.")]
            for field in relevant:
                want = list(oracle_values(layers[proto], field))
                got = values[field]
                if not want and not got:
                    continue
                checks[field] += 1
                checked.append(field)
                # Repeated TLV fields are an ordered multiset, never a set.
                if list(map(canonical, want)) != list(map(canonical, got)):
                    failures.append({"frame": frame, "protocol": proto, "field": field, "wireshark": want, "yak": got})
            if not e.get("flow"):
                failures.append({"frame": frame, "protocol": proto, "reason": "missing observed context ID"})
            if e["source_bytes"].get("CompletedIn", {}).get("Number") != frame:
                failures.append({"frame": frame, "protocol": proto, "reason": "wrong complete datagram source"})
            proof.append({"frame": frame, "protocol": proto, "wire_sha256": hashlib.sha256(raw).hexdigest(), "checks": len(checked), "flow": e["flow"]})
    if set(refused) != expected_weak:
        failures.append({"reason": "expected refusal inventory changed", "actual": refused, "expected": sorted(expected_weak)})
    result = {"capture_sha256": sha(capture), "oracle_sha256": sha(oracle), "events_sha256": sha(events), "script_sha256": sha(Path(__file__)), "wireshark_messages": dict(counts), "native_events": dict(native), "checks": dict(sorted(checks.items())), "total_checks": sum(checks.values()), "strict_context_refusals": refused, "incomplete_oracle_views":incomplete_views, "outside_udp_ingress":outside_ingress, "rejected_wire_views":rejected_views, "mismatches": failures, "message_proofs": proof, "scope": "Complete original UDP PDUs and selected registered key fields; no authenticated identity, router election or link-health inference. Missing initial sender evidence and malformed inputs remain explicit refusals, not label parity; non-UDP transports remain uncovered."}
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(result, indent=2), encoding="utf-8")
    print(json.dumps({k: v for k, v in result.items() if k not in ("message_proofs", "checks")}, indent=2))
    if failures:
        raise SystemExit(1)


if __name__ == "__main__":
    p = argparse.ArgumentParser(description=__doc__)
    for key in ("oracle", "events", "capture", "out"):
        p.add_argument("--" + key, type=Path, required=True)
    p.add_argument("--expected-weak", default="")
    a = p.parse_args()
    audit(a.oracle, a.events, a.capture, a.out, {int(x) for x in a.expected_weak.split(",") if x})
