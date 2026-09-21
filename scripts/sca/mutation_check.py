#!/usr/bin/env python3
"""Prove SCA tests detect six semantic regressions using isolated Go overlays.

Never edits product files. Requires an offline, already populated Go toolchain.
Logs and mutant copies go to the explicitly supplied output directory.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]
MUTANTS = [
    ("budget", "common/sca/core/budget/context.go", "bytes > maxResult-s.result", "false", "./common/sca/core/budget", "."),
    ("cancel", "common/sca/core/gomod/parse.go", "if ctx == nil {", "ctx = context.Background(); if ctx == nil {", "./common/sca/core/gomod", "^TestMalformedAndBudgets$"),
    ("edge", "common/sca/report.go", "q.Resolved = append(grown, id)", "q.Resolved = grown", "./common/sca", "^TestFourFormatFullFieldContract$/cargo$"),
    ("range-boundary", "common/sca/analyzer/utils_dependence.go", '">= %d.%d.%d && < %d.%d.%d"', '">= %d.%d.%d && <= %d.%d.%d"', "./common/sca/analyzer", "^TestSemverRange$"),
    ("source", "common/sca/report.go", "Source: p.Source, Architecture:", 'Source: "", Architecture:', "./common/sca", "^TestFourFormatFullFieldContract$/cargo$"),
    ("error-code", "common/sca/core/scanerr/error.go", 'return &Error{Code: code, Err: fmt.Errorf(format, args...)}', 'if code == ResourceLimit { code = MalformedInput }; return &Error{Code: code, Err: fmt.Errorf(format, args...)}', "./common/sca", "^TestRPMResourceLimitTyped$"),
]


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--go", default="go")
    ap.add_argument("--out", required=True, type=Path)
    args = ap.parse_args()
    args.out.mkdir(parents=True, exist_ok=True)
    env = dict(os.environ, GOWORK="off", GOPROXY="off", GOSUMDB="off", CGO_ENABLED="0")
    results = []
    for name, file, before, after, package, pattern in MUTANTS:
        source = ROOT / file
        text = source.read_text()
        if before not in text:
            raise RuntimeError("mutation anchor missing: " + name)
        cmd = [args.go, "test", package, "-count=1", "-timeout=90s", "-run", pattern, "-v"]
        baseline = subprocess.run(cmd, cwd=ROOT, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=120)
        (args.out / (name + "-baseline.log")).write_text(baseline.stdout)
        if baseline.returncode or "--- PASS:" not in baseline.stdout:
            raise RuntimeError("baseline test did not execute/pass: " + name)
        mutant = args.out / (name + ".go")
        mutant.write_text(text.replace(before, after, 1))
        overlay = args.out / (name + ".json")
        overlay.write_text(json.dumps({"Replace": {str(source): str(mutant.resolve())}}))
        changed = subprocess.run(cmd + ["-overlay", str(overlay.resolve())], cwd=ROOT, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=120)
        (args.out / (name + "-mutant.log")).write_text(changed.stdout)
        killed = changed.returncode != 0 and "--- FAIL:" in changed.stdout and "[build failed]" not in changed.stdout
        results.append({"mutation": name, "file": file, "test": pattern, "killed_by_assertion": killed})
        print(name + ": " + ("PASS" if killed else "FAIL"), flush=True)
    (args.out / "results.json").write_text(json.dumps(results, indent=2) + "\n")
    if not all(r["killed_by_assertion"] for r in results):
        raise SystemExit(1)


if __name__ == "__main__":
    main()
