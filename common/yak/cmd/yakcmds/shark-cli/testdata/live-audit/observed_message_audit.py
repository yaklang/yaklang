"""Complete BGP/AJP/RTMP messages against full TShark JSON (-2 -n -T json -x).

Inputs and raw output must live outside Git. No port override is applied here;
record explicit AJP Decode-As arguments in the external oracle manifest.
Checks exact payload bytes, occurrence, contributing frame and observed fields.
Does not turn unsupported/fuzz prefixes into complete native messages.
"""
import argparse, base64, collections, hashlib, ipaddress, json
from pathlib import Path

p = argparse.ArgumentParser(description=__doc__)
p.add_argument("protocol", choices=["bgp", "ajp", "rtmp"])
p.add_argument("oracle", type=Path)
p.add_argument("events", type=Path)
p.add_argument("out", type=Path)
a = p.parse_args()
for path in (a.oracle, a.events, a.out):
    if any((d / ".git").exists() for d in [path.resolve(), *path.resolve().parents]):
        raise SystemExit("Raw audit inputs/outputs must stay outside Git")

def sha(w): return hashlib.sha256(w).hexdigest()
def file_sha(path): return sha(path.read_bytes())
def rows(v): return v if isinstance(v, list) else [v]
def occurrences(v, key):
    out = []
    if isinstance(v, dict):
        for k, x in v.items():
            if k == key: out.extend(rows(x))
            elif not k.endswith("_raw"): out.extend(occurrences(x, key))
    elif isinstance(v, list):
        for x in v: out.extend(occurrences(x, key))
    return out
def raw_rows(v):
    if not v: return []
    return [v] if isinstance(v[0], str) else v
def frames(e): return [int(r["Number"]) for r in e["source_bytes"].get("PacketRefs") or []]
def norm(v):
    if isinstance(v, str):
        try: return int(v, 16) if v.startswith("0x") else int(v)
        except ValueError: pass
    return v

events = [json.loads(l) for l in a.events.open(encoding="utf-8-sig") if l.startswith("{")]
events = [e for e in events if e["protocol"] == a.protocol or a.protocol == "rtmp" and e["protocol"] == "amf0"]
if a.protocol == "rtmp": events = [e for e in events if e["profile"] in ("rtmp-message", "rtmp-v3-observed") and e.get("raw")]
oracle = json.loads(a.oracle.read_text(encoding="utf-8-sig"))
index = collections.defaultdict(list)
for e in events:
    wire = base64.b64decode(e.get("raw") or "")
    for frame in frames(e): index[(frame, sha(wire))].append(e)
used, checked, mismatches, missing, ws_count = set(), collections.Counter(), [], [], 0
attribution, repeated_views, display_differences = [], [], []
oracle_prefix_views = []
flow_scope = {}
current_layers = {}

def find(frame, wire, label):
    global ws_count
    ws_count += 1
    choices = [e for e in index[(frame, sha(wire))] if e["id"] not in used]
    scope = (int(current_layers["frame"].get("frame.interface_id", 0)), current_layers.get("tcp", {}).get("tcp.stream"))
    if not choices and scope in flow_scope:
        candidates = [e for e in events if e["flow"] == flow_scope[scope] and sha(base64.b64decode(e.get("raw") or "")) == sha(wire)]
        unused = [e for e in candidates if e["id"] not in used]
        if len(unused) == 1:
            choices = unused
            attribution.append({"oracle_frame":frame,"native_frames":frames(unused[0]),"kind":label})
        elif candidates and not unused:
            repeated_views.append({"frame":frame,"kind":label,"native_pdu":candidates[-1]["id"]})
            return None
    if not choices:
        missing.append({"frame": frame, "kind": label, "bytes": len(wire), "sha256": sha(wire)})
        return None
    e = choices[0]; used.add(e["id"]); flow_scope[scope] = e["flow"]; checked["exact bytes and occurrence"] += 1
    if e["status"] != "decoded": mismatches.append({"frame": frame, "field": "status", "native": e["status"]})
    return e

def compare(frame, obj, key, observed):
    expected = [norm(v) for v in occurrences(obj, key)]
    if not expected: return
    actual = [norm(v) for v in observed]
    checked[key] += len(expected)
    if key == "ajp13.rhost" and expected == [""] and actual == [None]:
        display_differences.append({"frame":frame,"field":key,"reason":"wire null string retained as null; Wireshark displays empty"})
        return
    if expected != actual:
        # Values are summarized by hash to avoid exporting captured application
        # content. Raw evidence remains only in the external oracle/events.
        mismatches.append({"frame": frame, "field": key, "expected_count": len(expected), "native_count": len(actual), "expected_sha256": sha(json.dumps(expected, sort_keys=True).encode()), "native_sha256": sha(json.dumps(actual, sort_keys=True).encode())})

for packet in oracle:
    layers = packet["_source"]["layers"]; frame = int(layers["frame"]["frame.number"])
    current_layers = layers
    proto = {"bgp": "bgp", "ajp": "ajp13", "rtmp": "rtmpt"}[a.protocol]
    objects = rows(layers[proto]) if proto in layers else []
    raws = raw_rows(layers.get(proto + "_raw"))
    for i, obj in enumerate(objects):
        if i >= len(raws):
            missing.append({"frame": frame, "kind": "oracle omitted exact raw message"}); continue
        wire = bytes.fromhex(raws[i][0])
        if a.protocol == "bgp" and (len(wire) < 19 or wire[:16] != b"\xff" * 16 or int.from_bytes(wire[16:18], "big") != len(wire) or wire[18] not in range(1, 6)):
            oracle_prefix_views.append({"frame":frame,"bytes":len(wire),"reason":"no complete supported BGP marker/length/type envelope"})
            continue
        if a.protocol in ("bgp", "ajp"):
            e = find(frame, wire, a.protocol)
            if not e: continue
            f = e.get("session") or {}
            pairs = {"bgp": {"bgp.length":"Length", "bgp.type":"Type", "bgp.open.version":"Version", "bgp.open.myas":"My AS", "bgp.open.holdtime":"Hold Time", "bgp.open.identifier":"BGP Identifier", "bgp.update.withdrawn_routes.length":"Withdrawn Length", "bgp.update.path_attributes.length":"Path Attributes Length"}, "ajp": {"ajp13.len":"Length", "ajp13.code":"Code", "ajp13.method":"Method Code", "ajp13.ver":"Protocol", "ajp13.uri":"URI", "ajp13.raddr":"Remote Address", "ajp13.rhost":"Remote Host", "ajp13.srv":"Server Name", "ajp13.port":"Server Port", "ajp13.sslp":"SSL", "ajp13.nhdr":"Header Count", "ajp13.rstatus":"Status Code", "ajp13.rsmsg":"Status Message", "ajp13.reusep":"Reuse"}}[a.protocol]
            for key, field in pairs.items(): compare(frame, obj, key, [f[field]] if field in f else [])
            if a.protocol == "ajp":
                headers = f.get("Headers") or []
                for name in ("accept", "connection", "content-length", "content-type", "cookie", "host", "location", "referer", "user-agent"):
                    compare(frame, obj, "ajp13." + name.replace("-", "_"), [r["Value"] for r in headers if r.get("Name") == name])
                compare(frame, obj, "ajp13.route", [r["Value"] for r in f.get("Attributes") or [] if r.get("Name") == "Route"])
            if a.protocol == "bgp":
                attrs = f.get("Path Attributes") or []
                for key, field in {"flags":"Flags", "type_code":"Type", "length":"Length", "origin":"Origin", "next_hop":"Next Hop", "multi_exit_disc":"MED", "local_pref":"Local Preference"}.items():
                    compare(frame, obj, "bgp.update.path_attribute." + key, [r[field] for r in attrs if field in r])
                caps = f.get("Capabilities") or []
                for key, field in {"type":"Code", "length":"Length", "4as":"ASN"}.items(): compare(frame, obj, "bgp.cap." + key, [r[field] for r in caps if field in r])
                prefix_groups = [f.get("Withdrawn Routes") or {}] + [r["NLRI"] for r in attrs if "NLRI" in r] + [f.get("NLRI") or {}]
                prefixes = [r for g in prefix_groups for r in g.get("Prefixes") or []]
                compare(frame, obj, "bgp.prefix_length", [r["Prefix Length"] for r in prefixes])
                for version, key in [(4,"bgp.nlri_prefix"),(6,"bgp.mp_reach_nlri_ipv6_prefix")]:
                    compare(frame, obj, key, [str(ipaddress.ip_network(r["Prefix"]).network_address) for r in prefixes if ipaddress.ip_network(r["Prefix"]).version == version])
                for code, name in [(14,"mp_reach_nlri"),(15,"mp_unreach_nlri")]:
                    group = [r for r in attrs if r["Type"] == code]
                    for key, field in [("afi","AFI"),("safi","SAFI")]: compare(frame,obj,"bgp.update.path_attribute."+name+"."+key,[r[field] for r in group])
                    if code == 14:
                        hops = [r.get("Next Hops") or [] for r in group if r.get("AFI") == 2]
                        compare(frame,obj,"bgp.update.path_attribute.mp_reach_nlri.next_hop.ipv6",[r[0] for r in hops if r])
                        compare(frame,obj,"bgp.update.path_attribute.mp_reach_nlri.next_hop.ipv6.link_local",[r[1] for r in hops if len(r)>1])
            continue
        # Handshake S0/S1/S2 may be grouped by TShark into one view; native
        # directional framing deliberately emits initial and echo separately.
        is_handshake = False
        for role in ("c", "s"):
            for initial in (True, False):
                names = [f"rtmpt.handshake.{role}0_raw", f"rtmpt.handshake.{role}1_raw"] if initial else [f"rtmpt.handshake.{role}2_raw"]
                parts = []
                for name in names:
                    def getraw(v):
                        if isinstance(v, dict):
                            if name in v: return raw_rows(v[name])[0][0]
                            for k, x in v.items():
                                if not k.endswith("_raw"):
                                    found = getraw(x)
                                    if found is not None: return found
                        return None
                    parts.append(getraw(obj))
                if all(x is not None for x in parts): find(frame, bytes.fromhex("".join(parts)), "handshake"); is_handshake = True
        if is_handshake: continue
        lengths = occurrences(obj, "rtmpt.header.bodysize")
        if not lengths: continue
        n = int(lengths[0]); payload = wire[-n:] if n else b""
        e = find(frame, payload, "logical message")
        if not e: continue
        f = e.get("session") or {}
        for key, field in {"csid":"Chunk Stream ID", "timestamp":"Timestamp", "bodysize":"Message Length", "typeid":"Message Type", "streamid":"Message Stream ID"}.items(): compare(frame, obj, "rtmpt.header." + key, [f[field]] if field in f else [])
        for key, field in {"rtmpt.scm.chunksize":"Chunk Size", "rtmpt.scm.was":"Bandwidth" if f.get("Message Type") == 6 else "Acknowledgement Window", "rtmpt.scm.limittype":"Limit Type", "rtmpt.ucm.eventtype":"Event Type"}.items(): compare(frame, obj, key, [f[field]] if field in f else [])

extra = [{"id": e["id"], "frames": frames(e), "status": e["status"], "profile": e["profile"], "bytes": len(base64.b64decode(e.get("raw") or ""))} for e in events if e["id"] not in used and (a.protocol != "rtmp" or e["profile"] == "rtmp-message" or (e.get("session") or {}).get("Packet Name") != "Chunk")]
summary = {"protocol": a.protocol, "oracle_sha256": file_sha(a.oracle), "events_sha256": file_sha(a.events), "native_events": len(events), "wireshark_complete_views": ws_count, "matched": len(used), "checks": dict(checked), "mismatches": mismatches, "missing": missing, "native_without_oracle_view": extra, "oracle_prefix_views":oracle_prefix_views, "attribution_differences":attribution,"repeated_oracle_views":repeated_views,"lossless_display_differences":display_differences, "notes": "Unmatched native messages remain explicit; scope interfaces before treating cross-interface TCP duplicate views as decoder errors. RTMP handshake grouping differs; byte identities are checked separately. Cross-frame matching requires the same already-observed interface/TCP-stream/native-flow partition and a unique remaining byte identity; it never borrows across TCP generations. Unsupported/truncated BGP envelopes are listed separately, never counted as complete messages."}
a.out.parent.mkdir(parents=True, exist_ok=True)
a.out.write_text(json.dumps(summary, indent=2, ensure_ascii=False), encoding="utf-8")
print(json.dumps({k: summary[k] for k in ("protocol", "native_events", "wireshark_complete_views", "matched")} | {"field_checks": sum(checked.values()), "mismatches": len(mismatches), "missing": len(missing), "native_without_oracle": len(extra)}))
raise SystemExit(bool(mismatches or missing))
