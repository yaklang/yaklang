"""Model immutable legacy prefix access and compare only closed a82 output.

No Go or candidate/native output is executed/read. The original-byte 807-frame
source oracle supplies valid LSA extents. Legacy accesses use the pinned old
decoder's visible remaining-packet view, retaining crossed-LSA and hard-index
failures explicitly. Old CLI lacks an exported decode_error bool; its literal
layer/label and incomplete marker are reported without inventing one.
"""
from pathlib import Path
import argparse, collections, hashlib, json, subprocess

PHASE = Path(__file__).resolve().parent.parent
REPO = Path(r"C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang")
DEPENDENCY = Path(r"C:\Users\V\go\pkg\mod\github.com\gopacket\gopacket@v1.3.1\layers\ospf.go")
OLD_SOURCE = "a82acd0fff97ae50dfc1bbb403c16af359d7bf8f"

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def file_sha(path):
    h = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()

def read(path):
    return json.loads(Path(path).read_bytes())

def model(row):
    wire = Path(row["original_frame_file"]).read_bytes()
    assert sha(wire) == row["frame_wire_SHA256"]
    packet_end = row["OSPF_source"]["complete_message"]["physical_range"][1]
    result = dict(frame=row["frame"], frame_wire_SHA256=row["frame_wire_SHA256"],
                  message_type=row["OSPF_source"]["header"]["type"]["value"],
                  deterministic_hard_failure=None, visible_slice_bound_uncertainty=[], semantic_differences=[], LSA_models=[])
    for lsa_index, lsa in enumerate(row["OSPF_source"]["full_LSAs"]):
        start, end = lsa["complete_wire"]["physical_range"]
        view = wire[start:packet_end]
        code = lsa["header_fields"]["LS_type"]["value"]
        assert len(view) >= end - start >= 20
        if code not in (0x2009, 0x0008, 0x4005, 0x2007):
            continue
        trace = []; failure = None
        if code in (0x4005, 0x2007):
            for prefix in lsa["prefixes"]:
                source_bits = prefix["prefix_length"]["value"]
                legacy_bits_field = source_bits // 8
                if legacy_bits_field != source_bits:
                    result["semantic_differences"].append(dict(LSA_index=lsa_index, LS_type=code,
                        kind="ASExternal PrefixLength exposes truncated byte count instead of source bits",
                        source_prefix_bits=source_bits, legacy_PrefixLength_value=legacy_bits_field,
                        source_prefix_length_range=prefix["prefix_length"]["physical_range"]))
            continue
        offset = 32 if code == 0x2009 else 44
        source_prefixes = lsa["prefixes"]
        for index, prefix in enumerate(source_prefixes):
            # data[offset] and data[offset+1] are Go indexing operations:
            # they cannot read beyond visible len even with spare capacity.
            if offset >= len(view):
                failure = dict(LSA_index=lsa_index, prefix_index=index, operation="read prefix length index", legacy_relative_offset=offset,
                               physical_offset=start + offset, visible_decoder_view_length=len(view), visible_decoder_view_range=[start, packet_end],
                               original_LSA_range=[start, end], source_prefix_header=prefix["prefix_length"]["physical_range"], deterministic_index_failure=True)
                break
            bits = view[offset]
            if offset + 1 >= len(view):
                failure = dict(LSA_index=lsa_index, prefix_index=index, operation="read prefix options index", legacy_relative_offset=offset + 1,
                               physical_offset=start + offset + 1, visible_decoder_view_length=len(view), visible_decoder_view_range=[start, packet_end],
                               original_LSA_range=[start, end], deterministic_index_failure=True)
                break
            source_offset = prefix["prefix_length"]["physical_range"][0] - start
            encoded = bits // 8
            slice_start, slice_end = offset + 4, offset + 4 + encoded
            if slice_end > len(view):
                result["visible_slice_bound_uncertainty"].append(dict(LSA_index=lsa_index, prefix_index=index, operation="address prefix slice",
                    requested_range=[start + slice_start, start + slice_end], visible_view_range=[start, packet_end],
                    reason="Go two-index slice also depends on underlying cap; do not call this an index proof"))
            legacy_next = offset + 4 + (bits if code == 0x2009 else encoded)
            entry = dict(prefix_index=index, legacy_header_relative_offset=offset, source_header_relative_offset=source_offset,
                         legacy_length_bits=bits, source_length_bits=prefix["prefix_length"]["value"],
                         legacy_encoded_bytes=encoded, correct_rounded_bytes=prefix["rounded_bytes"],
                         legacy_address_slice_physical_range=[start + slice_start, start + slice_end],
                         source_address_range=prefix["encoded_address_prefix"]["physical_range"],
                         legacy_next_relative_offset=legacy_next, correct_next_relative_offset=prefix["next_logical_PDU_offset"] - lsa["complete_wire"]["logical_PDU_range"][0],
                         crossed_original_LSA_extent=start + slice_end > end)
            trace.append(entry)
            if offset != source_offset or bits != prefix["prefix_length"]["value"] or encoded != prefix["rounded_bytes"]:
                result["semantic_differences"].append(dict(LSA_index=lsa_index, LS_type=code, kind="legacy prefix source/span/rounded width differs", **entry))
            offset = legacy_next
        result["LSA_models"].append(dict(LSA_index=lsa_index, LS_type=code, LSA_range=[start, end], legacy_visible_view_range=[start, packet_end],
                                         source_prefix_count=len(source_prefixes), trace=trace, failure=failure))
        if failure is not None:
            result["deterministic_hard_failure"] = failure
            break
    return result

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--scan", type=Path, required=True)
    p.add_argument("--out", type=Path, required=True)
    a = p.parse_args(); a.out.mkdir(exist_ok=False)
    dependency_bytes = DEPENDENCY.read_bytes()
    old_source03 = read(PHASE / "ESP-OSPFv3-source-readonly30-01/source03/summary.json")
    assert sha(dependency_bytes) == old_source03["dependency_SHA256"]
    assert b"prefixOffset = prefixOffset + 4 + uint32(prefixLen)" in dependency_bytes
    assert b"PrefixLength:      prefixLen," in dependency_bytes
    snapshots = {}
    for relative in ("common/pcapx/pcaputil/config.go", "common/pcapx/pcaputil/reassembly_validation.go", "common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/flow-observer/main.go"):
        raw = subprocess.check_output(["git", "show", OLD_SOURCE + ":" + relative], cwd=REPO)
        snapshots[relative] = dict(source=OLD_SOURCE, SHA256=sha(raw))
    result = []; scan = read(a.scan)
    for source in scan["captures"]:
        if not source["NextHeader89_candidates"]:
            continue
        name = "sharon" if source["input"] == 35 else "weber"
        rows = read(Path(source["source_facts_file"])); expected = {row["frame"]: row for row in rows}
        models = [model(row) for row in rows]
        hard = {row["frame"] for row in models if row["deterministic_hard_failure"]}
        semantics = {row["frame"] for row in models if row["semantic_differences"]}
        cli_root = PHASE / "runtime-public-CLI-35/preservation" / name
        cli_path, cli_receipt = cli_root / "yak.jsonl", read(cli_root / "yak-producer.json")
        assert cli_receipt["source"] == OLD_SOURCE and cli_receipt["exit_code"] == 0
        assert file_sha(cli_path) == cli_receipt["export_SHA256"]
        selected = []; missing_layer, incomplete, decode_key = set(), set(), []
        for line in cli_path.open("rb"):
            item = json.loads(line)
            number = item.get("number")
            if number not in expected:
                continue
            if "OSPF" not in item.get("layers", []):
                missing_layer.add(number)
            if "[incomplete/undecoded]" in item.get("info", ""):
                incomplete.add(number)
            if "decode_error" in item:
                decode_key.append(number)
            selected.append(dict(frame=number, literal_row_SHA256=sha(line), protocol=item.get("protocol"), layers=item.get("layers"),
                                 info=item.get("info"), capture_domain=item.get("capture_domain"), exported_decode_error_key="decode_error" in item))
        assert len(selected) == len(rows) and {item["frame"] for item in selected} == set(expected)
        assert hard == missing_layer == incomplete, (name, "model and old actual label/incomplete sets differ", sorted(hard ^ missing_layer), sorted(hard ^ incomplete))
        flow_root = PHASE / "runtime-public-native-35/preservation" / name
        flow_path, flow_receipt = flow_root / "flows.json", read(flow_root / "flows-producer.json")
        assert flow_receipt["source"] == OLD_SOURCE and flow_receipt["exit_code"] == 0
        assert file_sha(flow_path) == flow_receipt["export_SHA256"]
        flow = read(flow_path); before = flow["stats"]["DecodeErrors"]
        assert flow["capture_SHA256"] == source["capture_SHA256"]
        delta = dict(before_DecodeErrors=before, independently_attributable_legacy_OSPFv3_failures=len(hard),
                     conditional_expected_DecodeErrors_after_shared_capture_observer=before - len(hard),
                     condition="All source-valid OSPFv3 legacy errors are successfully observed/consumed before existing malformedPacket counter, and no unrelated decoder behavior changes",
                     complete_flow_object_not_claimed_identical=True, no_future_counter_observation_used=True)
        folder = a.out / name; folder.mkdir()
        for file, value in (("legacy-access-source-model.json", models), ("old-CLI-target-metadata.json", selected)):
            (folder / file).write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")
        value = dict(source=name, capture=source["capture"], capture_SHA256=source["capture_SHA256"], source_frames=len(rows),
                     legacy_failure_frames=sorted(hard), legacy_failure_count=len(hard), old_CLI_non_OSPF_layer_frames=sorted(missing_layer),
                     old_CLI_incomplete_marker_frames=sorted(incomplete), old_CLI_exported_decode_error_key_frames=decode_key,
                     actual_decode_error_bool_not_exported=not decode_key, literal_label_counts=dict(collections.Counter(item["protocol"] for item in selected)),
                     failure_reason_counts=dict(collections.Counter(row["deterministic_hard_failure"]["operation"] for row in models if row["deterministic_hard_failure"])),
                     semantic_prefix_difference_frames=sorted(semantics), semantic_prefix_difference_without_hard_error_frames=sorted(semantics - hard),
                     slice_cap_uncertainty_frames=[row["frame"] for row in models if row["visible_slice_bound_uncertainty"]],
                     old_CLI_file=str(cli_path), old_CLI_SHA256=file_sha(cli_path), old_CLI_receipt_SHA256=file_sha(cli_root / "yak-producer.json"),
                     old_flow_file=str(flow_path), old_flow_SHA256=file_sha(flow_path), old_flow_receipt_SHA256=file_sha(flow_root / "flows-producer.json"),
                     existing_stats=flow["stats"], decoder_counter_source_delta=delta,
                     outputs={file: file_sha(folder / file) for file in ("legacy-access-source-model.json", "old-CLI-target-metadata.json")})
        (folder / "summary.json").write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")
        result.append(value)
    summary = dict(scope=__doc__, auditor_SHA256=file_sha(Path(__file__)), source_scan_SHA256=file_sha(a.scan),
                   immutable_gopacket_source=str(DEPENDENCY), immutable_gopacket_source_SHA256=sha(dependency_bytes), immutable_a82_source_bindings=snapshots,
                   sources=result, exact_actual_source_frames=sum(row["source_frames"] for row in result),
                   deterministic_legacy_error_frames=sum(row["legacy_failure_count"] for row in result),
                   no_Go_no_candidate_or_future_output=True, no_old_plan_or_output_modified=True)
    target = a.out / "summary.json"; target.write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(dict(summary=str(target), SHA256=file_sha(target),
                          sources=[dict(name=row["source"], failures=row["legacy_failure_count"],
                                        semantic_without_error=len(row["semantic_prefix_difference_without_hard_error_frames"]),
                                        before=row["decoder_counter_source_delta"]["before_DecodeErrors"],
                                        conditional_after=row["decoder_counter_source_delta"]["conditional_expected_DecodeErrors_after_shared_capture_observer"]) for row in result])))

if __name__ == "__main__":
    main()
