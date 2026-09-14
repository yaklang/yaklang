#!/usr/bin/env python3
"""Compare the same public-path benchmark against a frozen git revision.

Run from any directory in this checkout. Uses a Go source overlay, leaving both
the checkout and git index untouched. Only the common benchmark harness is
injected into the baseline; production code and existing tests come from git.
"""

import argparse
import hashlib
import json
import os
import pathlib
import platform
import re
import statistics
import subprocess
import tarfile
import tempfile


def output(*args, cwd=None):
    return subprocess.check_output(args, cwd=cwd, text=True).strip()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", default="dac31e13a332b7e6a16adb8ea70bed1fe926f7fd")
    parser.add_argument("--baseline-archive", help="frozen source tar.gz, instead of git baseline")
    parser.add_argument("--bench", default="Benchmark(TrafficPool|TrafficPoolManyFlows|OpenPcapFile)$")
    parser.add_argument("--extra-harness", action="append", default=[], help="additional repo-relative benchmark file to inject unchanged")
    parser.add_argument("--benchtime", default="200ms")
    parser.add_argument("--count", type=int, default=5)
    parser.add_argument("--output", default="/tmp/pcapx-performance")
    args = parser.parse_args()
    root = pathlib.Path(output("git", "rev-parse", "--show-toplevel"))
    package = pathlib.Path("common/pcapx/pcaputil")
    environment = dict(os.environ, PWD=str(root))
    destination = pathlib.Path(args.output).resolve()
    destination.mkdir(parents=True, exist_ok=True)
    archived = {}
    if args.baseline_archive:
        archive = pathlib.Path(args.baseline_archive).resolve()
        with tarfile.open(archive) as src:
            for member in src.getmembers():
                if not member.isfile() or not member.name.endswith(".go"):
                    continue
                path = pathlib.PurePosixPath(member.name)
                if path.is_absolute() or ".." in path.parts or not path.is_relative_to(package):
                    raise ValueError("source archive entry outside package: " + member.name)
                archived[member.name] = src.extractfile(member).read()
        if not archived:
            raise ValueError("source archive contains no Go files")
        baseline = "archive-sha256:" + hashlib.sha256(archive.read_bytes()).hexdigest()
    else:
        baseline = output("git", "rev-parse", args.baseline, cwd=root)
    harness = package / "reassembly_bench_test.go"
    files = set(archived) if archived else set(output("git", "ls-tree", "-r", "--name-only", baseline, "--", str(package), cwd=root).splitlines())
    harnesses = {str(harness), *args.extra_harness}
    current = {str(path.relative_to(root)) for path in (root / package).rglob("*.go")}
    metadata = {
        "baseline": baseline,
        "working_head": output("git", "rev-parse", "HEAD", cwd=root),
        "go": output("go", "version", cwd=root),
        "platform": platform.platform(),
        "harness_sha256": hashlib.sha256((root / harness).read_bytes()).hexdigest(),
        "extra_harnesses_sha256": {path: hashlib.sha256((root / path).read_bytes()).hexdigest() for path in args.extra_harness},
        "benchmark": args.bench,
        "benchtime": args.benchtime,
        "count": args.count,
        "cwd": str(root),
        "PWD": str(root),
        "sources_sha256": {
            path: hashlib.sha256((root / path).read_bytes()).hexdigest()
            for path in sorted(current)
        },
    }
    (destination / "environment.json").write_text(json.dumps(metadata, indent=2) + "\n")
    common = ["go", "test", "-vet=off", "./" + str(package), "-run", "^$", "-bench",
              args.bench, "-benchmem", "-benchtime=" + args.benchtime, "-count=" + str(args.count)]
    # Vet in Go 1.22 can resolve imports from the physical file rather than its
    # overlay. Normal go test/go vet on the final checkout is a separate check.
    with tempfile.TemporaryDirectory(prefix="pcapx-baseline-") as scratch_dir:
        scratch = pathlib.Path(scratch_dir)
        mapping = {}
        for path in sorted(files | current):
            if not path.endswith(".go") or path in harnesses:
                continue
            if path in files:
                original = scratch / path
                original.parent.mkdir(parents=True, exist_ok=True)
                original.write_bytes(archived[path] if archived else subprocess.check_output(["git", "show", baseline + ":" + path], cwd=root))
                mapping[str(root / path)] = str(original)
            else:
                mapping[str(root / path)] = ""
        overlay = scratch / "overlay.json"
        overlay.write_text(json.dumps({"Replace": mapping}))
        for label in ("before", "after"):
            command = common + ["-o", str(scratch / (label + ".test")),
                                "-cpuprofile=" + str(destination / (label + ".cpu")),
                                "-memprofile=" + str(destination / (label + ".mem"))]
            if label == "before":
                command.insert(2, "-overlay=" + str(overlay))
            print("Running", label, flush=True)
            with (destination / (label + ".txt")).open("w") as result:
                subprocess.run(command, cwd=root, env=environment, stdout=result, check=True)
    samples = []
    pattern = re.compile(r"(Benchmark\S+)\s+\d+\s+(\d+) ns/op\s+([\d.]+) MB/s\s+(\d+) B/op\s+(\d+) allocs/op")
    for label in ("before", "after"):
        rows = {}
        for line in (destination / (label + ".txt")).read_text().splitlines():
            match = pattern.match(line)
            if match:
                rows.setdefault(match[1], []).append([float(v) for v in match.groups()[1:]])
        samples.append({key: [statistics.median(row[i] for row in data) for i in range(4)] for key, data in rows.items()})
    if not samples[0] or samples[0].keys() != samples[1].keys():
        raise ValueError("baseline/current benchmark sets are empty or different")
    comparison = {key: {"before": values, "after": samples[1][key], "speedup": values[0] / samples[1][key][0]}
                  for key, values in samples[0].items()}
    (destination / "medians.json").write_text(json.dumps(comparison, indent=2) + "\n")
    print(json.dumps(comparison, indent=2))


if __name__ == "__main__":
    main()
