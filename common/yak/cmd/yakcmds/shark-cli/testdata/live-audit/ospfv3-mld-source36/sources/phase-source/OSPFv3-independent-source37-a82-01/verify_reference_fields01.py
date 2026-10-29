"""Bind independent source scalars/spans to fresh literal Wireshark PDML.

No native output is read. Masked fields are checked both as original unmasked
bytes and as their semantic bit value. Missing named reserved fields remain
explicit source-only coverage, never fabricated reference agreement. Fletcher
and normalization are independent derived checks, not claims of WS validation.
"""
from pathlib import Path
import argparse, collections, hashlib, ipaddress, json, traceback, xml.etree.ElementTree as ET

def sha(path):
    h = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()

def main():
    p = argparse.ArgumentParser(description=__doc__)
    for name in ("scan", "reference", "out"):
        p.add_argument("--" + name, type=Path, required=True)
    a = p.parse_args(); a.out.mkdir(exist_ok=False)
    source, reference = json.loads(a.scan.read_bytes()), json.loads(a.reference.read_bytes())
    count = collections.Counter(); coverage = []; comparisons = []; semantic_notes = []
    try:
        for index, (item, ref) in enumerate(zip(source["captures"], reference["receipts"]), 1):
            assert item["input"] == ref["input"] == index and ref["source_bytes_equal"] and ref["exit_code"] == 0
            rows = json.loads(Path(item["source_facts_file"]).read_bytes())
            assert sha(Path(item["source_facts_file"])) == item["source_facts_SHA256"]
            by_frame = {row["frame"]: row for row in rows}
            assert sorted(by_frame) == sorted(ref["OSPFv3_physical_frames"]), (index, "source/reference frame-set difference")
            pdml = a.reference.parent / ("input-%02d" % index) / "ospfv3.pdml"
            assert sha(pdml) == ref["PDML_SHA256"]
            done = []
            for _, packet in ET.iterparse(pdml, events=("end",)):
                if packet.tag != "packet":
                    continue
                number = int(packet.find(".//field[@name='frame.number']").get("show"))
                row = by_frame[number]
                wire_path = Path(row["original_frame_file"]); wire = wire_path.read_bytes()
                assert sha(wire_path) == row["frame_wire_SHA256"]
                ospf = row["OSPF_source"]
                fields = [dict(field.attrib) for field in packet.findall(".//field")]
                observed = collections.defaultdict(list)
                for field in fields:
                    if "pos" in field and "size" in field:
                        observed[(field.get("name"), int(field["pos"]), int(field["size"]))].append(field)
                checks = []
                def check(name, scalar, semantic=None, optional=False):
                    lo, hi = scalar["physical_range"]
                    size = hi - lo
                    assert hashlib.sha256(wire[lo:hi]).hexdigest() == scalar["source_SHA256"]
                    matches = observed.get((name, lo, size), [])
                    if not matches and optional:
                        coverage.append(dict(input=index, frame=number, source_scalar=scalar, requested_reference_field=name,
                                             reason="no standalone named reference field; source bytes/range remain independently available"))
                        count["reserved_or_optional_source_only_fields"] += 1
                        return
                    assert matches, (index, number, name, lo, hi, "reference named field/span absent")
                    for match in matches:
                        unmasked = match.get("unmaskedvalue", match.get("value"))
                        if size == 0:
                            assert unmasked in (None, "")
                        else:
                            assert unmasked is not None and int(unmasked, 16) == int.from_bytes(wire[lo:hi], "big"), (index, number, name, "raw bits differ")
                        if "value" in scalar:
                            expected = scalar["value"]
                            if isinstance(expected, int):
                                assert int.from_bytes(wire[lo:hi], "big") == expected
                            elif ":" in expected:
                                assert ipaddress.IPv6Address(match["show"]) == ipaddress.IPv6Address(expected)
                        if "dotted_router_identifier" in scalar:
                            assert match["show"] == scalar["dotted_router_identifier"]
                        if semantic is not None:
                            value = match.get("value")
                            assert value is not None and int(value, 16) == int(semantic), (index, number, name, "semantic mask differs", value, semantic)
                    count["strict_named_field_source_value_and_span_checks"] += 1
                    checks.append(dict(reference_field=name, physical_range=[lo, hi], source_SHA256=scalar["source_SHA256"], semantic_bit_value=semantic))
                h = ospf["header"]
                for key, name in [("version", "ospf.version"), ("type", "ospf.msg"), ("packet_length", "ospf.packet_length"),
                                  ("router_ID", "ospf.srcrouter"), ("area_ID", "ospf.area_id"), ("checksum", "ospf.checksum"),
                                  ("instance_ID", "ospf.v3.instance_id"), ("reserved", "ospf.reserved")]:
                    check(name, h[key])
                protocol = [proto for proto in packet.findall("proto") if proto.get("name") == "ospf"]
                assert len(protocol) == 1
                lo, hi = ospf["complete_message"]["physical_range"]
                assert int(protocol[0].get("pos")) == lo and int(protocol[0].get("size")) == hi - lo
                cs = observed[("ospf.checksum", h["checksum"]["physical_range"][0], 2)][0]
                assert cs.get("showname", "").endswith("[correct]") and ospf["source_checksum"]["mathematical_match"]
                count["OSPFv3_source_and_WS_checksum_match"] += 1
                typ, body = h["type"]["value"], ospf["fields"]
                if typ == 1:
                    for key, name in [("interface_ID", "ospf.hello.interface_id"), ("router_priority", "ospf.hello.router_priority"),
                                      ("options", "ospf.v3.options"), ("hello_interval", "ospf.hello.hello_interval"),
                                      ("router_dead_interval", "ospf.hello.router_dead_interval"), ("designated_router_ID", "ospf.hello.designated_router"),
                                      ("backup_designated_router_ID", "ospf.hello.backup_designated_router")]:
                        check(name, body[key])
                    for scalar in body["neighbor_router_IDs"]:
                        check("ospf.hello.active_neighbor", scalar)
                elif typ == 2:
                    for key, name in [("reserved_1", "ospf.reserved"), ("options", "ospf.v3.options"), ("interface_MTU", "ospf.db.interface_mtu"),
                                      ("reserved_2", "ospf.reserved"), ("flags", "ospf.dbd"), ("DD_sequence", "ospf.db.dd_sequence")]:
                        check(name, body[key])
                elif typ == 3:
                    for request in ospf["requests"]:
                        for key, name in [("reserved", "ospf.reserved"), ("LS_type", "ospf.v3.lsa"),
                                          ("link_state_ID", "ospf.link_state_id"), ("advertising_router_ID", "ospf.advrouter")]:
                            check(name, request[key])
                elif typ == 4:
                    check("ospf.ls.number_of_lsas", body["LSA_count"])
                if "options" in body:
                    for name, mask in [("ospf.v3.options.at", 0x400), ("ospf.v3.options.l", 0x200), ("ospf.v3.options.af", 0x100)]:
                        check(name, body["options"], bool(body["options"]["value"] & mask))
                for lsa in ospf["full_LSAs"] + [dict(header_fields=value) for value in ospf["header_only_LSAs"]]:
                    header = lsa["header_fields"]
                    for key, name in [("age", "ospf.lsa.age"), ("LS_type", "ospf.v3.lsa"), ("link_state_ID", "ospf.link_state_id"),
                                      ("advertising_router_ID", "ospf.advrouter"), ("sequence", "ospf.lsa.seqnum"),
                                      ("checksum", "ospf.lsa.chksum"), ("length", "ospf.lsa.length")]:
                        check(name, header[key])
                    check("ospf.v3.lsa.do_not_age", header["age"], header["DoNotAge"])
                    for name, expected in [("ospf.v3.lsa.u", header["U_bit"]), ("ospf.v3.lsa.s12", header["scope_bits"]), ("ospf.v3.lsa.fc", header["function_code"])]:
                        check(name, header["LS_type"], expected)
                    if "fields" not in lsa:
                        count["header_only_LSA_no_Fletcher_claim"] += 1
                        continue
                    assert lsa["full_LSA_checksum"]["valid"]
                    count["complete_LSA_independent_Fletcher_valid"] += 1
                    fields_body, code = lsa["fields"], header["LS_type"]["value"]
                    body_map = {
                        0x2001: [("flags", "ospf.v3.router.lsa.flags"), ("options", "ospf.v3.options")],
                        0x2002: [("reserved", "ospf.reserved"), ("options", "ospf.v3.options")],
                        0x2003: [("reserved_metric", "ospf.reserved"), ("metric", "ospf.metric")],
                        0x2004: [("reserved_options", "ospf.reserved"), ("options", "ospf.v3.options"), ("reserved_metric", "ospf.reserved"), ("metric", "ospf.metric"), ("destination_router_ID", "ospf.v3.lsa.destination_router_id")],
                        0x0008: [("router_priority", "ospf.v3.lsa.router_priority"), ("options", "ospf.v3.options"), ("link_local_address", "ospf.v3.lsa.link_local_interface_address.ipv6"), ("prefix_count", "ospf.v3.lsa.num_prefixes")],
                        0x2009: [("prefix_count", "ospf.v3.lsa.num_prefixes"), ("referenced_LS_type", "ospf.v3.lsa.referenced_ls_type"), ("referenced_link_state_ID", "ospf.v3.lsa.referenced_link_state_id"), ("referenced_advertising_router_ID", "ospf.v3.lsa.referenced_advertising_router")],
                        0x4005: [("flags", "ospf.v3.as.external.flags"), ("metric", "ospf.metric"), ("forwarding_address", "ospf.v3.forwarding_address.ipv6"), ("external_route_tag", "ospf.v3.lsa.external_route_tag"), ("referenced_link_state_ID", "ospf.v3.lsa.referenced_link_state_id")],
                    }
                    for key, name in body_map.get(code, []):
                        if key in fields_body:
                            check(name, fields_body[key], optional=key.startswith("reserved"))
                    for link in fields_body.get("links", []):
                        for key, name in [("type", "ospf.v3.lsa.type"), ("reserved", "ospf.reserved"), ("metric", "ospf.metric"),
                                          ("interface_ID", "ospf.v3.lsa.interface_id"), ("neighbor_interface_ID", "ospf.v3.lsa.neighbor_interface_id"), ("neighbor_router_ID", "ospf.v3.lsa.neighbor_router_id")]:
                            check(name, link[key], optional=key == "reserved")
                    for scalar in fields_body.get("attached_router_IDs", []):
                        check("ospf.v3.lsa.attached_router", scalar)
                    for prefix in lsa["prefixes"]:
                        check("ospf.prefix_length", prefix["prefix_length"])
                        check("ospf.v3.prefix.options", prefix["prefix_options"])
                        for key, name in [("metric", "ospf.metric"), ("reserved", "ospf.reserved"), ("referenced_LS_type", "ospf.v3.lsa.referenced_ls_type")]:
                            if key in prefix:
                                check(name, prefix[key], optional=key == "reserved")
                        scalar = prefix["encoded_address_prefix"]
                        check("ospf.v3.address_prefix.ipv6", dict(scalar, value=prefix["source_padded_address"]))
                        count["all_family_prefix_values_ranges_rounding_and_normalization"] += 1
                        if prefix.get("referenced_LS_type", {}).get("value") == 0:
                            left, right = prefix["referenced_LS_type"]["physical_range"]
                            reference_field = observed[("ospf.v3.lsa.referenced_ls_type", left, right - left)][0]
                            semantic_notes.append(dict(input=index, frame=number, physical_range=[left, right],
                                                       reference_showname=reference_field.get("showname"), source_value=0,
                                                       RFC_meaning="no referenced LSA/optional referenced Link-State-ID; not an unknown outer LSA body"))
                    if code in (0x4005, 0x2007):
                        for bit in ("E", "F", "T"):
                            check("ospf.v3.as.external.flags." + bit.lower(), fields_body["flags"], fields_body[bit + "_bit"])
                for ext in row["ingress"]["extensions"]:
                    if ext["kind"] != "AH":
                        continue
                    lo, hi = ext["range"]
                    for name, offset, size, expected in [("ah.next_header", 0, 1, ext["next_header"]), ("ah.length", 1, 1, ext["length_octet"]),
                                                         ("ah.spi", 4, 4, ext["SPI"]), ("ah.sequence", 8, 4, ext["sequence"])]:
                        check(name, dict(physical_range=[lo + offset, lo + offset + size], source_SHA256=hashlib.sha256(wire[lo + offset:lo + offset + size]).hexdigest(), value=expected))
                    count["AH_headers_observed_not_authenticated"] += 1
                comparisons.append(dict(input=index, frame=number, original_frame_SHA256=row["frame_wire_SHA256"],
                                        named_source_checks=checks, complete_message_length=ospf["header"]["packet_length"]["value"]))
                done.append(number); packet.clear()
            assert sorted(done) == sorted(by_frame)
        for name, value in (("source-only-reference-coverage.json", coverage), ("literal-reference-semantic-notes.json", semantic_notes), ("per-frame-checks.json", comparisons)):
            (a.out / name).write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")
        summary = dict(scope=__doc__, driver_SHA256=sha(Path(__file__)), source_scan_SHA256=sha(a.scan), reference_SHA256=sha(a.reference),
                       input_count=37, complete_original_frame_sets_equal=True, checked_OSPFv3_frames=len(comparisons), counts=dict(count),
                       exact_reference_semantic_notes=len(semantic_notes), source_only_named_field_gaps=len(coverage),
                       full_LSA_checksum_provenance="Independent complete-wire Fletcher calculation plus literal WS checksum field; DD/Ack never validated as complete LSA",
                       native_or_candidate_output_used=False, new_native_expectations_generated=False,
                       outputs={name: sha(a.out / name) for name in ("source-only-reference-coverage.json", "literal-reference-semantic-notes.json", "per-frame-checks.json")})
        target = a.out / "summary.json"; target.write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(dict(summary=str(target), SHA256=sha(target), frames=len(comparisons), counts=dict(count))))
    except Exception:
        (a.out / "failure.json").write_text(json.dumps(dict(traceback=traceback.format_exc(), completed_frames=len(comparisons), counts=dict(count), source_only_fields=coverage, semantic_notes=semantic_notes), indent=2) + "\n", encoding="utf-8")
        raise

if __name__ == "__main__":
    main()
