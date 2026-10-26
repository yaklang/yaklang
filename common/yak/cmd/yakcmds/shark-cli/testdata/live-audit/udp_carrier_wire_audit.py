"""Literal carrier boundary audit against unmodified TShark JSON -x and SDK JSONL.

Select (udp.length.bad || udp.length.bad_zero || ipv6.hopopts) in original
captures. Offline controls may export all frames instead. Raw carrier offsets,
IP lengths and capture lengths determine malformed versus incomplete; application
labels do not grant admission. Quoted/multiple-IP/reassembled reference TVBs are
reported separately, not silently treated as a physical packet.
"""
from pathlib import Path
import base64, collections, hashlib, ipaddress, json, sys

oracle, events, output = map(Path, sys.argv[1:4])
u = lambda b: int.from_bytes(b, "big")

def sha(path):
    h=hashlib.sha256()
    with path.open("rb") as f:
        for block in iter(lambda:f.read(1<<20),b""): h.update(block)
    return h.hexdigest()

def packets(path):
    # A fuzz-heavy original capture can produce gigabytes of reference JSON.
    # Retain one packet, never the entire reference or SDK field forest.
    decoder=json.JSONDecoder()
    with path.open(encoding="utf-8-sig") as f:
        buffer=""; eof=False; started=False
        while True:
            buffer=buffer.lstrip()
            if not buffer:
                chunk=f.read(1<<20)
                if not chunk:
                    if started: raise ValueError("reference JSON array is incomplete")
                    return
                buffer+=chunk
                continue
            if not started:
                assert buffer[0]=="["
                buffer=buffer[1:];started=True;continue
            if buffer[0]=="]": return
            if buffer[0]==",": buffer=buffer[1:];continue
            try: item,end=decoder.raw_decode(buffer)
            except json.JSONDecodeError:
                chunk=f.read(1<<20)
                if not chunk: raise
                buffer+=chunk;continue
            yield item
            buffer=buffer[end:]

def raw_spans(value):
    if isinstance(value, list) and len(value) >= 3 and isinstance(value[0], str):
        return [value]
    if isinstance(value, list):
        return [s for child in value for s in raw_spans(child)]
    return []

def values(node, name):
    result = []
    if isinstance(node, dict):
        for key, value in node.items():
            if key == name:
                result.extend(value if isinstance(value, list) else [value])
            elif isinstance(value, (dict, list)):
                result.extend(values(value, name))
    elif isinstance(node, list):
        for child in node:
            result.extend(values(child, name))
    return result

by_frame = collections.defaultdict(list)
selected_frames={int(p["_source"]["layers"]["frame"]["frame.number"]) for p in packets(oracle)}
for line in events.open(encoding="utf-8-sig"):
    event = json.loads(line)
    if not (event["transport"] == "udp" and event["status"] not in ["decoded","deferred"] or event["protocol"] in ["dns","icmpv6"]): continue
    for ref in event["source_bytes"].get("PacketRefs") or []:
        if ref["Number"] in selected_frames:
            owned={k:event.get(k) for k in ["transport","source","destination","status","protocol","profile","admission","expert","error","fields"]}
            owned["field_values"]={k:event["field_values"].get(k) for k in ["completeness","incomplete"]}
            by_frame[ref["Number"]].append(owned)

checks = 0
def check(condition, message):
    global checks
    checks += 1
    if not condition:
        raise AssertionError(message)

ledger, unselected = [], []
asserted_frames=set()
outcomes = collections.Counter()
for packet in packets(oracle):
    layers = packet["_source"]["layers"]
    number = int(layers["frame"]["frame.number"])
    physical = bytes.fromhex(layers["frame_raw"][0])
    caplen = int(layers["frame"]["frame.cap_len"])
    wirelen = int(layers["frame"]["frame.len"])
    check(len(physical) == caplen, f"frame {number}: physical capture bytes")
    ipv4 = raw_spans(layers.get("ip_raw"))
    ipv6 = raw_spans(layers.get("ipv6_raw"))
    udp = raw_spans(layers.get("udp_raw"))
    candidates = by_frame[number]
    if len(ipv4) + len(ipv6) != 1 or len(udp) > 1:
        unselected.append({"frame": number, "reason": "multiple/quoted IP or UDP TVBs", "IP_views": len(ipv4)+len(ipv6), "UDP_views": len(udp)})
        continue
    ip = (ipv4 or ipv6)[0]
    offset = int(ip[1])
    version = physical[offset] >> 4
    if version not in [4,6]:
        unselected.append({"frame":number,"reason":"invalid literal IP version; outside valid IP carrier scope","version":version})
        continue
    if version==6 and u(physical[offset+4:offset+6])==0:
        unselected.append({"frame":number,"reason":"zero IPv6 payload length/jumbogram profile unselected"})
        continue
    if version == 4:
        ipend = offset + u(physical[offset+2:offset+4])
        source, destination = physical[offset+12:offset+16], physical[offset+16:offset+20]
    else:
        ipend = offset + 40 + u(physical[offset+4:offset+6])
        source, destination = physical[offset+8:offset+24], physical[offset+24:offset+40]
    source, destination = str(ipaddress.ip_address(source)), str(ipaddress.ip_address(destination))
    if udp:
        start = int(udp[0][1])
        if version==4:
            header=(physical[offset]&15)*4
            if physical[offset+9]!=17 or start!=offset+header:
                unselected.append({"frame":number,"reason":"UDP view belongs to an unselected nested carrier, not this literal IPv4 UDP envelope","IPv4_protocol":physical[offset+9]})
                continue
            if u(physical[offset+6:offset+8])&0x3fff:
                unselected.append({"frame":number,"reason":"physical IP fragment requires independent reassembly proof"})
                continue
        else:
            next_header=physical[offset+6];at=offset+40;selected=True
            for depth in range(32):
                if next_header==17:break
                if at+2>caplen or next_header not in [0,43,60,51,44]:selected=False;break
                size=8 if next_header==44 else (physical[at+1]+(2 if next_header==51 else 1))*(4 if next_header==51 else 8)
                if next_header==44 or at+size>caplen:selected=False;break
                next_header,at=physical[at],at+size
            if not selected or next_header!=17 or at!=start:
                unselected.append({"frame":number,"reason":"UDP view requires an unselected nested/fragment carrier rather than this IPv6 extension chain"})
                continue
        if start+8 > caplen:
            unselected.append({"frame": number, "reason": "physical UDP header unavailable; possible reassembly TVB"})
            continue
        header = physical[start:start+8]
        if bytes.fromhex(udp[0][0])[:8] != header:
            unselected.append({"frame": number, "reason": "UDP TVB differs from physical bytes; reassembly/normalization unselected"})
            continue
        declared, capacity = u(header[4:6]), ipend-start
        source_port, destination_port = u(header[:2]), u(header[2:4])
        check(values(layers,"udp.length") == [str(declared)], f"frame {number}: reference/literal UDP length")
        check(values(layers,"udp.srcport") == [str(source_port)] and values(layers,"udp.dstport") == [str(destination_port)], f"frame {number}: reference/literal UDP ports")
        family="ip" if version==4 else "ipv6"
        check(values(layers,family+".src") == [source] and values(layers,family+".dst") == [destination], f"frame {number}: reference/literal network addresses")
        if declared < 8 or declared > capacity:
            expected, expert = "malformed", "MalformedMessage"
        elif caplen < wirelen or ipend > caplen or start+declared > caplen:
            expected, expert = "incomplete", "NeedMore"
        else:
            expected, expert = None, None
        endpoint = lambda address, port: f"[{address}]:{port}" if ":" in address else f"{address}:{port}"
        candidates = [e for e in candidates if e["transport"] == "udp" and e["source"] == endpoint(source,source_port) and e["destination"] == endpoint(destination,destination_port)]
        if expected:
            check(len(candidates) == 1, f"frame {number}: one UDP refusal, got {len(candidates)}")
            e = candidates[0]
            check(e["status"] == expected, f"frame {number}: {e['status']} != {expected}")
            check(e["expert"] == expert, f"frame {number}: typed carrier cause")
            selected_l2tp = e["protocol"] == "l2tp" and e["profile"] == "l2tp3-bounded-wire-envelope"
            check(not e["protocol"] or selected_l2tp, f"frame {number}: invalid carrier acquired application identity")
            if selected_l2tp: outcomes["selected_L2TPv3_codec_carrier_diagnostic_no_fields"] += 1
            check(bool(e.get("error")), f"frame {number}: diagnostic cause absent")
            check(not e.get("fields"), f"frame {number}: rejected carrier exported application fields")
            check(e["field_values"]["completeness"] == "limited", f"frame {number}: incomplete field coverage")
            check(e["field_values"].get("incomplete"), f"frame {number}: negative field claims are unsafe")
            outcomes[expected+"_UDP_carrier"] += 1
            asserted_frames.add(number)
            ledger.append({"frame": number, "status": expected, "UDP_declared": declared, "IP_UDP_capacity": capacity, "captured_bytes": caplen, "wire_bytes": wirelen, "reference_protocol": layers["frame"].get("frame.protocols")})
        elif values(layers,"dns.id"):
            check(any(e["protocol"] == "dns" and e["status"] == "decoded" for e in candidates), f"frame {number}: complete DNS admission")
            outcomes["complete_DNS_UDP"] += 1
            asserted_frames.add(number)
    if version == 6 and values(layers,"ipv6.hopopts.nxt"):
        types, codes = values(layers,"icmpv6.type"), values(layers,"icmpv6.code")
        if types and codes and ipend <= caplen and caplen == wirelen:
            native = [e for e in by_frame[number] if e["protocol"] == "icmpv6" and e["source"] == source and e["destination"] == destination]
            check(len(native) == 1, f"frame {number}: HBH ICMPv6 event")
            e = native[0]
            check(e["status"] == "decoded", f"frame {number}: HBH ICMPv6 status")
            check(e["fields"]["Type"] == int(types[0]), f"frame {number}: ICMPv6 type")
            check(e["fields"]["Code"] == int(codes[0]), f"frame {number}: ICMPv6 code")
            outcomes["complete_HBH_ICMPv6_selected_base_fields"] += 1
            asserted_frames.add(number)

unasserted=sorted(selected_frames-asserted_frames-{x['frame'] for x in unselected})
result = {"oracle_sha256":sha(oracle),"events_sha256":sha(events),"reference_frames":len(selected_frames),"semantically_asserted_frames":len(asserted_frames),"reference_frames_without_selected_semantic_assertion":unasserted,"checks":checks,"selected_differences":[],"outcomes":dict(outcomes),"UDP_carrier_ledger":ledger,"unselected_reference_views":unselected,"scope":"Literal physical UDP carrier lengths/status/admission and complete HBH ICMPv6 base type/code. This does not certify arbitrary extension options, MLD bodies, checksum validation, jumbograms or all protocol fields. Unselected and semantically unasserted frames stay explicit."}
output.write_text(json.dumps(result,ensure_ascii=False,indent=2)+"\n",encoding="utf-8")
print(json.dumps({k:v for k,v in result.items() if k not in ["UDP_carrier_ledger","unselected_reference_views","reference_frames_without_selected_semantic_assertion"]},ensure_ascii=False))
