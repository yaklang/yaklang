#!/usr/bin/env python3
"""Offline source control checker. No product runtime is used."""
import argparse
import copy
import hashlib
import ipaddress
import json
from pathlib import Path
import struct

def sha(b):
    return hashlib.sha256(b).hexdigest()

def load(p):
    return json.loads(Path(p).read_text(encoding="utf-8-sig"))

def csum(b):
    if len(b) & 1:
        b += b"\0"
    n = sum(struct.unpack("!"+"H"*(len(b)//2), b))
    while n >> 16:
        n = (n & 65535)+(n >> 16)
    return (~n)&65535

def must(test, message):
    if not test:
        raise AssertionError(message)

def frames(p):
    b = p.read_bytes()
    must(b[:4] == bytes.fromhex("d4c3b2a1"), "PCAP byte order")
    must(struct.unpack("<I", b[20:24])[0] == 1, "DLT")
    pos = 24
    rows = []
    while pos < len(b):
        must(pos+16 <= len(b), "PCAP record header")
        sec, us, cap, orig = struct.unpack("<IIII", b[pos:pos+16])
        pos += 16
        w = b[pos:pos+cap]
        must(len(w) == cap == orig, "authored source is complete")
        rows.append((sec,us,w))
        pos += cap
    must(pos == len(b), "PCAP closed extent")
    return rows

def check_semantic(case, markers, selected_layouts):
    client = [n for n in case["literal_client_names"] if n not in markers]
    server = {n for n in case["literal_server_names"] if n not in markers}
    selected = next(n for n in client if n in server)
    must(selected == case["selected_literal_KEX"], "exact client preference lost")
    must(selected_layouts.get(selected) == case["selected_method_profile"], "unknown name assigned known method profile")
    for step in case["steps"]:
        if "SSH_message" not in step or step["SSH_message"] in (20,21):
            continue
        must(step["selected_literal_KEX"] == selected, "literal selected name")
        if step["selected_method_profile"] is None or step.get("ignored_guess"):
            must(all(x["name"] == "Opaque Method Payload" for x in step["source_fields"]),
                 "unknown/ignored method fabricated a standard body")
            must(not any(k in step for k in ("structural_hostkey_blob_SHA256","structural_hostkey_fingerprint")),
                 "unknown method fabricated host key fingerprint")
    return selected

def verify(plan):
    manifest = load(plan/"manifest.json")
    for row in manifest["files"] + manifest["primary_manifests"]:
        p = Path(row["path"])
        must(sha(p.read_bytes()) == row["SHA256"], "frozen file changed: "+str(p))
        if "bytes" in row:
            must(len(p.read_bytes()) == row["bytes"], "length changed")
    allow = load(plan/"allowlist-and-markers.json")
    for row in allow["selected_complete_layout"]:
        e = row["evidence"]
        must(sha(Path(e["path"]).read_bytes()) == e["SHA256"], "primary hash")
        must(all(x["line_numbers"] for x in e["matches"]), "missing primary string")
    selected_layouts = {r["name"]: r["profile"] for r in allow["selected_complete_layout"]}
    markers = set(allow["marker_names"])
    must(len(markers) == 4, "marker exact set")
    source = load(plan/"source-facts.json")
    pc = frames(Path(manifest["capture"]["path"]))
    checks = {"literal_source_fields": 0, "binary_clear_packets": 0, "banners": 0,
              "opaque_post_NEWKEYS_steps": 0, "physical_frames": len(pc), "checksums": 0}
    used = set()
    allrows = {s["frame"]: (case,s) for case in source["cases"] for s in case["steps"]}
    must(len(allrows) == len(pc), "physical source rows")
    tuples = {}
    for case in source["cases"]:
        selected = check_semantic(case, markers, selected_layouts)
        must(case["TCP_generation_source"]["SYN"] == case["steps"][0]["frame"], "SYN source")
        must(case["steps"][0]["TCP Flags"] == 2, "new connection SYN")
        tuples[case["sourcecase"]] = case["tuple"]
        if case.get("tuple_reuse_of"):
            must(case["tuple"] == tuples[case["tuple_reuse_of"]], "reuse tuple source")
            previous = next(c for c in source["cases"] if c["sourcecase"] == case["tuple_reuse_of"])
            must(case["TCP_generation_source"]["client_ISN"] != previous["TCP_generation_source"]["client_ISN"],
                 "reuse generation ISN")
        for s in case["steps"]:
            _,_,w = pc[s["frame"]-1]
            used.add(s["frame"])
            must(sha(w) == s["wire_SHA256"], "wire SHA")
            ip = w[14:34]
            total = struct.unpack("!H", ip[2:4])[0]
            must(ip[0] == 0x45 and ip[9] == 6, "literal IPv4/TCP")
            must(total+14 == len(w), "IP declared extent")
            must(csum(ip) == 0, "IPv4 checksum")
            tcp = w[34:]
            must(csum(ip[12:20]+b"\0\x06"+struct.pack("!H",len(tcp))+tcp) == 0, "TCP checksum")
            checks["checksums"] += 2
            must(tcp[12] >> 4 == 5, "TCP header")
            p = tcp[20:]
            must(len(p) == s["TCP_payload_length"] and sha(p) == s["payload_SHA256"], "captured payload")
            must(s["physical_TCP_payload_span"] == [54,len(w)], "physical payload span")
            if p:
                must(Path(s["input_path"]).read_bytes() == p, "raw payload path")
            kind = s["kind"]
            if kind == "tcp-control":
                must(not p, "TCP control bytes")
                continue
            if kind == "identification":
                must(p.startswith(b"SSH-2.0-") and p.endswith(b"\r\n"), "identification syntax")
                checks["banners"] += 1
                continue
            if kind == "protected-phase-sentinel":
                must(s.get("must_not_create_plaintext_KEX_or_banner") is True, "protected scope")
                checks["opaque_post_NEWKEYS_steps"] += 1
                continue
            n = struct.unpack("!I", p[:4])[0]
            pad = p[4]
            must(n+4 == len(p) and len(p) % 8 == 0 and len(p) >=16, "clear packet extent/alignment")
            must(pad >=4 and pad <= n-2, "padding bounds")
            end = len(p)-pad
            must(p[5] == s["SSH_message"], "literal SSH message")
            for f in s["source_fields"]:
                start, stop = f["span"]
                must(6 <= start <= stop <= end, "field borrows padding/outside payload")
                raw = p[start:stop]
                if "bytes_SHA256" in f:
                    must(len(raw) == f["byte_length"] and sha(raw) == f["bytes_SHA256"], "field bytes")
                if "ascii" in f:
                    must(raw.decode("ascii") == f["ascii"], "name-list literal")
                if "value" in f:
                    v = f["value"]
                    if isinstance(v,list):
                        must(list(raw) == v, "literal fixed bytes")
                    elif isinstance(v,bool):
                        must(bool(int.from_bytes(raw,"big")) == v, "literal boolean")
                    else:
                        must(int.from_bytes(raw,"big") == v, "literal unsigned")
                checks["literal_source_fields"] += 1
            checks["binary_clear_packets"] += 1
    must(used == set(range(1,len(pc)+1)), "full PCAP coverage")
    # Seven selftests intentionally alter only an independent copy of facts/selection.
    negative = []
    def rejected(label, fn):
        try:
            fn()
        except (AssertionError, StopIteration):
            negative.append(label)
        else:
            raise AssertionError("negative control accepted: "+label)
    base = source["cases"][0]
    modified = copy.deepcopy(base)
    modified["selected_literal_KEX"] = "curve25519-sha256"
    rejected("prefix-marker-stripping-changes-selected-literal",
             lambda: check_semantic(modified, markers, selected_layouts))
    modified = copy.deepcopy(source["cases"][2])
    modified["selected_method_profile"] = "dh"
    rejected("private-dh-prefix-does-not-select-dh",
             lambda: check_semantic(modified, markers, selected_layouts))
    modified = copy.deepcopy(source["cases"][3])
    modified["selected_method_profile"] = "dh-gex"
    rejected("private-gex-prefix-does-not-select-gex",
             lambda: check_semantic(modified, markers, selected_layouts))
    modified = copy.deepcopy(source["cases"][4])
    modified["selected_method_profile"] = "ecdh-curve25519"
    rejected("curve-substring-does-not-select-curve",
             lambda: check_semantic(modified, markers, selected_layouts))
    modified = copy.deepcopy(source["cases"][5])
    reply = next(s for s in modified["steps"] if s.get("SSH_message")==31)
    reply["structural_hostkey_fingerprint"] = "SHA256:invented"
    rejected("unknown-reply-first-string-not-hostkey",
             lambda: check_semantic(modified, markers, selected_layouts))
    modified = copy.deepcopy(source["cases"][-1])
    guessed = next(s for s in modified["steps"] if s.get("ignored_guess"))
    guessed["source_fields"][0]["name"] = "Client Exchange Value"
    rejected("ignored-guess-has-no-standard-method-fields",
             lambda: check_semantic(modified, markers, selected_layouts))
    modified = copy.deepcopy(source["cases"][7])
    modified["selected_method_profile"] = None
    rejected("explicit-libssh-alias-must-remain-supported",
             lambda: check_semantic(modified, markers, selected_layouts))
    return {
        "result": "PASS", "plan_manifest_SHA256": sha((plan/"manifest.json").read_bytes()),
        "checker": {"path": str(Path(__file__).resolve()), "SHA256": sha(Path(__file__).read_bytes())},
        "counts": checks, "cases": len(source["cases"]), "negative_controls": negative,
        "candidate_runtime_read": False,
        "scope": "Independent source construction/selection/framing/checksum selftest, not product correctness or authentication proof",
    }

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--plan", required=True, type=Path)
    ap.add_argument("--out", required=True, type=Path)
    args = ap.parse_args()
    args.out.mkdir(parents=True,exist_ok=False)
    r = verify(args.plan.resolve())
    p = args.out/"summary.json"
    p.write_text(json.dumps(r,indent=2,sort_keys=True)+"\n", encoding="utf-8")
    print(json.dumps({"summary":str(p.resolve()),"SHA256":sha(p.read_bytes()),"counts":r["counts"],"negative_controls":len(r["negative_controls"])}))

if __name__ == "__main__":
    main()

