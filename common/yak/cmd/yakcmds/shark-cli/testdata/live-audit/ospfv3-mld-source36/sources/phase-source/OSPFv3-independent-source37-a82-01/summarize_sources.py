"""Counts/RFC applicability and immutable-a82 canonical source review only.

Uses the independent source and reference certificates. No new native output,
expected field tree, capture, key or existing proof is modified. Occurrences
are wire observations, not inferred unique LSDB/neighbor/session identities.
"""
from pathlib import Path
import argparse, collections, csv, hashlib, json, subprocess

REPO = Path(r"C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang")
A82 = "a82acd0fff97ae50dfc1bbb403c16af359d7bf8f"

def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()

def counts(values):
    return dict(collections.Counter(str(value) for value in values))

def main():
    p = argparse.ArgumentParser(description=__doc__)
    for name in ("scan", "reference-proof", "fragments", "out"):
        p.add_argument("--" + name, type=Path, required=True)
    a = p.parse_args(); a.out.mkdir(exist_ok=False)
    scan, reference, fragments = (json.loads(path.read_bytes()) for path in (a.scan, a.reference_proof, a.fragments))
    assert reference["checked_OSPFv3_frames"] == 807 and fragments["nested_NextHeader89_candidates"] == 0
    rows, sources = [], []
    for case in scan["captures"]:
        facts = json.loads(Path(case["source_facts_file"]).read_bytes())
        assert sha(Path(case["source_facts_file"])) == case["source_facts_SHA256"]
        lsas = [lsa for row in facts for lsa in row["OSPF_source"]["full_LSAs"]]
        prefixes = [prefix for lsa in lsas for prefix in lsa["prefixes"]]
        rows.append(dict(input=case["input"], capture=case["capture"], capture_SHA256=case["capture_SHA256"], physical_frames=case["physical_frames"],
                         IPv6_source_headers=case["counters"].get("IPv6_source_headers", 0), OSPFv3=len(facts),
                         Hello=sum(row["OSPF_source"]["header"]["type"]["value"] == 1 for row in facts),
                         Database_Description=sum(row["OSPF_source"]["header"]["type"]["value"] == 2 for row in facts),
                         Link_State_Request=sum(row["OSPF_source"]["header"]["type"]["value"] == 3 for row in facts),
                         Link_State_Update=sum(row["OSPF_source"]["header"]["type"]["value"] == 4 for row in facts),
                         Link_State_Acknowledgment=sum(row["OSPF_source"]["header"]["type"]["value"] == 5 for row in facts),
                         AH=sum(any(ext["kind"] == "AH" for ext in row["ingress"]["extensions"]) for row in facts),
                         no_extension=sum(not row["ingress"]["extensions"] for row in facts), DLT274=sum(row["link"] == 274 for row in facts),
                         source_truncated_OSPF=sum(not row["ingress"]["parent_complete"] for row in facts), OSPF_reassembled=0,
                         non_atomic_IPv6_fragments=case["IPv6_non_atomic_fragments"], full_LSA_occurrences=len(lsas), prefix_occurrences=len(prefixes)))
        if not facts:
            continue
        headers = [(row["OSPF_source"]["header"]["type"]["value"], lsa) for row in facts for lsa in row["OSPF_source"]["header_only_LSAs"]]
        requests = [request for row in facts for request in row["OSPF_source"]["requests"]]
        prefix_types = collections.defaultdict(collections.Counter)
        for lsa in lsas:
            for prefix in lsa["prefixes"]:
                prefix_types[lsa["header_fields"]["LS_type_hex"]][str(prefix["prefix_length"]["value"])] += 1
        source = dict(input=case["input"], capture=case["capture"], capture_SHA256=case["capture_SHA256"],
                      observation_count=len(facts), message_type_counts=counts(row["OSPF_source"]["header"]["type"]["value"] for row in facts),
                      message_length_counts=counts(row["OSPF_source"]["header"]["packet_length"]["value"] for row in facts),
                      instance_ID_counts=counts(row["OSPF_source"]["header"]["instance_ID"]["value"] for row in facts),
                      packet_option_counts=counts(row["OSPF_source"]["fields"]["options"]["value"] for row in facts if "options" in row["OSPF_source"]["fields"]),
                      options_AT_true=0, options_LLS_true=0, IPv4_AF_instances_observed=0,
                      parent_incomplete=sum(not row["ingress"]["parent_complete"] for row in facts),
                      parent_remainder_lengths=counts(row["OSPF_source"]["original_parent_remainder"]["logical_PDU_range"][1] - row["OSPF_source"]["original_parent_remainder"]["logical_PDU_range"][0] for row in facts),
                      direct_and_extension_paths=counts("/".join(ext["kind"] for ext in row["ingress"]["extensions"]) or "direct" for row in facts),
                      link_counts=counts(row["link"] for row in facts),
                      captured_checksum_mathematical_valid=sum(row["OSPF_source"]["source_checksum"]["mathematical_match"] for row in facts),
                      full_LSA_type_occurrences=counts(lsa["header_fields"]["LS_type_hex"] for lsa in lsas),
                      full_LSA_count=len(lsas), unique_full_LSA_wire_hashes=len({lsa["complete_wire"]["source_SHA256"] for lsa in lsas}),
                      complete_LSA_Fletcher_valid=sum(lsa["full_LSA_checksum"]["valid"] for lsa in lsas),
                      DBD_LSA_header_type_occurrences=counts(lsa["LS_type_hex"] for typ, lsa in headers if typ == 2),
                      Ack_LSA_header_type_occurrences=counts(lsa["LS_type_hex"] for typ, lsa in headers if typ == 5),
                      request_LS_type_occurrences=counts(request["LS_type"]["value"] for request in requests),
                      header_only_LSA_count=len(headers), header_only_Fletcher_verification_claimed=False,
                      unknown_full_LSA_bodies=sum(lsa["header_fields"]["unknown_to_supported_base_body_types"] for lsa in lsas),
                      prefix_count=len(prefixes), prefix_length_counts=counts(prefix["prefix_length"]["value"] for prefix in prefixes),
                      prefixes_by_LSA_type={key: dict(value) for key, value in prefix_types.items()},
                      prefix_option_counts=counts(prefix["prefix_options"]["value"] for prefix in prefixes),
                      prefix_nonzero_host_or_padding_bits=sum(prefix["host_or_padding_bits_nonzero"] for prefix in prefixes),
                      rounded_prefix_byte_counts=counts(prefix["rounded_bytes"] for prefix in prefixes),
                      prefix_field_kinds=counts(next(key for key in ("reserved", "metric", "referenced_LS_type") if key in prefix) for prefix in prefixes),
                      source_reassembly_used=False, original_frame_facts=case["source_facts_file"], original_frame_facts_SHA256=case["source_facts_SHA256"])
        assert all(row["OSPF_source"]["header"]["instance_ID"]["value"] == 0 and not row["OSPF_source"]["options_AT_bit"] and not row["OSPF_source"]["options_LLS_bit"] for row in facts)
        sources.append(source)
    with (a.out / "per-input-counts.csv").open("x", newline="", encoding="utf-8") as stream:
        writer = csv.DictWriter(stream, fieldnames=list(rows[0])); writer.writeheader(); writer.writerows(rows)
    snapshots = a.out / "immutable-a82-canonical-source"; snapshots.mkdir()
    pinned = []
    for relative in ("common/bin-parser/rules/ospfv3.yaml", "common/bin-parser/rules/internet_protocol_version_6.yaml", "common/bin-parser/parser/stream_parser/default-parser.go", "common/bin-parser/parser/stream_parser/operator_node.go", "common/bin-parser/parser/stream_parser/utils.go"):
        raw = subprocess.check_output(["git", "show", A82 + ":" + relative], cwd=REPO)
        path = snapshots / (Path(relative).name + ".txt"); path.write_bytes(raw)
        pinned.append(dict(source=A82, repository_path=relative, external_snapshot=str(path), SHA256=sha(path)))
    review = dict(source=A82, source_snapshots=pinned,
                  existing_YAML_constraints=["OSPFv3 Packet Length is bounded at >=16 and <=input; the packet operator requires exact consumption of that declared message.",
                                             "Hello fixed body20 and neighbor remainder multiple4; DD fixed fields and LSA-header remainder multiple20; requests remainder multiple12; Ack remainder multiple20.",
                                             "LSU first reads count32, bounds count by remaining bytes/20, uses that exact list length, and requires each LSA length>=20 and <=remaining bounded view; bodies remain raw bytes.",
                                             "No OSPF-specific per-collection parameter, message pseudoheader checksum or body-prefix semantic codec exists in this original YAML. Byte-count constraints are not proof of the new native collection contract.",
                                             "Original IPv6 extension loop has Hop-by-Hop/Routing/Fragment/Destination but no AH51 route; non-atomic fragment data is not interpreted as a complete child; active routing final-destination context is only passed to selected other codecs, not OSPF."],
                  nonclaims=["This is static review of immutable a82 source, not execution of current candidate YAML/native implementation.",
                             "The 807 actual source frames do not exercise AT/LLS, unknown LSAs, AF instances>=32, active routing headers, short/truncated OSPF or OSPF fragment recovery."])
    rfc = [
        dict(URL="https://www.rfc-editor.org/rfc/rfc5340.html", sections="A.3/A.4.1/A.4.2 and 2.9", applicability="Base message/LSA formats; address storage uses ceil(prefixBits/32) words. Reserved receive fields are preserved rather than a blanket reject. Unknown LSAs retain bounded envelope/body and U/scope semantics; header-only advertisements do not prove full-LSA checksum. Router/Area IDs are 32-bit identifiers, not authenticated data-plane peers."),
        dict(URL="https://www.rfc-editor.org/rfc/rfc7166.html", sections="2/2.1/4.1/4.2", applicability="AT follows OSPF message and optional LLS, excluded from OSPF Length but included in IPv6 payload. AT-bit0x400 appears in Hello/DD; other packet types need trusted context to know expectation. With AT, checksum verification is omitted. Trailer presence/SA identifier does not authenticate without key verification. This cohort has zero AT/LLS/tail, so no real AT validation claim."),
        dict(URL="https://www.rfc-editor.org/rfc/rfc4552.html", sections="2/3/6/13", applicability="AH/ESP protection is separate from the RFC7166 trailer alternative. All 191 visible AH sources provide clear lengths/SPI/sequence/ICV spans, but no SA/key/ICV or replay verification is performed. Observed AH cannot establish authenticated OSPF neighbors or LSDB state."),
    ]
    result = dict(scope=__doc__, driver_SHA256=sha(Path(__file__)), source_scan_SHA256=sha(a.scan), reference_field_proof_SHA256=sha(a.reference_proof),
                  fragment_source_proof_SHA256=sha(a.fragments), input_count=37, total_physical_records=sum(row["physical_frames"] for row in rows),
                  OSPFv3_observations=sum(row["OSPFv3"] for row in rows), per_input_counts_file=str(a.out / "per-input-counts.csv"),
                  per_input_counts_SHA256=sha(a.out / "per-input-counts.csv"), source_summaries=sources,
                  canonical_original_source_review=review, normative_primary_references=rfc,
                  no_candidate_output_or_new_expected_objects=True, every_quantity_is_observation_count_not_unique_LSDB_state=True)
    target = a.out / "summary.json"; target.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(dict(summary=str(target), SHA256=sha(target), inputs=37, OSPFv3=result["OSPFv3_observations"], source_counts=[dict(input=row["input"], frames=row["observation_count"], full_LSA=row["full_LSA_count"], prefixes=row["prefix_count"], links=row["link_counts"]) for row in sources])))

if __name__ == "__main__":
    main()
