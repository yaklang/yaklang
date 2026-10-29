"""Independent complete-fragment source reconstruction, not native replay.

Grouping is by capture-domain/src/dst/identification, never NextHeader. Every
range is bound back to original physical bytes. Holes, conflicting final size,
overlap or NextHeader disagreement retain uncertainty and do not create a
complete child. Reconstructed offsets are explicitly fragmentable-part offsets,
not fabricated physical frame/PDU offsets.
"""
from pathlib import Path
import argparse, collections, hashlib, importlib.util, json, sys
from scan_source37 import Census

def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()

def main():
    p = argparse.ArgumentParser(description=__doc__)
    for name in ("scan", "reader", "out"):
        p.add_argument("--" + name, type=Path, required=True)
    a = p.parse_args(); a.out.mkdir(exist_ok=False)
    scan = json.loads(a.scan.read_bytes())
    sys.path.insert(0, str(a.reader.parent))
    spec = importlib.util.spec_from_file_location("fragment_original_record_reader", a.reader)
    reader = importlib.util.module_from_spec(spec); spec.loader.exec_module(reader)
    results = []
    for source in scan["captures"]:
        if not source["IPv6_non_atomic_fragments"]:
            continue
        facts = json.loads(Path(source["fragment_file"]).read_bytes())
        selected = {row["frame"] for row in facts}
        records = {index: record for index, record in enumerate(reader.capture_records(Path(source["capture"])), 1) if index in selected}
        assert len(records) == len(selected)
        folder = a.out / ("input-%02d" % source["input"]); folder.mkdir()
        groups = collections.defaultdict(list)
        for row in facts:
            record = records[row["frame"]]
            assert sha_bytes(record["wire"]) == row["frame_wire_SHA256"]
            lo, hi = row["fragment_payload_range"]
            payload = record["wire"][lo:hi]
            assert sha_bytes(payload) == row["payload_SHA256"]
            target = folder / ("frame-%d.bin" % row["frame"])
            target.write_bytes(record["wire"])
            key = tuple(row["domain"]) + (row["ipv6_source"], row["ipv6_destination"], row["fragment"]["identification"])
            groups[key].append((row, payload))
        observations = []
        for index, (key, pieces) in enumerate(groups.items(), 1):
            ordered = sorted(pieces, key=lambda item: (item[0]["fragment"]["fragment_offset"], item[0]["frame"]))
            next_headers = {row["fragment"]["next_header"] for row, _ in pieces}
            final_sizes = {row["fragment"]["fragment_offset"] + len(body) for row, body in pieces if not row["fragment"]["more"]}
            pointer, overlap, holes, ranges, body = 0, False, [], [], bytearray()
            for row, payload in ordered:
                offset = row["fragment"]["fragment_offset"]
                if offset < pointer:
                    overlap = True
                elif offset > pointer:
                    holes.append([pointer, offset])
                if offset == pointer:
                    body.extend(payload)
                pointer = max(pointer, offset + len(payload))
                ranges.append(dict(physical_frame=row["frame"], capture_domain=row["domain"],
                                   source_frame_SHA256=row["frame_wire_SHA256"],
                                   physical_payload_range=row["fragment_payload_range"],
                                   fragmentable_part_range=[offset, offset + len(payload)], payload_SHA256=row["payload_SHA256"]))
            complete = len(next_headers) == len(final_sizes) == 1 and not overlap and not holes and bool(ordered) and ordered[0][0]["fragment"]["fragment_offset"] == 0 and pointer in final_sizes and all(row["parent_complete"] for row, _ in pieces)
            item = dict(group=index, domain=list(key[:2]), ipv6_source=key[2], ipv6_destination=key[3], identification=key[4],
                        physical_frames=[row["frame"] for row, _ in pieces], next_headers=sorted(next_headers), final_sizes=sorted(final_sizes),
                        source_ranges=ranges, holes=holes, overlapping_ranges=overlap, complete_nonoverlapping_source_coverage=complete,
                        epoch_or_sender_authentication_inferred=False)
            if complete:
                assert len(body) == pointer
                target = folder / ("group-%03d-fragmentable-part.bin" % index); target.write_bytes(body)
                counter = collections.Counter()
                virtual_record = dict(section=key[0], interface=key[1], link=101)
                visitor = Census(bytes(body), virtual_record, 0, counter)
                error = None
                try:
                    visitor.upper(next(iter(next_headers)), 0, len(body), [], 0)
                except (AssertionError, ValueError, IndexError) as failed:
                    error = str(failed)
                item.update(reconstructed_fragmentable_part_file=str(target), reconstructed_SHA256=sha(target),
                            reconstructed_length=len(body), reconstruction_coordinate_system="original fragmentable-part offsets; translate via source_ranges, never direct physical offsets",
                            child_counters=dict(counter), source_child_parse_refusal=error,
                            nested_NextHeader89_candidates=visitor.candidates, nested_new_fragments=visitor.fragments)
            observations.append(item)
        target = folder / "groups.json"; target.write_text(json.dumps(observations, indent=2) + "\n", encoding="utf-8")
        results.append(dict(input=source["input"], capture=source["capture"], capture_SHA256=source["capture_SHA256"],
                            fragments=len(facts), source_groups=len(observations), complete_source_groups=sum(row["complete_nonoverlapping_source_coverage"] for row in observations),
                            nested_NextHeader89_candidates=sum(len(row.get("nested_NextHeader89_candidates", [])) for row in observations),
                            group_facts=str(target), group_facts_SHA256=sha(target)))
    result = dict(scope=__doc__, source_scan_SHA256=sha(a.scan), auditor_SHA256=sha(Path(__file__)),
                  sources=results, source_fragments=sum(row["fragments"] for row in results),
                  source_groups=sum(row["source_groups"] for row in results), complete_source_groups=sum(row["complete_source_groups"] for row in results),
                  nested_NextHeader89_candidates=sum(row["nested_NextHeader89_candidates"] for row in results),
                  native_or_future_output_read=False, new_native_expected_object_created=False)
    target = a.out / "summary.json"; target.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(dict(summary=str(target), SHA256=sha(target), fragments=result["source_fragments"],
                          groups=result["source_groups"], complete_groups=result["complete_source_groups"], nested_OSPFv3_candidates=result["nested_NextHeader89_candidates"])))

def sha_bytes(raw):
    return hashlib.sha256(raw).hexdigest()

if __name__ == "__main__":
    main()
