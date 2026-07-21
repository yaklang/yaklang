#!/usr/bin/env python3
"""
ci-promote-monitor.py — Monitor yaklang/yaklang main branch for merged PRs
and run the SSA promote flow locally (simulating GitHub Actions self-hosted runner).

No admin permissions needed: uses the public GitHub REST API (unauthenticated
or with GITHUB_TOKEN for higher rate limits) to poll main HEAD, compares it
against the local manifest.main_sha, and runs promote-base-on-merge.sh when
a new merge is detected.

Usage:
  python3 ci-promote-monitor.py [--once] [--interval 300] [--repo yaklang/yaklang]

  --once        Run a single check and exit (for cron-style scheduling)
  --interval N  Poll interval in seconds (default 300)
  --repo        GitHub repo in owner/name format (default yaklang/yaklang)

Environment:
  GITHUB_TOKEN  Optional. Raises API rate limit from 60 to 5000 req/hour.
  CI_SSA_DATA_DIR  Path to SSA data dir (default: ./ci-ssa-data)
  YAKLANG_WORKTREE Path to yaklang worktree with yak binary + scripts/ci-ssa/
                   (default: ~/yaklang_workspace/yhellow-ssa-incremental)

Flow:
  1. Fetch latest main HEAD SHA from GitHub API.
  2. Read local manifest.json -> main_sha (last promoted SHA).
  3. If main HEAD != manifest main_sha:
     a. Fetch merged PRs between old SHA and new SHA.
     b. For each merged PR: run promote-base-on-merge.sh <new_sha> <pr_number>.
        (promote script handles incremental compile + manifest update + cleanup)
  4. If main HEAD == manifest main_sha: nothing to do.
"""

import argparse
import json
import os
import subprocess
import sys
import time
from pathlib import Path
from datetime import datetime, timezone

import requests
import urllib3

urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)


GITHUB_API = "https://api.github.com"
DEFAULT_REPO = "yaklang/yaklang"
DEFAULT_INTERVAL = 300  # 5 minutes
DEFAULT_WORKTREE = os.path.expanduser("~/yaklang_workspace/yhellow-ssa-incremental")
DEFAULT_DATA_DIR = "./ci-ssa-data"


def log(msg: str, level: str = "INFO") -> None:
    ts = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    print(f"[{ts}] [{level}] {msg}", flush=True)


def github_headers(token: str | None) -> dict:
    h = {"Accept": "application/vnd.github+json.v3"}
    if token:
        h["Authorization"] = f"Bearer {token}"
    return h


def get_main_head(repo: str, token: str | None) -> str:
    """Fetch the latest commit SHA of the main branch."""
    url = f"{GITHUB_API}/repos/{repo}/branches/main"
    r = requests.get(url, headers=github_headers(token), timeout=30)
    r.raise_for_status()
    return r.json()["commit"]["sha"]


def get_main_head_from_worktree(worktree: Path) -> str:
    """Fallback: get main HEAD from local git fetch."""
    try:
        subprocess.run(
            ["git", "fetch", "origin", "main"],
            cwd=str(worktree),
            capture_output=True,
            timeout=60,
            check=False,
        )
        out = subprocess.run(
            ["git", "rev-parse", "origin/main"],
            cwd=str(worktree),
            capture_output=True,
            text=True,
            timeout=10,
            check=True,
        )
        return out.stdout.strip()
    except Exception as e:
        log(f"git fetch failed: {e}", "ERROR")
        return ""


def get_merged_prs_in_range(repo: str, old_sha: str, new_sha: str, token: str | None) -> list[dict]:
    """
    Fetch PRs merged between old_sha and new_sha.
    Uses the commits comparison API + searches for PRs.
    Returns list of {number, title, merge_commit_sha, html_url}.
    """
    # Get commits between old and new
    url = f"{GITHUB_API}/repos/{repo}/compare/{old_sha}...{new_sha}"
    r = requests.get(url, headers=github_headers(token), timeout=30)
    if r.status_code != 200:
        log(f"compare API returned {r.status_code}, falling back to empty PR list", "WARN")
        return []
    data = r.json()
    commits = data.get("commits", [])

    # Search for PRs whose merge_commit_sha matches any commit in the range
    merged_prs = []
    commit_shas = {c["sha"] for c in commits}
    # Also include the new_sha itself (it's the merge commit)
    commit_shas.add(new_sha)

    # Use search API to find recently merged PRs
    search_url = f"{GITHUB_API}/search/issues"
    params = {
        "q": f"repo:{repo} is:pr is:merged base:main sort:updated",
        "per_page": 20,
    }
    r = requests.get(search_url, headers=github_headers(token), params=params, timeout=30)
    if r.status_code != 200:
        log(f"search API returned {r.status_code}", "WARN")
        return []

    for item in r.json().get("items", []):
        merge_sha = item.get("pull_request", {}).get("merge_commit_sha", "")
        if merge_sha in commit_shas:
            merged_prs.append({
                "number": item["number"],
                "title": item["title"],
                "merge_commit_sha": merge_sha,
                "html_url": item["html_url"],
            })

    return merged_prs


def read_manifest(data_dir: Path) -> dict | None:
    """Read the local manifest.json."""
    manifest_path = data_dir / "manifest.json"
    if not manifest_path.exists():
        return None
    try:
        return json.loads(manifest_path.read_text())
    except Exception as e:
        log(f"Failed to read manifest: {e}", "ERROR")
        return None


def prepare_clone(worktree: Path, new_sha: str) -> Path | None:
    """
    Create a shallow plain clone of the worktree's repo for running promote.
    yak gitefs uses go-git which can't resolve refs in a worktree (gitdir:
    pointer to .bare); a plain clone fixes this. The clone shares the same
    object store as the worktree so it's fast.
    Returns the clone path, or None on failure.
    """
    clone_dir = Path(f"/tmp/ci-promote-clone-{new_sha[:8]}")
    if clone_dir.exists():
        log(f"Reusing existing clone: {clone_dir}")
        return clone_dir

    log(f"Creating plain clone for promote: {clone_dir}")
    try:
        # Clone from the worktree (local, fast)
        result = subprocess.run(
            ["git", "clone", "--depth", "50", str(worktree), str(clone_dir)],
            capture_output=True, text=True, timeout=120, check=True,
        )
        # Fetch the new SHA into the clone (in case it's not in depth-50)
        subprocess.run(
            ["git", "fetch", "origin", "main"],
            cwd=str(clone_dir), capture_output=True, text=True,
            timeout=60, check=False,
        )
        # Create local main branch tracking origin/main
        subprocess.run(
            ["git", "branch", "-f", "main", "origin/main"],
            cwd=str(clone_dir), capture_output=True, text=True,
            timeout=10, check=False,
        )
        return clone_dir
    except Exception as e:
        log(f"clone failed: {e}", "ERROR")
        if clone_dir.exists():
            subprocess.run(["rm", "-rf", str(clone_dir)], check=False)
        return None


def run_promote(worktree: Path, data_dir: Path, new_sha: str, pr_number: str) -> bool:
    """
    Run promote-base-on-merge.sh to simulate the CI promote flow.
    Creates a plain clone first (yak gitefs needs a real .git dir, not a
    worktree gitdir pointer), then runs the promote script there with
    symlinks to the yak binary and ci-ssa-data.
    Returns True on success.
    """
    script = worktree / "scripts" / "ci-ssa" / "promote-base-on-merge.sh"
    if not script.exists():
        log(f"promote script not found: {script}", "ERROR")
        return False

    # 1. Create a plain clone for gitefs to work
    clone_dir = prepare_clone(worktree, new_sha)
    if clone_dir is None:
        log("Failed to prepare clone, aborting promote", "ERROR")
        return False

    # 2. Symlink yak binary and ci-ssa scripts + data into the clone
    yak_bin = worktree / "yak"
    if not yak_bin.exists():
        log(f"yak binary not found: {yak_bin}", "ERROR")
        return False
    clone_yak = clone_dir / "yak"
    if not clone_yak.exists():
        clone_yak.symlink_to(yak_bin.resolve())

    clone_scripts = clone_dir / "scripts"
    if not clone_scripts.exists():
        clone_scripts.symlink_to(worktree.resolve() / "scripts")

    clone_data = clone_dir / "ci-ssa-data"
    if not clone_data.exists():
        clone_data.symlink_to(data_dir.resolve())

    # 3. Run promote in the clone
    env = os.environ.copy()
    env["SSA_CI_DATA_DIR"] = str(clone_data)
    env["SSA_DATABASE_RAW"] = str(clone_data / "default-yakssa.db")
    env["CI_SSA_BASE_PROGRAM"] = (data_dir / "base-program-name").read_text().strip() if (data_dir / "base-program-name").exists() else "ci-yaklang-base"
    env["PATH"] = env.get("PATH", "") + ":/usr/local/go/bin:" + os.path.expanduser("~/.local/bin") + ":" + os.path.expanduser("~/go/bin")

    log(f"Running promote in clone: {new_sha[:8]} (PR={pr_number or 'none'})")
    cmd = ["bash", str(script), new_sha, pr_number]
    try:
        result = subprocess.run(
            cmd,
            cwd=str(clone_dir),
            env=env,
            capture_output=True,
            text=True,
            timeout=600,  # 10 min max (incremental promote should be fast)
        )
        # Print output
        if result.stdout:
            for line in result.stdout.splitlines():
                print(f"  [promote] {line}", flush=True)
        if result.stderr:
            for line in result.stderr.splitlines()[-10:]:
                print(f"  [promote:err] {line}", flush=True)

        if result.returncode != 0:
            log(f"promote failed (exit {result.returncode})", "ERROR")
            return False
        log("promote completed successfully")
        return True
    except subprocess.TimeoutExpired:
        log("promote timed out after 600s", "ERROR")
        return False
    except Exception as e:
        log(f"promote exception: {e}", "ERROR")
        return False


def check_and_promote(repo: str, worktree: Path, data_dir: Path, token: str | None) -> bool:
    """
    Single check cycle: compare main HEAD vs manifest, promote if needed.
    Returns True if a promote was executed (regardless of success).
    """
    # 1. Get current main HEAD
    main_head = ""
    try:
        main_head = get_main_head(repo, token)
        log(f"GitHub main HEAD: {main_head[:12]}")
    except Exception as e:
        log(f"GitHub API failed ({e}), falling back to git fetch", "WARN")
        main_head = get_main_head_from_worktree(worktree)
        if not main_head:
            log("Could not determine main HEAD", "ERROR")
            return False
        log(f"Local origin/main HEAD: {main_head[:12]}")

    # 2. Read manifest
    manifest = read_manifest(data_dir)
    if manifest is None:
        log("No manifest found, run weekly full compile first", "ERROR")
        return False

    manifest_sha = manifest.get("main_sha", "")
    manifest_base = manifest.get("base_program_name", "ci-yaklang-base")
    manifest_depth = manifest.get("overlay_depth", 0)
    log(f"Manifest: sha={manifest_sha[:12]} base={manifest_base} depth={manifest_depth}")

    # 3. Compare
    if main_head == manifest_sha:
        log("main HEAD == manifest sha, nothing to promote")
        return False

    if not manifest_sha:
        log("manifest main_sha is empty, nothing to compare", "WARN")
        return False

    # 4. Fetch merged PRs in range
    merged_prs = get_merged_prs_in_range(repo, manifest_sha, main_head, token)
    if merged_prs:
        pr_list = ", ".join(f"#{p['number']} ({p['title'][:40]})" for p in merged_prs)
        log(f"Found {len(merged_prs)} merged PR(s): {pr_list}")
    else:
        log(f"No PRs found in range {manifest_sha[:8]}...{main_head[:8]} (may be direct push or search miss)")

    # 5. Run promote for the new HEAD
    # Use the last PR number if available (cleanup targets that PR's diff programs)
    pr_number = str(merged_prs[-1]["number"]) if merged_prs else ""
    success = run_promote(worktree, data_dir, main_head, pr_number)

    # 6. Verify: read manifest again
    new_manifest = read_manifest(data_dir)
    if new_manifest:
        new_sha = new_manifest.get("main_sha", "")
        new_base = new_manifest.get("base_program_name", "")
        new_depth = new_manifest.get("overlay_depth", 0)
        log(f"Post-promote manifest: sha={new_sha[:12]} base={new_base} depth={new_depth}")
        if new_sha == main_head:
            log("✅ Promote verified: manifest sha matches main HEAD")
        else:
            log(f"⚠️ Manifest sha {new_sha[:12]} != main HEAD {main_head[:12]}", "WARN")

    return True


def main():
    parser = argparse.ArgumentParser(description="Monitor yaklang main and run SSA promote")
    parser.add_argument("--once", action="store_true", help="Run a single check and exit")
    parser.add_argument("--interval", type=int, default=DEFAULT_INTERVAL, help=f"Poll interval seconds (default {DEFAULT_INTERVAL})")
    parser.add_argument("--repo", type=str, default=DEFAULT_REPO, help=f"GitHub repo (default {DEFAULT_REPO})")
    parser.add_argument("--worktree", type=str, default=DEFAULT_WORKTREE, help="Path to yaklang worktree")
    parser.add_argument("--data-dir", type=str, default=DEFAULT_DATA_DIR, help="Path to ci-ssa-data dir")
    args = parser.parse_args()

    worktree = Path(os.path.expanduser(args.worktree))
    data_dir = Path(os.path.expanduser(args.data_dir))
    token = os.environ.get("GITHUB_TOKEN", "")

    log(f"CI Promote Monitor started")
    log(f"  repo:      {args.repo}")
    log(f"  worktree:  {worktree}")
    log(f"  data_dir:  {data_dir}")
    log(f"  interval:  {args.interval}s")
    log(f"  mode:      {'once' if args.once else 'poll'}")
    log(f"  token:     {'yes' if token else 'no (unauthenticated, 60 req/hr)'}")

    if not worktree.exists():
        log(f"worktree not found: {worktree}", "ERROR")
        sys.exit(1)
    if not data_dir.exists():
        log(f"data_dir not found: {data_dir}", "ERROR")
        sys.exit(1)

    while True:
        try:
            check_and_promote(args.repo, worktree, data_dir, token)
        except KeyboardInterrupt:
            log("Interrupted by user, exiting")
            break
        except Exception as e:
            log(f"check cycle error: {e}", "ERROR")

        if args.once:
            break
        log(f"Next check in {args.interval}s...")
        try:
            time.sleep(args.interval)
        except KeyboardInterrupt:
            log("Interrupted by user, exiting")
            break


if __name__ == "__main__":
    main()