"""Prove only the pre-runtime typed exact_value contract policy changed.

The first contract was supplied incorrectly. This is a correction against the
immutable existing a82 public API, not a candidate parser/runtime failure.
All 807 source fields/session trees and every other field-grid property must
remain literal, including all range leaves and duplicate occurrence order.
"""
from pathlib import Path
import copy, hashlib, json, subprocess

ROOT = Path(__file__).resolve().parent
OLD = ROOT.parent / "OSPFv3-field-plan-before-native-01"
REPO = Path(r"C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang")

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False, allow_nan=False)

assert sha((OLD / "manifest01.json").read_bytes()) == "890463948bf0e78a7096a90940f43c1e2696ff35ebdd8a1aec9212307151cdde"
for name, pin in json.loads((OLD / "manifest01.json").read_bytes())["files"].items():
    path = OLD / name
    assert path.stat().st_size == pin["bytes"] and sha(path.read_bytes()) == pin["SHA256"]
old_contract = json.loads((OLD / "frozen01/frozen-pure-contract.json").read_bytes())
new_contract = json.loads((ROOT / "frozen02/frozen-pure-contract.json").read_bytes())
assert sha((ROOT / "frozen02/frozen-pure-contract.json").read_bytes()) == "ae0359a175fdf9fe8350f37284a1398ca85904fce75b5153c08aec9f9c601a7d"
for key in old_contract:
    if key != "unsigned_json_contract":
        assert canonical(old_contract[key]) == canonical(new_contract[key]), key
correction = new_contract["correction_source"]
raw = subprocess.check_output(["git", "show", correction["original_immutable_source"] + ":" + correction["file"]], cwd=REPO)
assert sha(raw) == correction["file_SHA256"] == "a79a8003be2a96b80c28ac05ec0e1ff9986eec191edb6b0d382e4391804b2b5c"
assert correction["original_policy"].encode() in raw
old_grid = OLD / "frozen01/fields-session-and-typed-source-grid.jsonl"
new_grid = ROOT / "frozen02/fields-session-and-typed-source-grid.jsonl"
old_rows = [json.loads(line) for line in old_grid.read_bytes().splitlines()]
new_rows = [json.loads(line) for line in new_grid.read_bytes().splitlines()]
assert len(old_rows) == len(new_rows) == 807
bits_by_name = {row[0]: row[3] for row in new_contract["unsigned"]}
deleted = collections_count = 0
per_frame = []
for before, after in zip(old_rows, new_rows):
    assert before["source_identity"] == after["source_identity"]
    # Literal dictionary serialization preserves insertion order as well as
    # JSON type/value; neither sorted equality nor raw redecoding hides a delta.
    for key in ("expected_fields", "expected_session"):
        assert json.dumps(before[key], ensure_ascii=False, separators=(",", ":")) == json.dumps(after[key], ensure_ascii=False, separators=(",", ":"))
    changed = copy.deepcopy(before)
    count = 0
    for name, values in changed["expected_typed_values_by_name"].items():
        if name not in bits_by_name or bits_by_name[name] > 53:
            continue
        for value in values:
            assert value.pop("exact_value") == str(value["value"])
            count += 1
    assert canonical(changed) == canonical(after), ("unplanned source-grid change", before["source_identity"])
    deleted += count
    per_frame.append(dict(source_identity=before["source_identity"], fields_literal_equal=True, session_literal_equal=True,
                          exact_value_only_removed=count, every_other_source_grid_value_equal=True))
result = dict(scope=__doc__, bridge_source_SHA256=sha(Path(__file__).read_bytes()),
              old_plan_SHA256=sha((OLD / "frozen01/plan.json").read_bytes()),
              new_plan_SHA256=sha((ROOT / "frozen02/plan.json").read_bytes()),
              old_grid_SHA256=sha(old_grid.read_bytes()), new_grid_SHA256=sha(new_grid.read_bytes()),
              old_pure_contract_SHA256=sha((OLD / "frozen01/frozen-pure-contract.json").read_bytes()),
              new_pure_contract_SHA256=sha((ROOT / "frozen02/frozen-pure-contract.json").read_bytes()),
              immutable_existing_API_source=correction, immutable_API_policy_literal_verified=True,
              complete_source_frames=807, complete_fields_trees_literal_equal=807, complete_session_trees_literal_equal=807,
              exact_value_only_deletions=deleted, native_or_candidate_output_read=False,
              classification="root pre-runtime pure-contract correction; not product parsing bug or candidate runtime failure",
              source_values_ranges_occurrence_order_unchanged=True, per_frame=per_frame)
target = ROOT / "bridge02.json"
assert not target.exists()
target.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
print(json.dumps(dict(bridge=str(target), SHA256=sha(target.read_bytes()), frames=807,
                      fields_literal=807, session_literal=807, only_exact_value_deletions=deleted)))
