#!/usr/bin/env python3
"""Reject case-insensitive path collisions in a Git tree, including directories.

Read Git objects rather than the checkout: a case-insensitive filesystem can
silently merge directories whose distinct files still break Go module zips.
"""

import argparse
from collections import defaultdict
import subprocess
import sys


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tree", nargs="?", default="HEAD", help="Git tree-ish (default: HEAD)")
    args = parser.parse_args()
    result = subprocess.run(
        ["git", "ls-tree", "-rtz", "--full-tree", "--name-only", args.tree, "--"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if result.returncode:
        sys.stderr.buffer.write(result.stderr)
        return 1

    groups = defaultdict(set)
    # -t includes every parent directory; -z preserves whitespace in path names.
    for path in result.stdout.decode("utf-8", errors="surrogateescape").split("\0"):
        if path:
            groups[path.casefold()].add(path)
    collisions = sorted(sorted(paths) for paths in groups.values() if len(paths) > 1)
    for paths in collisions:
        print("case-insensitive path collision: " + " | ".join(repr(path) for path in paths), file=sys.stderr)
    if collisions:
        return 1
    print(f"No case-insensitive file or directory collisions in {args.tree}.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
