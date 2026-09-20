#!/usr/bin/env python3
"""Verify checked-in source/fixture provenance, without fetching any references."""
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCA = ROOT / "common/sca"


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    errors = []
    sources = json.loads((SCA / "source-extraction-map.json").read_text(encoding="utf-8"))["entries"]
    fixtures = json.loads((SCA / "testdata/manifest.json").read_text(encoding="utf-8"))["fixtures"]
    for base, rows, key in [(ROOT, sources, "local_file"), (SCA, fixtures, "path")]:
        for row in rows:
            path = base / row[key]
            if not path.is_file() or digest(path) != row["sha256"]:
                errors.append("missing or stale hash: " + row[key])
    candidates = json.loads((SCA / "test-migration-map.json").read_text(encoding="utf-8"))["candidates"]
    for candidate in candidates:
        for target in candidate["local_tests"]:
            if not (SCA / target.split(":", 1)[0]).is_file():
                errors.append("missing mapped test: " + target)
    actual = {p.relative_to(ROOT).as_posix() for p in SCA.rglob("*.go") if not p.name.endswith("_test.go") and "testdata" not in p.parts and "testcheck" not in p.parts}
    mapped = {row["local_file"] for row in sources}
    errors += ["unmapped source: " + path for path in sorted(actual - mapped)]
    contracts = json.loads((SCA / "function-contracts.json").read_text(encoding="utf-8"))["contracts"]
    if len(contracts) != 20 or len({x["analyzer"] for x in contracts}) != 20:
        errors.append("the frozen set must contain 20 distinct analyzers")
    print(json.dumps({"passed": not errors, "sources": len(sources), "fixtures": len(fixtures), "test_candidates": len(candidates), "errors": errors,
                      "scope": "hashes and mapping targets only; not independent semantic acceptance"}, indent=2))
    return bool(errors)


if __name__ == "__main__":
    raise SystemExit(main())
