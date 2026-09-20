#!/usr/bin/env python3
"""Audit complete Go package streams; errors never become zero-dependency reports.
Default is a failing release gate until every public SCA root is clean.
--inventory records pending work without claiming the release gate passed.
"""
import argparse
import json
import os
import subprocess
import sys
from pathlib import Path

OWN = "github.com/yaklang/yaklang/common/sca"
LEAF = "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"

def decode_stream(text):
    decoder = json.JSONDecoder()
    objects = []
    rest = text.lstrip()
    while rest:
        obj, end = decoder.raw_decode(rest)
        objects.append(obj)
        rest = rest[end:].lstrip()
    if not objects:
        raise ValueError("empty go list output")
    return objects

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--inventory", action="store_true")
    parser.add_argument("--test", action="store_true")
    parser.add_argument("--output", type=Path)
    parser.add_argument("roots", nargs="*", default=["./common/sca/..."])
    args = parser.parse_args()
    cmd = ["go", "list", "-mod=readonly", "-deps", "-json"]
    if args.test:
        cmd.append("-test")
    cmd.extend(args.roots)
    env = dict(os.environ, GOWORK="off", GOTOOLCHAIN="local", CGO_ENABLED="0")
    # Go emits UTF-8 JSON (including package documentation), independently of
    # Windows' active ANSI code page.
    result = subprocess.run(cmd, env=env, text=True, encoding="utf-8", capture_output=True)
    if result.returncode:
        sys.stderr.write(result.stderr)
        return 2
    try:
        packages = decode_stream(result.stdout)
        failures = [(p.get("ImportPath"), p.get("Error"), p.get("DepsErrors"))
                    for p in packages if p.get("Error") or p.get("DepsErrors")]
        if failures:
            raise ValueError(f"package graph errors: {failures}")
    except (ValueError, TypeError) as error:
        sys.stderr.write(str(error) + "\n")
        return 2
    external, forbidden, cgo = {}, [], []
    for pkg in packages:
        path = pkg["ImportPath"].split(" [", 1)[0]
        # go list -test emits generated test-main packages, not source roots.
        if args.test and pkg.get("Name") == "main" and path.endswith(".test"):
            tested = path[:-5]
            if tested == OWN or tested.startswith(OWN + "/"):
                path = tested
        if pkg.get("Standard"):
            continue
        module = pkg.get("Module", {})
        if module.get("Path") != "github.com/yaklang/yaklang":
            external.setdefault(module.get("Path", "UNKNOWN:" + path), []).append(path)
        elif not (path == OWN or path.startswith(OWN + "/") or path == LEAF):
            forbidden.append(path)
        if pkg.get("CgoFiles"):
            cgo.append(path)
    report = {
        "roots": args.roots, "test_closure": args.test,
        "package_count": len(packages), "third_party_module_count": len(external),
        "external_modules": {k: sorted(v) for k, v in sorted(external.items())},
        "forbidden_internal": sorted(forbidden), "cgo_packages": sorted(cgo),
        "passed": not external and not forbidden and not cgo,
        "classification": "PACKAGE_CLOSURE_ONLY_NOT_FULL_RELEASE_ACCEPTANCE",
    }
    serialized = json.dumps(report, indent=2) + "\n"
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(serialized)
    print(serialized, end="")
    return 0 if args.inventory or report["passed"] else 1

if __name__ == "__main__":
    sys.exit(main())
