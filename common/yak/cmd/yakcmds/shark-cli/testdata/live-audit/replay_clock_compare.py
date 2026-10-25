"""Compare complete replay-clock probe exports without normalizing flow IDs.

Both inputs must come from the same original capture and binary. The delayed
run adds processing latency, not packet loss or timestamp edits. Summary output
contains only counters, hashes and differing flow numbers; payloads stay absent.
"""
import argparse
import collections
import hashlib
import json
from pathlib import Path

p = argparse.ArgumentParser(description=__doc__)
for name in ("capture", "binary", "baseline", "delayed", "out"):
    p.add_argument("--" + name, required=True, type=Path)
a = p.parse_args()
for path in (a.capture, a.binary, a.baseline, a.delayed, a.out):
    if any((parent / ".git").exists() for parent in [path.resolve(), *path.resolve().parents]):
        raise SystemExit("capture, binaries and audit outputs must remain outside Git")


def sha(path):
    h = hashlib.sha256()
    with path.open("rb") as f:
        for block in iter(lambda: f.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def load(path):
    associations, chunks = collections.defaultdict(list), collections.defaultdict(list)
    admitted_bytes = 0
    with path.open(encoding="utf-8-sig") as f:
        for line in f:
            row = json.loads(line)
            flow = row["flow"]
            assert flow > 0 and row["packet_refs"]
            if row["kind"] == "association":
                associations[flow].append(row["packet_refs"])
            elif row["kind"] == "admitted":
                assert row["length"] > 0 and len(row["sha256"]) == 64
                chunks[flow].append(row)
                admitted_bytes += row["length"]
            else:
                raise AssertionError("unknown probe record")
    assert associations and chunks.keys() <= associations.keys()
    fingerprint = {"associations": dict(associations), "chunks": dict(chunks)}
    return {"flows": len(associations), "association_callbacks": sum(map(len, associations.values())),
            "admitted_chunks": sum(map(len, chunks.values())), "admitted_bytes": admitted_bytes,
            "associations_sha256": digest(fingerprint["associations"]), "chunks_sha256": digest(fingerprint["chunks"])}, fingerprint


baseline, left = load(a.baseline)
delayed, right = load(a.delayed)
different_associations = sorted(flow for flow in left["associations"].keys() | right["associations"].keys()
                                if left["associations"].get(flow) != right["associations"].get(flow))
different_chunks = sorted(flow for flow in left["chunks"].keys() | right["chunks"].keys()
                          if left["chunks"].get(flow) != right["chunks"].get(flow))
result = {"capture_sha256": sha(a.capture), "binary_sha256": sha(a.binary), "script_sha256": sha(Path(__file__)),
          "baseline_jsonl_sha256": sha(a.baseline), "delayed_jsonl_sha256": sha(a.delayed),
          "baseline": baseline, "delayed": delayed,
          "different_association_flow_ids": different_associations, "different_chunk_flow_ids": different_chunks,
          "scope": "complete original-file native TCP observation IDs, exact contributor/domain associations and ordered admitted chunk hashes/sequence/direction; no flow-ID normalization, no UDP or full application-field equivalence claim"}
a.out.parent.mkdir(parents=True, exist_ok=True)
a.out.write_text(json.dumps(result, indent=2), encoding="utf-8")
print(json.dumps(result, indent=2))
if different_associations or different_chunks:
    raise SystemExit(1)
