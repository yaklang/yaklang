#!/usr/bin/env python3
"""Normalize concatenated irify-full snapshots without dropping findings.

Some scanners append cumulative snapshots to a .json file. Merge by immutable
risk hash, preserve all rule/file records, and reject inconsistent duplicates.
Severity and the repository's downstream gate policy are unchanged.
"""
import json
import pathlib
import sys


def normalize(text):
    decoder = json.JSONDecoder()
    documents = []
    offset = 0
    while offset < len(text):
        while offset < len(text) and text[offset].isspace():
            offset += 1
        if offset == len(text):
            break
        document, offset = decoder.raw_decode(text, offset)
        if not isinstance(document, dict):
            raise ValueError("Risk report must contain JSON objects")
        risks = document.get("Risks")
        count = document.get("RiskNums")
        if not isinstance(risks, dict) or type(count) is not int or count != len(risks):
            raise ValueError("RiskNums must equal the number of risk hashes")
        documents.append(document)
    if not documents:
        raise ValueError("Empty scanner report")
    result = dict(documents[-1])
    result["Risks"] = {}
    for document in documents:
        if document.get("report_type") != "irify-full":
            raise ValueError("Unsupported scanner report type")
        for key, finding in document["Risks"].items():
            if not isinstance(finding, dict) or finding.get("hash") != key:
                raise ValueError("Missing or inconsistent finding hash")
            if key in result["Risks"] and result["Risks"][key] != finding:
                raise ValueError(f"Conflicting finding snapshots: {key}")
            result["Risks"][key] = finding
    for name in ("Rules", "File"):
        values, seen = [], set()
        for document in documents:
            records = document.get(name, [])
            if not isinstance(records, list):
                raise ValueError(f"{name} must be a list")
            for record in records:
                identity = json.dumps(record, ensure_ascii=False, sort_keys=True)
                if identity not in seen:
                    seen.add(identity)
                    values.append(record)
        result[name] = values
    result["RiskNums"] = len(result["Risks"])
    return result


if __name__ == "__main__":
    if len(sys.argv) != 3:
        raise SystemExit("usage: normalize-report.py INPUT OUTPUT")
    source, destination = map(pathlib.Path, sys.argv[1:])
    report = normalize(source.read_text(encoding="utf-8"))
    destination.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"Preserved {report['RiskNums']} unique findings")
