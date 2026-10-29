"""Freeze exact OSPFv3 fields/session trees and typed values from source facts.

No candidate Go implementation or native output is read. The value/range
oracle is the independent original-byte census and literal Wireshark check;
the supplied contract supplies names, JSON representations and alias types.
This creates field-only expectations, never complete native event objects.
"""
from pathlib import Path
import argparse, base64, collections, copy, hashlib, ipaddress, json

CONTRACT_SHA = "ae0359a175fdf9fe8350f37284a1398ca85904fce75b5153c08aec9f9c601a7d"
SOURCE_SCAN_SHA = "78bb8a45668758d38f31e9c3a77a0a3ed15a9cbb23f9065328553e59d167fb2b"
REFERENCE_PROOF_SHA = "795071791d702ccedbe9ad48dfbcd0302675019e9412cd0fe6ac33f114d2019e"
PACKET_NAMES = {1: "Hello", 2: "Database Description", 3: "Link State Request", 4: "Link State Update", 5: "Link State Acknowledgment"}
ROOT_SCOPE = "bounded wire advertisements only; no LSDB, verified neighbor or installed route"
ROOT_CHECKSUM_SCOPE = "OSPF packet and IPv6 pseudo-header; authentication unverified"
FULL_CHECKSUM_SCOPE = "complete LSA excluding LS Age; authentication unverified"
HEADER_CHECKSUM_SCOPE = "unverified: advertised LSA header without body"

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False, allow_nan=False)

def same(a, b):
    return canonical(a) == canonical(b)

def context():
    return {"Wire Field Ranges": {}}

def nullable(rows):
    return rows if rows else None

class Builder:
    def __init__(self, source):
        self.source, self.ospf = source, source["OSPF_source"]
        self.frame = Path(source["original_frame_file"]).read_bytes()
        assert sha(self.frame) == source["frame_wire_SHA256"]
        lo, hi = self.ospf["complete_message"]["physical_range"]
        self.raw = self.frame[lo:hi]
        assert sha(self.raw) == self.ospf["complete_message"]["source_SHA256"]
        self.bindings = []

    def put(self, target, path, label, scalar, kind="uint64", value=None):
        physical, logical = scalar["physical_range"], scalar["logical_PDU_range"]
        wire = self.frame[slice(*physical)]
        assert sha(wire) == scalar["source_SHA256"] and self.raw[slice(*logical)] == wire
        if value is None:
            value = scalar["dotted_router_identifier"] if kind == "dotted_identifier" else scalar["value"]
        if kind == "uint64":
            assert type(value) is int and 0 <= value < 1 << 64 and int.from_bytes(wire, "big") == value
        elif kind == "dotted_identifier":
            assert str(ipaddress.IPv4Address(wire)) == value
        elif kind == "ipv6_address":
            assert str(ipaddress.IPv6Address(wire)) == value
        elif kind == "bytes":
            assert value == base64.b64encode(wire).decode()
        target[label] = value
        target["Wire Field Ranges"][label] = list(logical)
        self.bindings.append(dict(path=path + [label], standard_English_name=label, expected_JSON_value=value,
                                  source_kind=kind, physical_range=list(physical), logical_PDU_range=list(logical),
                                  source_SHA256=scalar["source_SHA256"], encoded_wire_bits=len(wire) * 8,
                                  source_context="original packet fields; no native value or raw search used"))

    def derived(self, target, path, label, value, provenance):
        target[label] = value
        self.bindings.append(dict(path=path + [label], standard_English_name=label,
                                  expected_JSON_value=value, source_kind="derived_or_contract_metadata",
                                  source_provenance=provenance, nonempty_wire_range_claimed=False))

    def prefixes(self, rows, path):
        result = []
        for index, source in enumerate(rows):
            target, current = context(), path + [index]
            self.put(target, current, "Prefix Length", source["prefix_length"])
            self.put(target, current, "Prefix Options", source["prefix_options"])
            for key, label in [("reserved", "Reserved"), ("metric", "Metric"), ("referenced_LS_type", "Referenced LS Type")]:
                if key in source:
                    self.put(target, current, label, source[key])
            scalar = source["encoded_address_prefix"]
            wire = self.frame[slice(*scalar["physical_range"])]
            self.put(target, current, "Encoded Prefix", scalar, "bytes", base64.b64encode(wire).decode())
            self.derived(target, current, "Encoded Prefix Bytes", source["rounded_bytes"], "ceil(Prefix Length /32)*4, checked against exact original encoded span")
            padded = wire + bytes(16 - len(wire))
            normalized = str(ipaddress.IPv6Network((ipaddress.IPv6Address(padded), source["prefix_length"]["value"]), strict=False))
            assert normalized == source["normalized_prefix"] and len(wire) == source["rounded_bytes"]
            self.derived(target, current, "Prefix", normalized, "independent IPv6 CIDR normalization of original encoded prefix plus Prefix Length")
            if len(wire):
                target["Wire Field Ranges"]["Prefix"] = list(scalar["logical_PDU_range"])
                self.bindings[-1].update(physical_range=list(scalar["physical_range"]), logical_PDU_range=list(scalar["logical_PDU_range"]),
                                         source_SHA256=scalar["source_SHA256"], nonempty_wire_range_claimed=True)
            result.append(target)
        return nullable(result)

    def lsa(self, source, path, full):
        head = source["header_fields"] if full else source
        target = context()
        for key, label, kind in [("age", "LS Age", "uint64"), ("LS_type", "LS Type", "uint64"), ("link_state_ID", "Link State ID", "dotted_identifier"),
                                 ("advertising_router_ID", "Advertising Router", "dotted_identifier"), ("sequence", "Sequence", "uint64"),
                                 ("checksum", "Checksum", "uint64"), ("length", "Length", "uint64")]:
            self.put(target, path, label, head[key], kind)
        if full:
            assert source["complete_original_LSA"] and source["full_LSA_checksum"]["valid"]
        self.derived(target, path, "Checksum Verified", full, "independent complete LSA Fletcher over bytes excluding LS Age" if full else "source contains only the advertised/acknowledged header, not LSA body")
        self.derived(target, path, "Checksum Scope", FULL_CHECKSUM_SCOPE if full else HEADER_CHECKSUM_SCOPE, "pre-output metadata contract; no authenticated source claim")
        if not full:
            return target
        code, fields, body, current = head["LS_type"]["value"], source["fields"], context(), path + ["Body"]
        if code == 0x2001:
            self.put(body, current, "Flags", fields["flags"])
            self.put(body, current, "Options", fields["options"])
            links = []
            for index, item in enumerate(fields["links"]):
                node, link_path = context(), current + ["Links", index]
                for key, label, kind in [("type", "Link Type", "uint64"), ("reserved", "Reserved", "uint64"), ("metric", "Metric", "uint64"),
                                         ("interface_ID", "Interface ID", "uint64"), ("neighbor_interface_ID", "Neighbor Interface ID", "uint64"),
                                         ("neighbor_router_ID", "Neighbor Router ID", "dotted_identifier")]:
                    self.put(node, link_path, label, item[key], kind)
                links.append(node)
            body["Links"] = nullable(links)
        elif code == 0x2002:
            self.put(body, current, "Reserved", fields["reserved"])
            self.put(body, current, "Options", fields["options"])
            nodes = []
            for index, scalar in enumerate(fields["attached_router_IDs"]):
                node = context(); self.put(node, current + ["Attached Routers", index], "Attached Router", scalar, "dotted_identifier"); nodes.append(node)
            body["Attached Routers"] = nullable(nodes)
        elif code == 0x0008:
            for key, label, kind in [("router_priority", "Router Priority", "uint64"), ("options", "Options", "uint64"),
                                     ("link_local_address", "Link Local Address", "ipv6_address"), ("prefix_count", "Prefix Count", "uint64")]:
                self.put(body, current, label, fields[key], kind)
            body["Prefixes"] = self.prefixes(source["prefixes"], current + ["Prefixes"])
        elif code == 0x2009:
            for key, label, kind in [("prefix_count", "Prefix Count", "uint64"), ("referenced_LS_type", "Referenced LS Type", "uint64"),
                                     ("referenced_link_state_ID", "Referenced Link State ID", "dotted_identifier"),
                                     ("referenced_advertising_router_ID", "Referenced Advertising Router", "dotted_identifier")]:
                self.put(body, current, label, fields[key], kind)
            body["Prefixes"] = self.prefixes(source["prefixes"], current + ["Prefixes"])
        elif code in (0x4005, 0x2007):
            self.put(body, current, "Flags", fields["flags"])
            self.put(body, current, "Metric", fields["metric"])
            body["Prefixes"] = self.prefixes(source["prefixes"], current + ["Prefixes"])
            for key, label, kind in [("forwarding_address", "Forwarding Address", "ipv6_address"), ("external_route_tag", "External Route Tag", "uint64"),
                                     ("referenced_link_state_ID", "Referenced Link State ID", "dotted_identifier")]:
                if key in fields:
                    self.put(body, current, label, fields[key], kind)
        else:
            # No unsupported body shape may silently enter a complete oracle.
            raise AssertionError(("uncontracted or unobserved body family", code))
        target["Body"] = body
        return target

    def build(self):
        source, head, target = self.ospf, self.ospf["header"], context()
        header = context()
        for key, label, kind in [("version", "Version", "uint64"), ("type", "Type", "uint64"), ("packet_length", "Packet Length", "uint64"),
                                 ("router_ID", "Router ID", "dotted_identifier"), ("area_ID", "Area ID", "dotted_identifier"),
                                 ("checksum", "Checksum", "uint64"), ("instance_ID", "Instance ID", "uint64"), ("reserved", "Reserved", "uint64")]:
            self.put(header, ["Header"], label, head[key], kind)
        target["Header"] = header
        self.derived(target, [], "Packet Name", PACKET_NAMES[head["type"]["value"]], "pre-output message naming contract, selected by original Type octet")
        self.derived(target, [], "Authentication Verified", False, "no SA/key/neighbor identity is authenticated by this source oracle")
        self.derived(target, [], "Observation Scope", ROOT_SCOPE, "pre-output metadata contract")
        assert source["source_checksum"]["mathematical_match"] and source["source_checksum"]["destination_context_known"]
        assert not source["options_AT_bit"] and not source["options_LLS_bit"]
        self.derived(target, [], "Checksum Verified", True, "independent original OSPF wire and IPv6 pseudo-header checksum, matched by literal Wireshark")
        self.derived(target, [], "Checksum Scope", ROOT_CHECKSUM_SCOPE, "pre-output metadata contract; no authentication result")
        typ, fields = head["type"]["value"], source["fields"]
        if typ == 1:
            node = context()
            for key, label, kind in [("interface_ID", "Interface ID", "uint64"), ("router_priority", "Router Priority", "uint64"), ("options", "Options", "uint64"),
                                     ("hello_interval", "Hello Interval", "uint64"), ("router_dead_interval", "Router Dead Interval", "uint64"),
                                     ("designated_router_ID", "Designated Router", "dotted_identifier"), ("backup_designated_router_ID", "Backup Designated Router", "dotted_identifier")]:
                self.put(node, ["Hello"], label, fields[key], kind)
            neighbors = []
            for index, scalar in enumerate(fields["neighbor_router_IDs"]):
                item = context(); self.put(item, ["Hello", "Neighbors", index], "Neighbor Router ID", scalar, "dotted_identifier"); neighbors.append(item)
            node["Neighbors"] = nullable(neighbors); target["Hello"] = node
        elif typ == 2:
            node = context()
            for key, label in [("reserved_1", "Reserved 1"), ("options", "Options"), ("interface_MTU", "Interface MTU"),
                               ("reserved_2", "Reserved 2"), ("flags", "Flags"), ("DD_sequence", "DD Sequence")]:
                self.put(node, ["Database Description"], label, fields[key])
            node["LSA Headers"] = nullable([self.lsa(item, ["Database Description", "LSA Headers", index], False)
                                             for index, item in enumerate(source["header_only_LSAs"])])
            target["Database Description"] = node
        elif typ == 3:
            rows = []
            for index, item in enumerate(source["requests"]):
                node = context()
                for key, label, kind in [("reserved", "Reserved", "uint64"), ("LS_type", "LS Type", "uint64"),
                                         ("link_state_ID", "Link State ID", "dotted_identifier"), ("advertising_router_ID", "Advertising Router", "dotted_identifier")]:
                    self.put(node, ["Requests", index], label, item[key], kind)
                rows.append(node)
            target["Requests"] = nullable(rows)
        elif typ == 4:
            self.put(target, [], "LSA Count", fields["LSA_count"])
            target["LSAs"] = nullable([self.lsa(item, ["LSAs", index], True) for index, item in enumerate(source["full_LSAs"])])
        elif typ == 5:
            target["LSA Headers"] = nullable([self.lsa(item, ["LSA Headers", index], False) for index, item in enumerate(source["header_only_LSAs"])])
        return target

def owner_maps(value, path=()):
    if isinstance(value, dict):
        if "Wire Field Ranges" in value:
            yield path, value
        for key, item in value.items():
            if key != "Wire Field Ranges":
                yield from owner_maps(item, path + (key,))
    elif isinstance(value, list):
        for index, item in enumerate(value):
            yield from owner_maps(item, path + (index,))

def typed_values(fields, contract):
    owners = []
    for ordinal, (path, node) in enumerate(owner_maps(fields)):
        ranges = node["Wire Field Ranges"]
        start = min((value[0] for value in ranges.values()), default=1 << 60)
        owners.append((start, ordinal, path, node))
    result, coverage = {}, []
    for kind in ("unsigned", "address", "string"):
        for name, key, scope, bits in contract[kind]:
            rows = []
            for _, ordinal, path, node in sorted(owners, key=lambda item: (item[0], item[1])):
                if key not in node or scope == "header" and path != ("Header",) or scope == "lsa" and "LS Age" not in node:
                    continue
                value = node[key]
                item = dict(name=name, value=value, origin="pdu")
                if kind == "unsigned":
                    assert type(value) is int and 0 <= value < 1 << bits
                    if bits > 53:
                        item["exact_value"] = str(value)
                else:
                    assert isinstance(value, str)
                    if kind == "address":
                        ipaddress.ip_address(value)
                span = node["Wire Field Ranges"].get(key)
                if span is not None and span[1] > span[0]:
                    item["pdu_range"] = dict(offset=span[0], length=span[1] - span[0])
                rows.append(item)
                coverage.append(dict(typed_name=name, source_owner_path=list(path), direct_key=key,
                                     declared_kind=kind, declared_unsigned_bits=bits if kind == "unsigned" else None,
                                     expected_occurrence_ordinal=len(rows) - 1, owner_ordinal=ordinal,
                                     logical_PDU_range=span, projected_nonempty_range="pdu_range" in item))
            if rows:
                assert name not in result
                result[name] = rows
    return result, coverage

def main():
    p = argparse.ArgumentParser(description=__doc__)
    for name in ("scan", "reference-proof", "contract", "out"):
        p.add_argument("--" + name, type=Path, required=True)
    a = p.parse_args(); a.out.mkdir(exist_ok=False)
    assert sha(a.scan.read_bytes()) == SOURCE_SCAN_SHA
    assert sha(a.reference_proof.read_bytes()) == REFERENCE_PROOF_SHA
    assert sha(a.contract.read_bytes()) == CONTRACT_SHA
    scan, reference, contract = (json.loads(path.read_bytes()) for path in (a.scan, a.reference_proof, a.contract))
    assert reference["checked_OSPFv3_frames"] == 807 and reference["source_only_named_field_gaps"] == 0
    (a.out / "frozen-pure-contract.json").write_bytes(a.contract.read_bytes())
    frames, wire_leaves, typed_occurrences, metadata_leaves, ranges = 0, 0, 0, 0, 0
    source_inputs, families, message_types = [], collections.Counter(), collections.Counter()
    target = a.out / "fields-session-and-typed-source-grid.jsonl"
    with target.open("x", encoding="utf-8") as stream:
        for case in scan["captures"]:
            source_file = Path(case["source_facts_file"])
            assert sha(source_file.read_bytes()) == case["source_facts_SHA256"]
            observations = json.loads(source_file.read_bytes())
            if observations:
                source_inputs.append(dict(input=case["input"], capture=case["capture"], capture_SHA256=case["capture_SHA256"],
                                          source_facts_file=str(source_file), source_facts_SHA256=case["source_facts_SHA256"], frames=len(observations)))
            for row in observations:
                assert "OSPF_source" in row and row["ingress"]["parent_complete"] and not row["source_fragment_reassembly_used"]
                builder = Builder(row); fields = builder.build()
                typed, coverage = typed_values(fields, contract)
                for _, node in owner_maps(fields):
                    for key, span in node["Wire Field Ranges"].items():
                        assert key in node and isinstance(span, list) and len(span) == 2 and all(type(value) is int for value in span)
                        assert 0 <= span[0] <= span[1] <= len(builder.raw)
                        if key == "Prefix":
                            assert span[1] > span[0]
                        ranges += 1
                for item in builder.bindings:
                    if item["source_kind"] == "derived_or_contract_metadata":
                        metadata_leaves += 1
                    else:
                        wire_leaves += 1
                typed_occurrences += sum(len(values) for values in typed.values())
                frames += 1
                message_types[str(row["OSPF_source"]["header"]["type"]["value"])] += 1
                for lsa in row["OSPF_source"]["full_LSAs"]:
                    families[lsa["header_fields"]["LS_type_hex"]] += 1
                value = dict(source_identity=dict(input=case["input"], capture_SHA256=case["capture_SHA256"], frame=row["frame"],
                                                  section=row["section"], interface=row["interface"], link=row["link"]),
                             original_frame_file=row["original_frame_file"], original_frame_SHA256=row["frame_wire_SHA256"],
                             source_message_physical_range=row["OSPF_source"]["complete_message"]["physical_range"],
                             source_message_SHA256=sha(builder.raw), source_message_length=len(builder.raw),
                             expected_fields=fields, expected_session=copy.deepcopy(fields),
                             complete_fields_canonical_SHA256=sha(canonical(fields).encode()),
                             expected_typed_values_by_name=typed, typed_source_coverage=coverage,
                             required_fields_and_metadata=builder.bindings,
                             expected_snapshot_schema_version=1, expected_snapshot_completeness="message",
                             expected_snapshot_incomplete=False, expected_snapshot_truncated=False,
                             event_ID_and_PDU_source_metadata="must be independently bound by the future whole-event bridge; no IDs invented in this field plan")
                stream.write(json.dumps(value, ensure_ascii=False, separators=(",", ":")) + "\n")
    assert frames == 807
    result = dict(scope=__doc__, constructor_SHA256=sha(Path(__file__).read_bytes()), source_scan_SHA256=SOURCE_SCAN_SHA,
                  reference_proof_SHA256=REFERENCE_PROOF_SHA, pure_name_contract_original_path=str(a.contract),
                  pure_name_contract_SHA256=CONTRACT_SHA, no_candidate_implementation_or_output_read=True,
                  complete_field_session_pairs=frames, wire_scalar_leaves=wire_leaves, range_entries=ranges,
                  metadata_and_derived_leaves=metadata_leaves, typed_occurrences=typed_occurrences,
                  message_type_counts=dict(message_types), full_LSA_family_counts=dict(families), source_inputs=source_inputs,
                  fields_session_grid=str(target), fields_session_grid_SHA256=sha(target.read_bytes()),
                  matching_policy="Exact whole fields and session trees including every range leaf; exact typed occurrences per name in owner-span/source-array order; no value deduplication",
                  typed_global_name_order="Not constrained by supplied pure contract; order within each name is exact",
                  JSON_type_scope="Check JSON integer/string/bool/bytes encoding and declared alias contract; no Go reflect.Type claim from JSON",
                  remaining_before_runtime_validation=["Whole-event/source/PDU metadata binding and future actual producer receipts remain a separate bridge; this plan contains no complete native event expectation."],
                  unsupported_real_source_families=["Inter-Area-Prefix LSA3", "Inter-Area-Router LSA4", "NSSA LSA7", "unknown LSA", "AF instance>=32", "AT/LLS", "active routing header", "OSPF fragment recovery"])
    summary = a.out / "plan.json"; summary.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(dict(plan=str(summary), SHA256=sha(summary.read_bytes()), frames=frames,
                          wire_scalars=wire_leaves, range_entries=ranges, typed_occurrences=typed_occurrences)))

if __name__ == "__main__":
    main()
