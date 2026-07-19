#!/usr/bin/env python3
"""Fast structural smoke test for Yak Agent benchmark tasks.

Validates all task definitions, oracle solutions, and verifier logic
**without** Docker, Harbor, or an AI provider.  Run this before any
expensive container-based benchmark to catch structural issues early.

Usage::

    python3 benchmarks/harbor/scripts/smoke_test.py

Checks performed:
1. Every task has all required files.
2. Task TOML is valid and complete.
3. Oracle ``solve.sh`` passes ``bash -n`` syntax check.
4. Verifier ``verify.py`` contains required reward fields and the
   standard scoring formula.
5. Yak and OpenCode runner argument parsing is functional.
"""
from __future__ import annotations

import argparse
import ast
import re
import subprocess
import sys
import tomllib
from pathlib import Path


REQUIRED_FILES = (
    "instruction.md",
    "task.toml",
    "environment/Dockerfile",
    "solution/solve.sh",
    "tests/test.sh",
    "tests/verify.py",
)

REQUIRED_TOML_SECTIONS = ("task", "agent", "verifier", "environment")


def _red(text: str) -> str:
    return f"\033[31m{text}\033[0m"


def _green(text: str) -> str:
    return f"\033[32m{text}\033[0m"


def _bold(text: str) -> str:
    return f"\033[1m{text}\033[0m"


def _yellow(text: str) -> str:
    return f"\033[33m{text}\033[0m"


# ---------------------------------------------------------------------------
# 1. Structural validation
# ---------------------------------------------------------------------------

def check_structural(dataset: Path) -> list[str]:
    """Validate that every task has all required files and valid TOML."""
    errors: list[str] = []
    task_dirs = sorted(p.parent for p in dataset.glob("*/task.toml"))
    if not task_dirs:
        errors.append("dataset contains no tasks")
        return errors

    names: set[str] = set()
    for task_dir in task_dirs:
        label = task_dir.name
        for relative in REQUIRED_FILES:
            if not (task_dir / relative).is_file():
                errors.append(f"{label}: missing {relative}")

        try:
            config = tomllib.loads((task_dir / "task.toml").read_text())
        except Exception as exc:
            errors.append(f"{label}: invalid task.toml: {exc}")
            continue

        name = config.get("task", {}).get("name")
        if not name:
            errors.append(f"{label}: missing task.name")
        elif name in names:
            errors.append(f"{label}: duplicate task.name {name!r}")
        if name:
            names.add(name)

        for section in REQUIRED_TOML_SECTIONS:
            if section not in config:
                errors.append(f"{label}: missing [{section}] section")

        agent_timeout = config.get("agent", {}).get("timeout_sec")
        if agent_timeout is not None and agent_timeout < 60:
            errors.append(f"{label}: agent timeout too short ({agent_timeout}s)")

        # Note: we no longer require environment network_mode="no-network"
        # because the AI agent needs internet access for API calls.
        # The verifier still benefits from no-network isolation.

        instruction = (task_dir / "instruction.md").read_text()
        if "/app/" not in instruction:
            errors.append(f"{label}: instruction has no output path under /app/")

        solve_path = task_dir / "solution" / "solve.sh"
        if solve_path.is_file():
            if not (solve_path.stat().st_mode & 0o111):
                errors.append(f"{label}: solution/solve.sh is not executable")

    return errors


# ---------------------------------------------------------------------------
# 2. Oracle syntax check
# ---------------------------------------------------------------------------

def check_oracle_syntax(task_dir: Path) -> list[str]:
    """Run ``bash -n`` on solve.sh to catch syntax errors."""
    errors: list[str] = []
    label = task_dir.name
    solve_path = task_dir / "solution" / "solve.sh"

    if not solve_path.is_file():
        return [f"{label}: solution/solve.sh missing"]

    result = subprocess.run(
        ["bash", "-n", str(solve_path)],
        capture_output=True,
        text=True,
        timeout=10,
    )
    if result.returncode != 0:
        errors.append(
            f"{label}: oracle syntax error: {result.stderr.strip()[:200]}"
        )
    return errors


# ---------------------------------------------------------------------------
# 3. Verifier logic validation
# ---------------------------------------------------------------------------

def check_verifier_logic(task_dir: Path) -> list[str]:
    """Validate verifier scoring formula via static analysis.

    Checks that ``verify.py``:
    - Has ``outcome``, ``evidence``, ``format`` scoring dimensions.
    - Computes ``reward = outcome * <w1> + evidence * <w2> + format * <w3>``.
    - Weights sum to approximately 1.0 (or near 1.0).
    """
    errors: list[str] = []
    label = task_dir.name
    verify_path = task_dir / "tests" / "verify.py"

    if not verify_path.is_file():
        return [f"{label}: tests/verify.py missing"]

    source = verify_path.read_text()

    # Check required scoring dimensions
    for key in ("outcome", "evidence", "format", "reward"):
        if key not in source:
            errors.append(f"{label}: verifier lacks '{key}' scoring field")
            return errors

    # Parse the Python AST and find the scoring formula
    try:
        tree = ast.parse(source)
    except SyntaxError as exc:
        errors.append(f"{label}: verifier syntax error: {exc}")
        return errors

    # Extract weights from the reward formula (regex fallback)
    weight_pattern = re.findall(
        r'(outcome|evidence|format)\s*\*\s*([\d.]+)', source
    )
    weights: dict[str, float] = {}
    for dim, w in weight_pattern:
        weights[dim] = float(w)

    if weights:
        total = sum(weights.values())
        if not (0.95 <= total <= 1.05):
            errors.append(
                f"{label}: verifier weights sum to {total}, expected ~1.0"
            )
        for dim in ("outcome", "evidence", "format"):
            if dim not in weights:
                errors.append(f"{label}: verifier missing '{dim}' weight")
    else:
        # Some verifiers might use a different pattern; warn but don't fail
        if "reward" in source:
            pass  # reward formula exists, just couldn't parse weights

    # Check that it writes to the correct path
    if "/logs/verifier/reward.json" not in source:
        errors.append(
            f"{label}: verifier should write to /logs/verifier/reward.json"
        )

    return errors


# ---------------------------------------------------------------------------
# 4. Gateway runner check
# ---------------------------------------------------------------------------

def check_gateway_runner() -> list[str]:
    """Validate gateway_runner.py argument parsing."""
    errors: list[str] = []
    runner_path = (
        Path(__file__).resolve().parents[1] / "agents" / "gateway_runner.py"
    )

    if not runner_path.is_file():
        return ["gateway_runner.py not found"]

    # Syntax check
    result = subprocess.run(
        [sys.executable, "-c", f"compile(open({str(runner_path)!r}).read(), {str(runner_path)!r}, 'exec')"],
        capture_output=True,
        text=True,
        timeout=10,
    )
    if result.returncode != 0:
        errors.append(
            f"gateway_runner.py syntax error: {result.stderr.strip()[:200]}"
        )
        return errors

    # Argument parsing check
    for mode in ("react", "forgetask"):
        result = subprocess.run(
            [
                sys.executable, str(runner_path),
                "--instruction", "test",
                "--service", "openai",
                "--model", "gpt-test",
                "--mode", mode,
                "--help",
            ],
            capture_output=True,
            text=True,
            timeout=10,
        )
        if result.returncode != 0:
            errors.append(
                f"gateway_runner.py --mode {mode} --help: "
                f"{result.stderr.strip()[:200]}"
            )

    return errors


def check_opencode_adapter() -> list[str]:
    """Validate the local-binary OpenCode adapter and runner."""
    errors: list[str] = []
    root = Path(__file__).resolve().parents[1]
    adapter = root / "agents" / "opencode_agent.py"
    runner = root / "agents" / "opencode_runner.py"

    for path in (adapter, runner):
        if not path.is_file():
            errors.append(f"{path.name} not found")
            continue
        result = subprocess.run(
            [
                sys.executable,
                "-c",
                f"compile(open({str(path)!r}).read(), {str(path)!r}, 'exec')",
            ],
            capture_output=True,
            text=True,
            timeout=10,
        )
        if result.returncode != 0:
            errors.append(
                f"{path.name} syntax error: {result.stderr.strip()[:200]}"
            )

    if runner.is_file():
        result = subprocess.run(
            [sys.executable, str(runner), "--help"],
            capture_output=True,
            text=True,
            timeout=10,
        )
        if result.returncode != 0:
            errors.append(
                f"opencode_runner.py --help: {result.stderr.strip()[:200]}"
            )
    return errors


# ---------------------------------------------------------------------------
# 5. YAML config check
# ---------------------------------------------------------------------------

def check_config_generator() -> list[str]:
    """Validate gen_ai_config_yaml.py."""
    errors: list[str] = []
    gen_path = (
        Path(__file__).resolve().parents[1] / "scripts" / "gen_ai_config_yaml.py"
    )

    if not gen_path.is_file():
        return ["gen_ai_config_yaml.py not found"]

    # Syntax check
    result = subprocess.run(
        [sys.executable, "-c", f"compile(open({str(gen_path)!r}).read(), {str(gen_path)!r}, 'exec')"],
        capture_output=True,
        text=True,
        timeout=10,
    )
    if result.returncode != 0:
        errors.append(
            f"gen_ai_config_yaml.py syntax error: {result.stderr.strip()[:200]}"
        )

    # Help flag check
    result = subprocess.run(
        [sys.executable, str(gen_path), "--help"],
        capture_output=True,
        text=True,
        timeout=10,
    )
    if result.returncode != 0:
        errors.append(
            f"gen_ai_config_yaml.py --help: {result.stderr.strip()[:200]}"
        )

    return errors


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main() -> int:
    parser = argparse.ArgumentParser(
        description="Smoke test Yak Agent benchmark tasks (no Docker needed)"
    )
    parser.add_argument(
        "--dataset",
        type=Path,
        default=Path(__file__).resolve().parents[1] / "datasets" / "yak-agent-v1",
        help="Path to the task dataset directory",
    )
    args = parser.parse_args()

    dataset = args.dataset
    if not dataset.is_dir():
        print(f"{_red('ERROR:')} dataset not found: {dataset}", file=sys.stderr)
        return 1

    all_errors: list[str] = []

    # 1. Structural validation
    print(f"{_bold('1. Structural validation')}")
    errors = check_structural(dataset)
    all_errors.extend(errors)
    if errors:
        for e in errors:
            print(f"  {_red('FAIL')} {e}")
    else:
        task_count = len(list(dataset.glob("*/task.toml")))
        print(f"  {_green('OK')} {task_count} tasks pass structural checks")

    # 2. Oracle syntax check
    print(f"\n{_bold('2. Oracle syntax check')}")
    for task_dir in sorted(p.parent for p in dataset.glob("*/task.toml")):
        errors = check_oracle_syntax(task_dir)
        all_errors.extend(errors)
        if errors:
            for e in errors:
                print(f"  {_red('FAIL')} {e}")
        else:
            print(f"  {_green('OK')} {task_dir.name}")

    # 3. Verifier logic check
    print(f"\n{_bold('3. Verifier logic check')}")
    for task_dir in sorted(p.parent for p in dataset.glob("*/task.toml")):
        errors = check_verifier_logic(task_dir)
        all_errors.extend(errors)
        if errors:
            for e in errors:
                print(f"  {_red('FAIL')} {e}")
        else:
            print(f"  {_green('OK')} {task_dir.name}")

    # 4. Gateway runner check
    print(f"\n{_bold('4. Gateway runner check')}")
    errors = check_gateway_runner()
    all_errors.extend(errors)
    if errors:
        for e in errors:
            print(f"  {_red('FAIL')} {e}")
    else:
        print(f"  {_green('OK')} gateway_runner.py")

    # 5. OpenCode adapter check
    print(f"\n{_bold('5. OpenCode adapter check')}")
    errors = check_opencode_adapter()
    all_errors.extend(errors)
    if errors:
        for e in errors:
            print(f"  {_red('FAIL')} {e}")
    else:
        print(f"  {_green('OK')} opencode_agent.py + opencode_runner.py")

    # 6. Config generator check
    print(f"\n{_bold('6. Config generator check')}")
    errors = check_config_generator()
    all_errors.extend(errors)
    if errors:
        for e in errors:
            print(f"  {_red('FAIL')} {e}")
    else:
        print(f"  {_green('OK')} gen_ai_config_yaml.py")

    # Summary
    print()
    if all_errors:
        print(
            f"{_red(f'{len(all_errors)} smoke test failure(s)')}", file=sys.stderr
        )
        return 1
    print(f"{_green('All smoke tests passed')}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
