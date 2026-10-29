"""Strict field-only verifier for the frozen 807 source-derived expectations.

Whole-event/PDU/source metadata binding belongs to the later independent bridge.
An actual field-export row must provide source_identity, fields, session and
typed_snapshot. This verifier compares complete trees and every typed occurrence
against frozen values, never re-decodes the candidate Raw to fill expectations.
"""
from pathlib import Path
import argparse, collections, copy, hashlib, json, traceback

PLAN_SHA = "0f7caa92c06cf6971fe13cae804810b6cab4bbda38235d1f7cd78dd7a3326036"

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False, allow_nan=False)

def load_unique(raw):
    def pairs(items):
        result = {}
        for key, value in items:
            assert key not in result, ("duplicate JSON key", key)
            result[key] = value
        return result
    return json.loads(raw, object_pairs_hook=pairs)

def source_key(identity):
    return canonical(identity)

def compare_fields(expected, fields, session, snapshot):
    assert canonical(fields) == canonical(expected["expected_fields"]), "whole fields tree/value/type/range differs"
    assert canonical(session) == canonical(expected["expected_session"]), "whole session tree/value/type/range differs"
    assert snapshot["schema_version"] == 1 and type(snapshot["schema_version"]) is int
    assert snapshot["completeness"] == "message"
    for key in ("incomplete", "truncated"):
        assert key not in snapshot or snapshot[key] is False, (key, "complete source must not become partial")
    actual = collections.defaultdict(list)
    declared = expected["expected_typed_values_by_name"]
    for value in snapshot["values"]:
        assert isinstance(value, dict) and isinstance(value.get("name"), str)
        if value["name"].startswith(("ospf.", "ospfv3.")):
            assert value.get("origin") == "pdu", "protocol source field cannot be reassigned to event origin"
        if value.get("origin") == "pdu":
            actual[value["name"]].append(value)
    assert set(actual) == set(declared), ("missing/extra typed protocol names", sorted(set(actual) ^ set(declared)))
    for name, values in declared.items():
        assert canonical(actual[name]) == canonical(values), ("typed value/type/range/exact_value/occurrence differs", name)
    return dict(whole_fields_verified=True, whole_session_verified=True, typed_names=len(declared),
                typed_occurrences=sum(len(values) for values in declared.values()),
                event_source_PDU_metadata_binding_verified=False)

def baseline(record):
    return dict(fields=copy.deepcopy(record["expected_fields"]), session=copy.deepcopy(record["expected_session"]),
                typed_snapshot=dict(schema_version=1, completeness="message",
                                    values=[copy.deepcopy(row) for name in sorted(record["expected_typed_values_by_name"]) for row in record["expected_typed_values_by_name"][name]]))

def prefixes(value):
    if isinstance(value, dict):
        if "Prefix Length" in value:
            yield value
        for key, item in value.items():
            if key != "Wire Field Ranges":
                yield from prefixes(item)
    elif isinstance(value, list):
        for item in value:
            yield from prefixes(item)

def selftest(records):
    results = []
    def reject(label, record, mutation):
        actual = baseline(record)
        mutation(actual)
        try:
            compare_fields(record, actual["fields"], actual["session"], actual["typed_snapshot"])
        except AssertionError:
            results.append(dict(control=label, rejected=True))
        else:
            raise AssertionError(("mutant accepted", label))
    first = records[0]
    compare_fields(first, **dict(fields=baseline(first)["fields"], session=baseline(first)["session"], snapshot=baseline(first)["typed_snapshot"]))
    reject("reserved scalar changed", first, lambda row: row["fields"]["Header"].update(Reserved=1))
    reject("authentication promoted without evidence", first, lambda row: row["fields"].update({"Authentication Verified": True}))
    reject("extra unknown whole-tree leaf", first, lambda row: row["fields"].update({"LSDB Verified": True}))
    reject("session diverges from source fields", first, lambda row: row["session"]["Header"].update({"Instance ID": 1}))
    reject("typed bool cannot equal numeric integer", first, lambda row: next(item for item in row["typed_snapshot"]["values"] if item["name"] == "ospf.version").update(value=True))
    reject("32bit dedicated alias falsely adds exact_value", first, lambda row: next(item for item in row["typed_snapshot"]["values"] if item["name"] == "ospf.version").update(exact_value="3"))
    reject("known field moved to event origin", first, lambda row: next(item for item in row["typed_snapshot"]["values"] if item["name"] == "ospf.version").update(origin="event"))
    reject("extra uncontracted protocol alias", first, lambda row: row["typed_snapshot"]["values"].append(dict(name="ospfv3.sa_verified", value=True, origin="pdu")))
    repeated = next(record for record in records if len(record["expected_typed_values_by_name"].get("ospfv3.options", [])) > 1)
    reject("equal options incorrectly deduplicated", repeated, lambda row: row["typed_snapshot"]["values"].remove(next(item for item in row["typed_snapshot"]["values"] if item["name"] == "ospfv3.options")))
    null_neighbors = next(record for record in records if record["expected_fields"].get("Hello", {}).get("Neighbors", []) is None)
    reject("nil collection silently changed to empty array", null_neighbors, lambda row: row["fields"]["Hello"].update(Neighbors=[]))
    header_only = next(record for record in records if record["expected_fields"].get("LSA Headers"))
    reject("Ack header falsely verified as full LSA", header_only, lambda row: row["fields"]["LSA Headers"][0].update({"Checksum Verified": True}))
    p127 = next(record for record in records if any(row["Prefix Length"] == 127 for row in prefixes(record["expected_fields"])))
    def truncate(row):
        field = next(item for item in prefixes(row["fields"]) if item["Prefix Length"] == 127)
        field["Wire Field Ranges"]["Encoded Prefix"][1] -= 1
    reject("prefix uses floor bits/8 instead of rounded storage", p127, truncate)
    p0 = next(record for record in records if any(row["Prefix Length"] == 0 for row in prefixes(record["expected_fields"])))
    def fake_range(row):
        field = next(item for item in prefixes(row["fields"]) if item["Prefix Length"] == 0)
        field["Wire Field Ranges"]["Prefix"] = list(field["Wire Field Ranges"]["Encoded Prefix"])
    reject("zero-prefix fabricates derived wire range", p0, fake_range)
    def null_bytes(row):
        next(item for item in prefixes(row["fields"]) if item["Prefix Length"] == 0)["Encoded Prefix"] = None
    reject("zero encoded prefix loses explicit empty-byte representation", p0, null_bytes)
    reject("source wire ranges become owner-relative", first, lambda row: row["fields"]["Header"]["Wire Field Ranges"].update(Checksum=[0, 2]))
    return results

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--plan", type=Path, required=True)
    p.add_argument("--field-exports", type=Path)
    p.add_argument("--selftest", action="store_true")
    p.add_argument("--out", type=Path, required=True)
    a = p.parse_args(); a.out.mkdir(exist_ok=False)
    result = dict(scope=__doc__, verifier_SHA256=sha(Path(__file__).read_bytes()), plan_SHA256=sha(a.plan.read_bytes()),
                  no_expected_values_from_actual=True, no_actual_ID_mapping_or_raw_redecode=True,
                  field_only_scope=True, event_source_PDU_metadata_binding_verified=False)
    try:
        assert sha(a.plan.read_bytes()) == PLAN_SHA
        plan = load_unique(a.plan.read_bytes())
        grid = a.plan.parent / "fields-session-and-typed-source-grid.jsonl"
        assert sha(grid.read_bytes()) == plan["fields_session_grid_SHA256"]
        records = [load_unique(line) for line in grid.read_bytes().splitlines()]
        assert len(records) == 807
        by_source = {source_key(record["source_identity"]): record for record in records}
        assert len(by_source) == 807
        result["validator_negative_controls"] = selftest(records)
        if a.selftest:
            assert a.field_exports is None
            result.update(mode="validator-source-selftest-only", complete_source_records=807, producer_correctness_not_claimed=True)
        else:
            assert a.field_exports is not None
            seen = set(); checks = []
            for line in a.field_exports.read_bytes().splitlines():
                actual = load_unique(line)
                key = source_key(actual["source_identity"])
                assert key in by_source and key not in seen, "unknown or duplicate source identity"
                expected = by_source[key]
                checked = compare_fields(expected, actual["fields"], actual["session"], actual["typed_snapshot"])
                checks.append(dict(source_identity=expected["source_identity"], **checked)); seen.add(key)
            assert seen == set(by_source), ("missing source fields", len(by_source) - len(seen))
            result.update(mode="strict-field-only-actual-export-audit", input_SHA256=sha(a.field_exports.read_bytes()),
                          complete_fields=807, complete_sessions=807, complete_typed_snapshots=807,
                          typed_occurrences=sum(row["typed_occurrences"] for row in checks))
            (a.out / "field-checks.json").write_text(json.dumps(checks, indent=2) + "\n", encoding="utf-8")
        result["passed"] = True
        target = a.out / "summary.json"; target.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(dict(summary=str(target), SHA256=sha(target.read_bytes()), mode=result["mode"], passed=True)))
    except Exception:
        result.update(passed=False, traceback=traceback.format_exc())
        target = a.out / "failure.json"; target.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
        raise

if __name__ == "__main__":
    main()
