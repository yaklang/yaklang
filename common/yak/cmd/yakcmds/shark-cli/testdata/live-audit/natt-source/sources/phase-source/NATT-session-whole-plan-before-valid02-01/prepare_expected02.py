"""Freeze complete C8 expectations, including both RADIUS close publications.

The two RRQs in B8 were intentionally routed to another server IP so the
old binary could exercise all eight complete grammars without its known
coarse TFTP admission bug. Restore only those two destination observations;
all IDs, transactions, flows, raw sources and every other value stay literal.
Future fixed-runtime output is not accepted by this constructor.
"""
from pathlib import Path
import argparse, base64, copy, hashlib, json

OLD_SOURCE = "5c88aed7aa9ee8c7438aca30a6bfaa114fff85d9"
REFERENCE_SHA = "0553ca7f5c9c89caa7d7865590160d19c25e3810c0d8e8905c4e98585b70baa2"
SOURCE_MANIFEST_SHA = "360fe260969ed3d7f48000724ca35d09b79ef84946b740d6ea5d7b1dc058adbf"
OLD_CLOSED_SUMMARY_SHA = "242b3e7b5ecf341b195222ec9af99843e7770f6e073f52b5a8d2a84cd16555a5"
CAPTURE_SHAS = {
    "A": "d48db1b8608123df52fd957cc88770821c19e5c5efc9a442b43006b30d738f33",
    "B": "656da57ea9373a651d72fb26d4dfe47cab3b3f413532bb0002fd4430e38e0bc5",
    "C": "80e228e228677e835960540ceae7226d7d6ede544cb3c79ac050ba0b0467bbc9",
}
BINARY_SHAS = {
    "events": "5c9e2cb96255f1dbd282d0a48dde4adbc30befa3b4ab109030ca774ce7104b98",
    "yak": "f4354d318c3a7b9dad97d17a3c82b87d2c43fedc30c4b052fb93872706be5a9e",
}

def sha(data):
    return hashlib.sha256(data).hexdigest()

def duplicate_free(pairs):
    value = {}
    for key, item in pairs:
        assert key not in value, ("duplicate JSON key", key)
        value[key] = item
    return value

def load(data):
    return json.loads(data, object_pairs_hook=duplicate_free)

def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False, allow_nan=False)

def objects(path):
    return [load(line) for line in path.read_bytes().splitlines()]

def old_export(root, kind, capture, count):
    receipt_path = root / (kind + "-producer.json")
    receipt = load(receipt_path.read_bytes())
    export = root / (kind + ".jsonl")
    stderr = root / (kind + ".stderr")
    assert receipt["kind"] == kind and receipt["binary_build_source"] == OLD_SOURCE
    assert receipt["binary_SHA256"] == BINARY_SHAS[kind] and receipt["exit_code"] == 0
    assert receipt["capture_SHA256"] == CAPTURE_SHAS[capture]
    assert receipt["reference_SHA256"] == REFERENCE_SHA
    assert receipt["export_SHA256"] == sha(export.read_bytes())
    assert receipt["stderr_SHA256"] == sha(stderr.read_bytes())
    args = receipt["arguments"]
    if kind == "events":
        assert args[1] == "replay" and args[3:] == ["-", "--ignore-embedded-secrets"]
        source_capture = Path(args[2])
    else:
        assert args[1:3] == ["shark", "-r"] and args[4:] == ["--json", "--ignore-embedded-secrets"]
        source_capture = Path(args[3])
    assert sha(source_capture.read_bytes()) == CAPTURE_SHAS[capture]
    rows = objects(export)
    assert len(rows) == count == receipt["rows"]
    return rows, dict(receipt=str(receipt_path), receipt_SHA256=sha(receipt_path.read_bytes()),
                      export=str(export), export_SHA256=sha(export.read_bytes()),
                      capture=str(source_capture), capture_SHA256=CAPTURE_SHAS[capture],
                      binary_build_source=OLD_SOURCE, binary_SHA256=receipt["binary_SHA256"])

def replace_field_values(snapshot, old_ip, new_ip, old_endpoint, new_endpoint):
    changed = []
    for index, value in enumerate(snapshot["values"]):
        if value["name"] == "destination":
            assert value["value"] == old_endpoint
            value["value"] = new_endpoint
            changed.append(index)
        elif value["name"] == "ip.addr" and value["value"] == old_ip:
            value["value"] = new_ip
            changed.append(index)
    assert len(changed) == 2

def restore_rrq(event, row, number):
    old_ip = "192.0.2.3" if number == 3 else "2001:db8::3"
    new_ip = "192.0.2.2" if number == 3 else "2001:db8::2"
    old_endpoint = old_ip + ":69" if number == 3 else "[" + old_ip + "]:69"
    new_endpoint = new_ip + ":69" if number == 3 else "[" + new_ip + "]:69"
    assert event["protocol"] == "tftp" and event["destination"] == old_endpoint
    assert event["id"] == number and event["flow"] == number
    assert event["status"] == "decoded" and not event.get("error")
    assert base64.b64decode(event["raw"]) == b"\x00\x01example.txt\x00octet\x00"
    assert event["session"]["Server Transfer ID"] == ""
    event["destination"] = new_endpoint
    replace_field_values(event["field_values"], old_ip, new_ip, old_endpoint, new_endpoint)
    assert row["destination"] == old_ip and row["message_destination"] == old_endpoint
    row["destination"], row["message_destination"] = new_ip, new_endpoint
    assert row["conversation_id"] == number and len(row["pdus"]) == 1
    pdu = row["pdus"][0]
    assert pdu["id"] == number and pdu["protocol"] == "tftp"
    replace_field_values(pdu["field_values"], old_ip, new_ip, old_endpoint, new_endpoint)
    assert pdu["fields"]["destination"] == [old_endpoint]
    assert pdu["fields"]["ip.addr"] == (["192.0.2.1", old_ip] if number == 3 else ["2001:db8::1", old_ip])
    pdu["fields"]["destination"] = [new_endpoint]
    pdu["fields"]["ip.addr"][1] = new_ip
    assert old_ip not in canonical(event) and old_ip not in canonical(row)

def differences(a, b, path=""):
    if type(a) is not type(b):
        return [dict(path=path, before=a, expected=b)]
    if isinstance(a, dict):
        assert set(a) == set(b), (path, set(a) ^ set(b))
        return [value for key in sorted(a) for value in differences(a[key], b[key], path + "/" + key)]
    if isinstance(a, list):
        assert len(a) == len(b), path
        return [value for index, (old, new) in enumerate(zip(a, b))
                for value in differences(old, new, path + "/" + str(index))]
    return [] if a == b else [dict(path=path, before=a, expected=b)]

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--a", type=Path, required=True, help="Closed old A2 producer directory")
    parser.add_argument("--b", type=Path, required=True, help="Closed old B8 producer directory")
    parser.add_argument("--source", type=Path, required=True, help="Frozen source02 directory")
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    manifest = args.source / "manifest02.json"
    assert sha(manifest.read_bytes()) == SOURCE_MANIFEST_SHA
    for name, expected in load(manifest.read_bytes())["files"].items():
        path = args.source / name
        assert path.stat().st_size == expected["bytes"] and sha(path.read_bytes()) == expected["SHA256"]
    assert args.a.parent.resolve() == args.b.parent.resolve()
    closed_summary = args.a.parent / "summary.json"
    assert sha(closed_summary.read_bytes()) == OLD_CLOSED_SUMMARY_SHA
    closed = load(closed_summary.read_bytes())
    assert closed["source_unchanged"] is True and closed["producer_code_source"] == OLD_SOURCE
    native_a, a_n_receipt = old_export(args.a, "events", "A", 4)
    cli_a, a_y_receipt = old_export(args.a, "yak", "A", 4)
    native_b, b_n_receipt = old_export(args.b, "events", "B", 10)
    cli_b, b_y_receipt = old_export(args.b, "yak", "B", 10)
    for root in (args.a, args.b):
        for kind in ("events", "yak"):
            standalone = load((root / (kind + "-producer.json")).read_bytes())
            matches = [r for r in closed["outputs"] if r["case"] == root.name and r["kind"] == kind]
            assert len(matches) == 1 and canonical(matches[0]) == canonical(standalone)
    native_names = ["stun", "coap", "tftp", "radius"] * 2
    cli_names = ["STUN", "COAP", "TFTP", "RADIUS"] * 2
    assert [e["protocol"] for e in native_b] == native_names + ["radius", "radius"]
    assert [r["protocol"] for r in cli_b[:8]] == cli_names
    assert [e["id"] for e in native_b] == list(range(1, 11))
    assert [e["flow"] for e in native_b] == list(range(1, 9)) + [4, 8]
    assert [e["status"] for e in native_b] == ["decoded"] * 8 + ["incomplete"] * 2
    assert all(not e.get("error") for e in native_b[:8])
    for index, event in enumerate(native_b[8:], 8):
        assert event["error"] == "RADIUS UDP unmatched observed Identifiers"
        assert event["expert"] == "NeedMore" and event["fields"] is None and event["raw"] is None
        assert event["session"] == {"Outstanding": 1} and event["transaction"] == event["response_to"] == 0
        assert event["source_bytes"]["PacketRefs"] is None and event["source_bytes"]["SHA256"] == ""
        row = cli_b[index]
        assert row["record_type"] == "protocol_publication" and row["publication_phase"] == "capture_close"
        assert "protocol" not in row and len(row["published_pdus"]) == 1
        assert row["published_pdus"][0]["id"] == index + 1
    radius_confirmations = []
    excluded = {"id", "flow", "transaction", "source_bytes", "field_values"}
    for a_index, b_index in [(0, 3), (1, 7)]:
        a, b = native_a[a_index], native_b[b_index]
        assert a["protocol"] == b["protocol"] == "radius"
        assert a["status"] == b["status"] == "decoded" and not a.get("error") and not b.get("error")
        assert a["id"] == a_index + 1 and b["id"] == b_index + 1
        assert a["transaction"] == a["id"] and b["transaction"] == b["id"]
        assert a["response_to"] == b["response_to"] == 0
        a_core = {k: v for k, v in a.items() if k not in excluded}
        b_core = {k: v for k, v in b.items() if k not in excluded}
        assert canonical(a_core) == canonical(b_core)
        assert a["session"]["NAS-IP-Address"] == "192.0.2.1"
        assert a["session"]["User-Name"] == "test" and a["session"]["Attribute Count"] == 2
        assert a["session"]["Authentication"] == "not-verified"
        assert len(base64.b64decode(a["raw"])) == 32
        assert cli_a[a_index]["protocol"] == "RADIUS" and cli_b[b_index]["protocol"] == "RADIUS"
        radius_confirmations.append(dict(A_frame=a_index + 1, B_C_frame=b_index + 1,
                                         identical_complete_non_identity_native_keys=sorted(a_core),
                                         complete_core_SHA256=sha(canonical(a_core).encode()),
                                         observed_transaction_owner_equals_own_request_ID=True,
                                         authentication_not_verified=True))
    expected_native, expected_cli = copy.deepcopy(native_b), copy.deepcopy(cli_b)
    for number in (3, 7):
        restore_rrq(expected_native[number - 1], expected_cli[number - 1], number)
    delta = []
    for index in range(10):
        n = differences(native_b[index], expected_native[index])
        y = differences(cli_b[index], expected_cli[index])
        assert (len(n), len(y)) == ((3, 6) if index + 1 in (3, 7) else (0, 0))
        delta.append(dict(frame=index + 1, native_source_corrections=n, CLI_source_corrections=y))
    args.out.mkdir(parents=True, exist_ok=False)
    outputs = {}
    for name, value in (("expected-native-C8.json", expected_native), ("expected-CLI-C8.json", expected_cli)):
        target = args.out / name
        target.write_text(json.dumps(value, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
        outputs[name] = dict(path=str(target), SHA256=sha(target.read_bytes()),
                             complete_object_canonical_SHA256=sha(canonical(value).encode()))
    plan = dict(scope=__doc__, constructor_SHA256=sha(Path(__file__).read_bytes()),
                frozen_before_fixed_producer=True, no_future_output_consumed=True,
                no_actual_ID_mapping_or_normalization=True,
                original_peer_capture_SHA256=CAPTURE_SHAS["C"], source_manifest_SHA256=SOURCE_MANIFEST_SHA,
                source_reference_SHA256=REFERENCE_SHA, old_binary_source=OLD_SOURCE,
                old_producers=[a_n_receipt, a_y_receipt, b_n_receipt, b_y_receipt],
                expected_outputs=outputs, source_corrections_B_to_C=delta,
                all_IDs_flows_transactions_and_other_values_literal_B8=True,
                old_closed_producer_summary=str(closed_summary), old_closed_producer_summary_SHA256=OLD_CLOSED_SUMMARY_SHA,
                complete_native_objects=10, complete_CLI_rows=10, physical_packets=8,
                close_publications=2, close_native_IDs=[9, 10], close_flow_IDs=[4, 8],
                all_close_objects_and_rows_literal_B8=True,
                independent_radius_A2_confirmations=radius_confirmations,
                Wireshark_default_CoAP_label_is_ESP_not_relabelled=True,
                scope_limitations=["Synthetic source controls, independent of public captures.",
                                   "Parsed RADIUS/NAS fields are unauthenticated observations.",
                                   "Existing candidate-prefix/known-TID refusal semantics need separate Go regression coverage.",
                                   "Complete objects are compared; matching protocol names alone is not the oracle."])
    path = args.out / "plan.json"
    path.write_text(json.dumps(plan, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(dict(plan=str(path), SHA256=sha(path.read_bytes()), complete_native=10,
                          complete_CLI=10, physical_packets=8, close_publications=2, future_output_consumed=False)))

if __name__ == "__main__":
    main()
