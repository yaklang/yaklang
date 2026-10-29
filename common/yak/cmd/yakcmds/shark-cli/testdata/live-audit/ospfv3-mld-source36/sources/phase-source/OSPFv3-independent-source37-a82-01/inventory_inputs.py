"""Freeze the existing 37 capture inputs and count source link domains only.

The already-closed a82 producer manifest supplies paths/hashes, never decoded
native output. Physical records, sizes, clocks and domains come from the raw
PCAP/PCAPNG source reader. No Go, replay, expected object or packet/key copy.
"""
from pathlib import Path
import argparse, collections, hashlib, importlib.util, json, sys

def sha(path):
    h = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()

def main():
    p = argparse.ArgumentParser(description=__doc__)
    for name in ("closed-manifest", "reader", "out"):
        p.add_argument("--" + name, type=Path, required=True)
    a = p.parse_args()
    a.out.mkdir(exist_ok=False)
    manifest = json.loads(a.closed_manifest.read_bytes())
    assert manifest["source"] == "a82acd0fff97ae50dfc1bbb403c16af359d7bf8f" and manifest["cases"] == 37
    captures = {}
    for record in manifest["outputs"]:
        value = record["capture"]
        if value in captures:
            assert captures[value] == record["capture_SHA256"]
        captures[value] = record["capture_SHA256"]
    assert len(captures) == 37
    sys.path.insert(0, str(a.reader.parent))
    spec = importlib.util.spec_from_file_location("source_capture_reader", a.reader)
    reader = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(reader)
    rows = []
    for index, (name, expected) in enumerate(captures.items(), 1):
        capture = Path(name)
        assert sha(capture) == expected
        counts, domains, short = collections.Counter(), collections.Counter(), 0
        times = []
        frames = 0
        for frames, record in enumerate(reader.capture_records(capture), 1):
            counts[str(record["link"])] += 1
            domains[str((record["section"], record["interface"], record["link"]))] += 1
            short += record["captured_length"] < record["original_length"]
            if not times:
                times = [record["time_ns"], record["time_ns"]]
            else:
                times[0], times[1] = min(times[0], record["time_ns"]), max(times[1], record["time_ns"])
        rows.append(dict(input=index, capture=str(capture), capture_SHA256=expected, bytes=capture.stat().st_size,
                         physical_frames=frames, link_counts=dict(counts), capture_domains=dict(domains),
                         capture_short_records=short, time_ns_range=times))
    result = dict(scope=__doc__, source_manifest=str(a.closed_manifest), source_manifest_SHA256=sha(a.closed_manifest),
                  driver_SHA256=sha(Path(__file__)), capture_reader=str(a.reader), capture_reader_SHA256=sha(a.reader),
                  inputs=rows, input_count=37, total_physical_records=sum(row["physical_frames"] for row in rows),
                  all_capture_SHA_match=True, no_native_events_read=True)
    target = a.out / "inputs.json"
    target.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    links = collections.Counter()
    for row in rows:
        links.update(row["link_counts"])
    print(json.dumps(dict(inventory=str(target), SHA256=sha(target), inputs=37,
                          frames=result["total_physical_records"], links=dict(links))))

if __name__ == "__main__":
    main()
