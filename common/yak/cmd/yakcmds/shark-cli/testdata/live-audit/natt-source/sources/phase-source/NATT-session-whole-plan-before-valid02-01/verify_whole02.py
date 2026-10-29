"""Compare every complete fixed C8 object and both close publications.

The before-plan is hard-bound to closed old-source A2/B8. No row, ID, flow,
field or publication from the actual output is normalized or used to fill an
expectation. This is independent of the separate public-37 whole verifier.
"""
from pathlib import Path
import argparse, copy, hashlib, json, traceback

PLAN_SHA = "3141da1650d6067d4a5064d9129965d76cc7e576b888d27105ea096f3dc492d7"
CAPTURE_SHA = "80e228e228677e835960540ceae7226d7d6ede544cb3c79ac050ba0b0467bbc9"
REFERENCE_SHA = "0553ca7f5c9c89caa7d7865590160d19c25e3810c0d8e8905c4e98585b70baa2"

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

def differences(expected, actual, path=""):
    if type(expected) is not type(actual):
        return [dict(path=path, expected=expected, actual=actual, reason="JSON type differs")]
    if isinstance(expected, dict):
        result = [dict(path=path, expected_keys=sorted(expected), actual_keys=sorted(actual), reason="keys differ")] if set(expected) != set(actual) else []
        for key in sorted(set(expected) & set(actual)):
            result.extend(differences(expected[key], actual[key], path + "/" + key))
        return result
    if isinstance(expected, list):
        result = [dict(path=path, expected_length=len(expected), actual_length=len(actual), reason="length differs")] if len(expected) != len(actual) else []
        for index, (a, b) in enumerate(zip(expected, actual)):
            result.extend(differences(a, b, path + "/" + str(index)))
        return result
    return [] if expected == actual else [dict(path=path, expected=expected, actual=actual)]

def read_plan(path):
    assert sha(path.read_bytes()) == PLAN_SHA
    plan = load(path.read_bytes())
    assert plan["no_future_output_consumed"] is True and plan["no_actual_ID_mapping_or_normalization"] is True
    assert plan["original_peer_capture_SHA256"] == CAPTURE_SHA
    result = {}
    for name, spec in plan["expected_outputs"].items():
        target = path.parent / name
        assert sha(target.read_bytes()) == spec["SHA256"]
        rows = load(target.read_bytes())
        assert sha(canonical(rows).encode()) == spec["complete_object_canonical_SHA256"]
        assert len(rows) == 10
        result["events" if "native" in name else "yak"] = rows
    assert set(result) == {"events", "yak"}
    return plan, result

def read_actual(root, kind, source):
    receipt_path = root / (kind + "-producer.json")
    receipt = load(receipt_path.read_bytes())
    export = root / (kind + ".jsonl")
    stderr = root / (kind + ".stderr")
    assert receipt["kind"] == kind and receipt["binary_build_source"] == source
    assert receipt["exit_code"] == 0 and receipt["capture_SHA256"] == CAPTURE_SHA
    assert receipt["reference_SHA256"] == REFERENCE_SHA
    assert receipt["export_SHA256"] == sha(export.read_bytes())
    assert receipt["stderr_SHA256"] == sha(stderr.read_bytes())
    args = receipt["arguments"]
    binary = Path(args[0])
    assert receipt["binary_SHA256"] == sha(binary.read_bytes())
    if kind == "events":
        assert args[1] == "replay" and args[3:] == ["-", "--ignore-embedded-secrets"]
        capture = Path(args[2])
    else:
        assert args[1:3] == ["shark", "-r"] and args[4:] == ["--json", "--ignore-embedded-secrets"]
        capture = Path(args[3])
    assert sha(capture.read_bytes()) == CAPTURE_SHA
    rows = [load(line) for line in export.read_bytes().splitlines()]
    assert len(rows) == receipt["rows"]
    return rows, dict(receipt=str(receipt_path), receipt_SHA256=sha(receipt_path.read_bytes()),
                      export=str(export), export_SHA256=sha(export.read_bytes()),
                      source=source, binary_SHA256=receipt["binary_SHA256"], capture_SHA256=CAPTURE_SHA,
                      exit_code=0, rows=len(rows))

def selftest(expected):
    assert not differences(expected, copy.deepcopy(expected))
    controls = []
    def reject(name, change):
        value = copy.deepcopy(expected)
        change(value)
        delta = differences(expected, value)
        assert delta, name
        controls.append(dict(control=name, rejected=True, changed_paths=[d["path"] for d in delta]))
    reject("protocol-only ESP misidentification", lambda v: v["events"][3].update(protocol="esp"))
    reject("RADIUS moved into TFTP flow", lambda v: v["events"][3].update(flow=3))
    reject("transaction owner changed", lambda v: v["events"][3].update(transaction=1))
    reject("NAS identity changed", lambda v: v["events"][3]["session"].update({"NAS-IP-Address": "192.0.2.2"}))
    reject("unverified authentication upgraded", lambda v: v["events"][3]["fields"].update(Authentication="verified"))
    reject("JSON bool must not equal integer", lambda v: v["events"][0].update(id=True))
    reject("native close object dropped", lambda v: v["events"].pop())
    reject("native close ID order swapped", lambda v: v["events"].__setitem__(slice(8, 10), [v["events"][9], v["events"][8]]))
    reject("CLI restored RRQ destination missing", lambda v: v["yak"][2].update(destination="192.0.2.3"))
    reject("CLI source fields missing", lambda v: v["yak"][3]["pdus"][0].pop("field_values"))
    reject("CLI close publication dropped", lambda v: v["yak"].pop())
    reject("CLI close PDU ID changed", lambda v: v["yak"][8]["published_pdus"][0].update(id=4))
    reject("extra EOF row added", lambda v: v["yak"].append(copy.deepcopy(v["yak"][-1])))
    return controls

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plan", type=Path, required=True)
    parser.add_argument("--after", type=Path, help="Closed fixed C8 directory with per-kind receipts and JSONL")
    parser.add_argument("--after-source", help="Exact build source of the fixed binaries")
    parser.add_argument("--selftest", action="store_true", help="Only synthetic validator controls; no runtime proof")
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    args.out.mkdir(parents=True, exist_ok=False)
    result = dict(scope=__doc__, verifier_SHA256=sha(Path(__file__).read_bytes()),
                  plan=str(args.plan), plan_SHA256=sha(args.plan.read_bytes()),
                  no_actual_ID_or_protocol_normalization=True, no_expected_values_from_actual=True)
    try:
        plan, expected = read_plan(args.plan)
        result["negative_controls"] = selftest(expected)
        if args.selftest:
            assert args.after is None and args.after_source is None
            result.update(mode="validator-selftest-only", runtime_correctness_not_claimed=True,
                          controls=len(result["negative_controls"]), passed=True)
        else:
            assert args.after is not None and args.after_source is not None
            result["actual_producers"] = []
            result["whole_objects"] = {}
            all_delta = []
            for kind in ("events", "yak"):
                actual, receipt = read_actual(args.after, kind, args.after_source)
                result["actual_producers"].append(receipt)
                delta = differences(expected[kind], actual)
                all_delta.extend(dict(kind=kind, **item) for item in delta)
                result["whole_objects"][kind] = len(actual)
            (args.out / "differences.json").write_text(json.dumps(all_delta, indent=2) + "\n", encoding="utf-8")
            result.update(mode="fixed-C8-whole-object-runtime-audit", physical_packets=8,
                          native_close_objects=2, CLI_close_publications=2, difference_count=len(all_delta), passed=not all_delta)
            assert not all_delta, ("complete source-expected objects differ", len(all_delta))
        target = args.out / "summary.json"
        target.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(dict(summary=str(target), SHA256=sha(target.read_bytes()), passed=True,
                              mode=result["mode"])))
    except Exception:
        result.update(passed=False, traceback=traceback.format_exc())
        target = args.out / "failure.json"
        target.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(dict(failure=str(target), SHA256=sha(target.read_bytes()), passed=False)))
        raise

if __name__ == "__main__":
    main()
