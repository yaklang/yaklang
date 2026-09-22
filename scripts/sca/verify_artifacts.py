#!/usr/bin/env python3
"""Verify checked-in source/fixture provenance, without fetching any references."""
import hashlib
import json
from pathlib import Path
from fixture_store import FixtureStore

ROOT = Path(__file__).resolve().parents[2]
SCA = ROOT / "common/sca"


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


NEED_TESTS = {"ADAPTED", "SEMANTIC_REPLACEMENT", "MIGRATED_AS_IS"}


def check_candidate_mapping(candidate, errors):
    tests = candidate.get("local_tests") or []
    disp = candidate.get("disposition")
    source = candidate.get("source", "")
    case = candidate.get("case", "")
    if disp in NEED_TESTS and not tests:
        errors.append("unmapped %s candidate: %s:%s" % (disp, source, case))
    for target in tests:
        rel, _, symbol = target.partition(":")
        path = SCA / rel
        if not path.is_file():
            errors.append("missing mapped test: " + target)
            continue
        if not symbol:
            continue
        text = path.read_text(encoding="utf-8")
        parent = symbol.split("/", 1)[0]
        if parent.startswith("Test") or parent.startswith("Benchmark"):
            if "func " + parent + "(" not in text:
                errors.append("mapped test missing func: " + target)
                continue
        if "/" not in symbol:
            continue
        sub = symbol.split("/", 1)[1]
        # Simple table/t.Run names must appear as source literals so a
        # mapping cannot point at an unrelated file. Nested corpus paths
        # (TOML walk, etc.) are generated at runtime and only the parent
        # test is required.
        if "/" in sub or "." in sub:
            continue
        if '"%s"' % sub not in text and "`%s`" % sub not in text:
            errors.append("mapped subtest missing: " + target)


def main():
    errors = []
    sources = json.loads((SCA / "source-extraction-map.json").read_text(encoding="utf-8"))["entries"]
    fixtures = json.loads((SCA / "testdata/manifest.json").read_text(encoding="utf-8"))["fixtures"]
    try:
        FixtureStore(SCA).verify()
    except (ValueError, OSError, KeyError) as exc:
        errors.append(str(exc))
    for base, rows, key in [(ROOT, sources, "local_file")]:
        for row in rows:
            path = base / row[key]
            if not path.is_file() or digest(path) != row["sha256"]:
                errors.append("missing or stale hash: " + row[key])
    candidates = json.loads((SCA / "test-migration-map.json").read_text(encoding="utf-8"))["candidates"]
    for candidate in candidates:
        check_candidate_mapping(candidate, errors)
    probe = []
    check_candidate_mapping({"disposition": "ADAPTED", "source": "probe", "case": "empty", "local_tests": []}, probe)
    if not any(e.startswith("unmapped ADAPTED candidate: probe:empty") for e in probe):
        errors.append("empty-mapping gate is inert")
    actual = {p.relative_to(ROOT).as_posix() for p in SCA.rglob("*.go") if not p.name.endswith("_test.go") and "testdata" not in p.parts and "testcheck" not in p.parts}
    mapped = {row["local_file"] for row in sources}
    errors += ["unmapped source: " + path for path in sorted(actual - mapped)]
    contracts = json.loads((SCA / "function-contracts.json").read_text(encoding="utf-8"))["contracts"]
    if len(contracts) != 20 or len({x["analyzer"] for x in contracts}) != 20:
        errors.append("the frozen set must contain 20 distinct analyzers")
    additional = json.loads((SCA / "function-contracts.json").read_text(encoding="utf-8")).get("additional_contracts", [])
    if {x["analyzer"] for x in additional} != {"nuget-lang", "swift-lang", "python-uv-lang", "bun-lang"} or len(additional) != 4:
        errors.append("expected four explicitly added lock analyzers")
    for row in additional:
        if row["analyzer"] in {x["analyzer"] for x in contracts}:
            errors.append("additional analyzer replaces a frozen contract")
        for path in [row["implementation"], *row["tests"], *row["additional_fixture_archives"]]:
            if not (SCA / path).is_file():
                errors.append("missing additional contract artifact: " + path)
    print(json.dumps({"passed": not errors, "sources": len(sources), "fixtures": len(fixtures), "test_candidates": len(candidates), "errors": errors,
                      "scope": "hashes and mapping targets only; not independent semantic acceptance"}, indent=2))
    return bool(errors)


if __name__ == "__main__":
    raise SystemExit(main())
