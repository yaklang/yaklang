#!/usr/bin/env python3
"""
ssa-tree.py — Print the SSA database program tree structure.

Shows base program, overlay chain (promote layers), and per-PR diff programs,
annotated with whether they belong to the main database or a temporary clone.

Usage:
  python3 ssa-tree.py [--data-dir ./ci-ssa-data] [--worktree ~/yaklang_workspace/yhellow-ssa-incremental]
"""
import argparse
import json
import os
import subprocess
import sys
from pathlib import Path
from datetime import datetime, timezone

DEFAULT_DATA_DIR = "./ci-ssa-data"
DEFAULT_WORKTREE = os.path.expanduser("~/yaklang_workspace/yhellow-ssa-incremental")


def run_yak_ssa_program(db_path: str, yak_bin: str) -> list[str]:
    """Get program list from yak ssa-program."""
    try:
        result = subprocess.run(
            [yak_bin, "ssa-program", "--database", db_path],
            capture_output=True, text=True, timeout=30, check=True,
        )
        programs = []
        for line in result.stdout.splitlines():
            line = line.strip()
            if line.startswith("[golang]:"):
                name = line.replace("[golang]:", "").strip()
                if name:
                    programs.append(name)
        return programs
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        return []


def read_manifest(data_dir: Path) -> dict | None:
    manifest_path = data_dir / "manifest.json"
    if not manifest_path.exists():
        return None
    try:
        return json.loads(manifest_path.read_text())
    except Exception:
        return None


def read_pointer(data_dir: Path) -> str:
    pointer_path = data_dir / "base-program-name"
    if pointer_path.exists():
        return pointer_path.read_text().strip()
    return ""


def find_clone_dirs() -> list[Path]:
    """Find /tmp/ci-promote-clone-* directories."""
    import glob
    return [Path(d) for d in sorted(glob.glob("/tmp/ci-promote-clone-*")) if Path(d).is_dir()]


def classify_program(name: str, manifest_base: str, pointer_base: str) -> str:
    """Classify a program's role in the tree."""
    if name == "ci-yaklang-base":
        return "full-baseline"
    if name.startswith("ci-yaklang-promote-"):
        if name == manifest_base:
            return "current-base (manifest)"
        if name == pointer_base:
            return "current-base (pointer)"
        return "promote-overlay"
    if name.startswith("ci-yaklang-flat-"):
        return "flattened"
    if name.startswith("ci-yaklang-diff-pr-"):
        return "pr-diff (temporary)"
    return "unknown"


def parse_pr_number(name: str) -> int | None:
    """Extract PR number from ci-yaklang-diff-pr-{N}-{sha}."""
    if not name.startswith("ci-yaklang-diff-pr-"):
        return None
    rest = name[len("ci-yaklang-diff-pr-"):]
    parts = rest.split("-", 1)
    try:
        return int(parts[0])
    except (ValueError, IndexError):
        return None


def build_promote_chain(programs: list[str]) -> list[str]:
    """Build the promote overlay chain in order (base → top)."""
    promote_progs = sorted(
        [p for p in programs if p.startswith("ci-yaklang-promote-")],
        key=lambda p: p,  # sorted by name (sha suffix gives chronological order)
    )
    return promote_progs


def print_tree(
    programs: list[str],
    manifest: dict | None,
    pointer_base: str,
    db_label: str,
    is_main: bool,
):
    """Print the program tree."""
    manifest_base = manifest.get("base_program_name", "") if manifest else ""
    main_sha = manifest.get("main_sha", "")[:12] if manifest else ""
    depth = manifest.get("overlay_depth", 0) if manifest else 0

    db_icon = "📦" if is_main else "📋"
    print(f"\n{db_icon} {db_label}")
    print(f"   DB: {'主数据库 (main)' if is_main else '临时数据库 (clone)'}")

    if manifest:
        print(f"   manifest: base={manifest_base} main_sha={main_sha}... depth={depth}")
    if pointer_base:
        print(f"   pointer: {pointer_base}")

    if not programs:
        print("   (no programs)")
        return

    # Build the tree
    base_progs = [p for p in programs if p == "ci-yaklang-base" or p.startswith("ci-yaklang-flat-")]
    promote_chain = build_promote_chain(programs)
    diff_progs = sorted([p for p in programs if p.startswith("ci-yaklang-diff-pr-")])
    unknown_progs = [p for p in programs if p not in base_progs and p not in promote_chain and p not in diff_progs]

    # Print base
    for p in base_progs:
        role = classify_program(p, manifest_base, pointer_base)
        marker = " ← current base" if "current-base" in role else ""
        print(f"   └── {p}  [{role}]{marker}")

    # Print promote chain
    if promote_chain:
        print(f"   └── promote overlay chain ({len(promote_chain)} layers):")
        for i, p in enumerate(promote_chain):
            role = classify_program(p, manifest_base, pointer_base)
            is_current = "current-base" in role
            marker = " ← current base" if is_current else ""
            print(f"       {'└──' if i == len(promote_chain)-1 else '├──'} [{i+1}] {p}  [{role}]{marker}")

    # Print diff programs (temporary)
    if diff_progs:
        print(f"   └── PR diff programs ({len(diff_progs)} temporary):")
        # Group by PR number
        pr_groups: dict[int, list[str]] = {}
        for p in diff_progs:
            pr_num = parse_pr_number(p) or 0
            pr_groups.setdefault(pr_num, []).append(p)
        for pr_num in sorted(pr_groups.keys()):
            progs = pr_groups[pr_num]
            print(f"       └── PR #{pr_num} ({len(progs)} program(s)):")
            for p in progs:
                print(f"           └── {p}  [pr-diff]")

    if unknown_progs:
        print(f"   └── unknown ({len(unknown_progs)}):")
        for p in unknown_progs:
            print(f"       └── {p}")


def main():
    parser = argparse.ArgumentParser(description="Print SSA database program tree")
    parser.add_argument("--data-dir", default=DEFAULT_DATA_DIR, help="Path to ci-ssa-data dir")
    parser.add_argument("--worktree", default=DEFAULT_WORKTREE, help="Path to yaklang worktree")
    args = parser.parse_args()

    data_dir = Path(os.path.expanduser(args.data_dir)).resolve()
    worktree = Path(os.path.expanduser(args.worktree)).resolve()
    yak_bin = worktree / "yak"

    if not yak_bin.exists():
        print(f"Error: yak binary not found: {yak_bin}", file=sys.stderr)
        sys.exit(1)

    db_path = data_dir / "default-yakssa.db"
    if not db_path.exists():
        print(f"Error: database not found: {db_path}", file=sys.stderr)
        sys.exit(1)

    manifest = read_manifest(data_dir)
    pointer_base = read_pointer(data_dir)

    # Main database
    programs = run_yak_ssa_program(str(db_path), str(yak_bin))
    print_tree(programs, manifest, pointer_base, str(db_path), is_main=True)

    # Check for temporary clone databases
    clone_dirs = find_clone_dirs()
    for clone_dir in clone_dirs:
        clone_data = clone_dir / "ci-ssa-data"
        if clone_data.is_symlink():
            # It's a symlink to the main data dir — same DB, skip
            target = Path(os.readlink(clone_data))
            if target.resolve() == data_dir:
                print(f"\n📋 {clone_dir}")
                print(f"   clone ci-ssa-data → symlink to main {target}")
                print(f"   (same database as main, not a temporary DB)")
                continue
        clone_db = clone_data / "default-yakssa.db"
        if clone_db.exists() and not clone_data.is_symlink():
            clone_programs = run_yak_ssa_program(str(clone_db), str(yak_bin))
            print_tree(clone_programs, None, "", str(clone_db), is_main=False)
        elif clone_db.exists() and clone_data.is_symlink():
            print(f"\n📋 {clone_dir}")
            print(f"   ci-ssa-data → {os.readlink(clone_data)}")
            print(f"   (symlink to main database, same programs as above)")

    print()


if __name__ == "__main__":
    main()