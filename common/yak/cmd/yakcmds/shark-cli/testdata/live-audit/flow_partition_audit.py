"""Review a flow-admission probe against exact original-frame TShark JSON.

The oracle must include all packets in each reviewed bidirectional tuple and
use the same two-pass/out-of-order preferences as the original whole-file audit.
Input/output remain outside Git; the resulting summary contains only hashes,
frame numbers, counters and explicit scope (no payload or endpoint addresses).
"""
import argparse
import collections
import hashlib
import json
from pathlib import Path

p = argparse.ArgumentParser(description=__doc__)
for name in ("admission", "oracle", "tuples", "out"):
    p.add_argument("--" + name, required=True, type=Path)
a = p.parse_args()
for path in (a.admission, a.oracle, a.tuples, a.out):
    if any((parent / ".git").exists() for parent in [path.resolve(), *path.resolve().parents]):
        raise SystemExit("audit inputs and output must remain outside Git")


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def raw(value):
    return bytes.fromhex(value[0])


admission = json.loads(a.admission.read_text(encoding="utf-8-sig"))
tuples = json.loads(a.tuples.read_text(encoding="utf-8-sig"))
oracle = {int(packet["_source"]["layers"]["frame"]["frame.number"]): packet["_source"]["layers"]
          for packet in json.loads(a.oracle.read_text(encoding="utf-8-sig"))}
reviews = []
for flow, r in admission.items():
    observed = {ref["Number"] for ref in r["observed"]}
    assert observed and all(ref["Domain"]["Interface"] == 226 for ref in r["observed"])
    frames = {int(row["frame.number"]): row for row in tuples[flow]}
    assert observed <= frames.keys() <= oracle.keys()
    chunks, admitted_frames = [], set()
    for chunk in r["admitted"]:
        refs = chunk["packet_refs"]
        assert len(refs) == 1 and refs[0]["Number"] in observed
        n = refs[0]["Number"]
        tcp = oracle[n]["tcp"]
        payload = raw(tcp["tcp.payload_raw"])
        assert chunk["length"] == len(payload) and chunk["sha256"] == hashlib.sha256(payload).hexdigest()
        assert chunk["sequence"] == (int(tcp["tcp.seq_raw"]) + bool(int(tcp["tcp.flags"], 0) & 2)) & 0xffffffff
        admitted_frames.add(n)
        chunks.append({"frame": n, "length": len(payload), "sha256": chunk["sha256"]})
    streams = collections.Counter(frames[n]["tcp.stream"] for n in observed)
    first_stream = frames[min(observed)]["tcp.stream"]
    post_rollover = {n for n in observed if frames[n]["tcp.stream"] != first_stream}
    assert not post_rollover & admitted_frames
    controls = []
    for n, row in frames.items():
        tcp = oracle[n]["tcp"]
        flags = int(tcp["tcp.flags"], 0)
        assert flags == int(row["tcp.flags"], 0)
        if flags & 7:
            controls.append({"frame": n, "flags": flags, "header_sha256": hashlib.sha256(raw(oracle[n]["tcp_raw"])).hexdigest(),
                             "native_observed": n in observed, "payload_admitted": n in admitted_frames,
                             "assessment": "invalid SYN+FIN/RST rejected before tuple lookup" if flags & 2 and flags & 5 else "SYN/ISN observation; conflicting later SYN does not replace active generation"})
    reviews.append({"native_flow": int(flow), "original_interface": 226, "tuple_frames": len(frames),
                    "observed_frames": sorted(observed), "wireshark_stream_packet_counts": dict(streams),
                    "initial_wireshark_stream": first_stream, "post_rollover_observed_frames": sorted(post_rollover),
                    "post_rollover_admitted_frames": [], "admitted_chunks": chunks, "control_frames": controls})
result = {"admission_sha256": sha(a.admission), "oracle_sha256": sha(a.oracle), "tuple_inventory_sha256": sha(a.tuples),
          "script_sha256": sha(Path(__file__)), "reviewed_native_flows": len(reviews),
          "admitted_chunks": sum(len(r["admitted_chunks"]) for r in reviews),
          "admitted_bytes": sum(c["length"] for r in reviews for c in r["admitted_chunks"]),
          "post_rollover_admitted_bytes": 0, "reviews": reviews,
          "scope": "six known fuzz TCP partition differences; observation IDs remain different, but only byte-proven initial chunks were admitted. Does not certify every TCP generation or UDP session model."}
a.out.parent.mkdir(parents=True, exist_ok=True)
a.out.write_text(json.dumps(result, indent=2), encoding="utf-8")
print(json.dumps({k: result[k] for k in ("reviewed_native_flows", "admitted_chunks", "admitted_bytes", "post_rollover_admitted_bytes")}))
