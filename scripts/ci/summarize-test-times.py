#!/usr/bin/env python3
"""Show actual leaf-case durations without double counting parent subtests."""
import re
import sys

results = {}
with open(sys.argv[1], encoding="utf-8", errors="replace") as log:
    for line in log:
        match = re.match(r"\s*--- (?:PASS|FAIL|SKIP): (\S+) \(([\d.]+)s\)", line)
        if match:
            results[match[1]] = float(match[2])
parents = {
    "/".join(parts[:i])
    for name in results
    for parts in [name.split("/")]
    for i in range(1, len(parts))
}
leaves = [(seconds, name) for name, seconds in results.items() if name not in parents]
print("Slowest test cases (excluding parent totals):")
for seconds, name in sorted(leaves, reverse=True)[:10]:
    print(f"  {seconds:.2f}s  {name}")
slow = [(seconds, name) for seconds, name in leaves if seconds > 3]
if slow:
    print(f"WARNING: {len(slow)} test cases exceed the 3s runtime target")
