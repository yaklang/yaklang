#!/usr/bin/env python3
from __future__ import annotations

import sys
import tomllib
from pathlib import Path


REQUIRED = (
    "instruction.md",
    "task.toml",
    "environment/Dockerfile",
    "solution/solve.sh",
    "tests/test.sh",
    "tests/verify.py",
)


def main() -> int:
    dataset = Path(__file__).resolve().parents[1] / "datasets" / "yak-agent-v1"
    errors: list[str] = []
    task_dirs = sorted(path.parent for path in dataset.glob("*/task.toml"))
    if not task_dirs:
        errors.append("dataset contains no tasks")

    names: set[str] = set()
    for task_dir in task_dirs:
        for relative in REQUIRED:
            if not (task_dir / relative).is_file():
                errors.append(f"{task_dir.name}: missing {relative}")
        try:
            config = tomllib.loads((task_dir / "task.toml").read_text())
        except Exception as exc:
            errors.append(f"{task_dir.name}: invalid task.toml: {exc}")
            continue
        name = config.get("task", {}).get("name")
        if not name:
            errors.append(f"{task_dir.name}: missing task.name")
        elif name in names:
            errors.append(f"{task_dir.name}: duplicate task.name {name}")
        names.add(name)
        # Note: we no longer require environment network_mode="no-network"
        # because the AI agent needs internet access for API calls.
        instruction = (task_dir / "instruction.md").read_text()
        if "/app/" not in instruction:
            errors.append(f"{task_dir.name}: instruction has no output path")
        test_text = (task_dir / "tests" / "verify.py").read_text()
        for reward_key in ("reward", "outcome", "evidence", "format"):
            if reward_key not in test_text:
                errors.append(f"{task_dir.name}: verifier lacks {reward_key} reward")

    if errors:
        print("\n".join(f"ERROR: {error}" for error in errors), file=sys.stderr)
        return 1
    print(f"validated {len(task_dirs)} Harbor tasks under {dataset}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

