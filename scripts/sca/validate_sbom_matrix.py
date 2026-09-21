#!/usr/bin/env python3
"""Validate all frozen-format BOMs against supplied, pinned local schemas.

Development-only: requires jsonschema in a separate tool environment. No
downloads or network schema resolution, and no product runtime dependency.
Generate input with SCA_MATRIX_FULL_FIELDS=1 run_format_scan_matrix.sh.
"""
import argparse
import copy
import json
from pathlib import Path

from jsonschema import Draft7Validator, RefResolver


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("matrix", type=Path)
    parser.add_argument("schemas", type=Path)
    args = parser.parse_args()
    schema = json.loads((args.schemas / "bom-1.5.schema.json").read_text())
    store = {}
    for file in args.schemas.glob("*.json"):
        value = json.loads(file.read_text())
        store["http://cyclonedx.org/schema/" + file.name] = value
        store["https://cyclonedx.org/schema/" + file.name] = value
        if "$id" in value:
            store[value["$id"]] = value

    def reject_network(url):
        raise ValueError("unresolved local schema; network prohibited: " + url)

    validator = Draft7Validator(schema, resolver=RefResolver.from_schema(
        schema, store=store, handlers={"http": reject_network, "https": reject_network}))
    rows = json.loads(args.matrix.read_text())["rows"]
    if len(rows) != 20 or len({row["case"] for row in rows}) != 20:
        raise ValueError("expected all 20 frozen format results")
    for row in rows:
        result = row["new"]
        if not result["complete"] or not result["ok"]:
            raise ValueError("incomplete scan: " + row["case"])
        bom = result["sbom"]
        if bom.get("specVersion") != "1.5" or bom.get("bomFormat") != "CycloneDX":
            raise ValueError("fixed output contract mismatch: " + row["case"])
        validator.validate(bom)
        broken = copy.deepcopy(bom)
        # The pinned schema allows arbitrary specVersion strings; its type is
        # constrained. The exact value is independently checked above.
        broken["specVersion"] = 15
        if not list(validator.iter_errors(broken)):
            raise ValueError("schema negative control did not fail")
        print(row["case"] + ": PASS (including negative control)")


if __name__ == "__main__":
    main()
