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


def get_compare_commits(repo: str, old_sha: str, new_sha: str, token: str | None) -> list[dict]:
    """
    Get the ordered list of commits from old_sha (exclusive) to new_sha (inclusive)
    using the GitHub compare API. Returns list of {sha, message}.
    Falls back to [new_sha] if the API fails (single-range promote).
    """
    url = f"{GITHUB_API}/repos/{repo}/compare/{old_sha}...{new_sha}"
    try:
        r = requests.get(url, headers=github_headers(token), timeout=30)
        if r.status_code != 200:
            log(f"compare API returned {r.status_code}, using single range", "WARN")
            return [{"sha": new_sha, "message": ""}]
        data = r.json()
        commits = data.get("commits", [])
        if not commits:
            log("compare returned 0 commits; using single range", "WARN")
            return [{"sha": new_sha, "message": ""}]
        return [{"sha": c["sha"], "message": c.get("commit", {}).get("message", "")} for c in commits]
    except Exception as e:
        log(f"compare API exception: {e}, using single range", "WARN")
        return [{"sha": new_sha, "message": ""}]


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
    Create a plain clone of the worktree's repo for running promote.
    We use a full clone (no --depth) and fetch origin/main so that:
      1. In method-B (GitHub compare+blobs API) the clone is just an isolated
         CWD — full history not strictly needed, but harmless.
      2. When the GitHub API is rate-limited and we fall back to letting the
         promote script run `yak gitefs --start X --end Y`, the clone must
         contain both SHAs' full history or gitefs fails with
         "not a valid commit name".
    The clone is local (from the worktree), so it's fast.
    Returns the clone path, or None on failure.
    """
    clone_dir = Path(f"/tmp/ci-promote-clone-{new_sha[:8]}")
    if clone_dir.exists():
        log(f"Reusing existing clone: {clone_dir}")
        return clone_dir

    log(f"Creating plain clone for promote: {clone_dir}")
    try:
        result = subprocess.run(
            ["git", "clone", str(worktree), str(clone_dir)],
            capture_output=True, text=True, timeout=300, check=True,
        )
        # Fetch full main history so yak gitefs fallback can resolve any SHA.
        subprocess.run(
            ["git", "fetch", "origin", "main"],
            cwd=str(clone_dir), capture_output=True, text=True,
            timeout=120, check=False,
        )
        return clone_dir
    except Exception as e:
        log(f"clone failed: {e}", "ERROR")
        if clone_dir.exists():
            subprocess.run(["rm", "-rf", str(clone_dir)], check=False)
        return None


def build_fs_zip_from_compare(
    repo: str, old_sha: str, new_sha: str, token: str | None, out_path: Path
) -> int:
    """
    Build fs.zip for the range old_sha..new_sha using the GitHub compare API
    and the git blobs API. This replaces `yak gitefs --start X --end Y` when the
    local clone does not have the full main history (worktree on a feature
    branch, shallow clone, etc.).
    Returns the number of file entries written to the zip.

    Flow:
      1. GET /repos/{repo}/compare/{old}...{new} -> files[] with {filename, sha, status}
      2. For added/modified: GET /repos/{repo}/git/blobs/{sha} (base64 content)
      3. Write a zip with only the added/modified files. Removed files are
         skipped (the SSA diff engine treats missing files in newFS as deleted
         relative to the base program).
    """
    import base64
    import zipfile

    url = f"{GITHUB_API}/repos/{repo}/compare/{old_sha}...{new_sha}"
    r = requests.get(url, headers=github_headers(token), timeout=30)
    if r.status_code != 200:
        log(f"compare API returned {r.status_code}: {r.text[:200]}", "ERROR")
        return -1
    data = r.json()
    files = data.get("files", [])
    log(f"compare {old_sha[:8]}...{new_sha[:8]}: status={data.get('status')} "
        f"ahead={data.get('ahead_by')} files={len(files)}")

    written = 0
    skipped = 0
    with zipfile.ZipFile(out_path, "w", zipfile.ZIP_DEFLATED) as zf:
        for f in files:
            name = f.get("filename", "")
            status = f.get("status", "")
            blob_sha = f.get("sha", "")
            if not name or not blob_sha:
                skipped += 1
                continue
            if status in ("removed",):
                # Removed files: skip. The diff engine sees them as deleted in
                # newFS relative to baseFS, which is handled by calculateFileSystemDiff.
                skipped += 1
                continue
            if status not in ("added", "modified", "renamed", "changed"):
                log(f"  skip {name}: unknown status={status}", "WARN")
                skipped += 1
                continue
            # Fetch blob content
            blob_url = f"{GITHUB_API}/repos/{repo}/git/blobs/{blob_sha}"
            br = requests.get(blob_url, headers=github_headers(token), timeout=30)
            if br.status_code != 200:
                log(f"  blob {name} ({blob_sha[:8]}) returned {br.status_code}, skip", "WARN")
                skipped += 1
                continue
            blob = br.json()
            content_b64 = blob.get("content", "")
            try:
                content = base64.b64decode(content_b64.replace("\n", ""))
            except Exception as e:
                log(f"  decode {name} failed: {e}, skip", "WARN")
                skipped += 1
                continue
            zf.writestr(name, content)
            written += 1

    log(f"fs.zip built: {written} files written, {skipped} skipped -> {out_path}")
    return written


def run_promote_once(
    clone_dir: Path,
    script: Path,
    new_sha: str,
    pr_number: str,
    fs_zip_path: Path | None,
    base_program: str,
    clone_data: Path,
    worktree: Path,
) -> bool:
    """
    Run promote-base-on-merge.sh once for a single range. If fs_zip_path is
    given, it is copied into the clone CWD and FS_ZIP_PREBUILT=1 is set so the
    promote script skips `yak gitefs`. Returns True on success.
    """
    env = os.environ.copy()
    env["SSA_CI_DATA_DIR"] = str(clone_data)
    env["SSA_DATABASE_RAW"] = str(clone_data / "default-yakssa.db")
    env["CI_SSA_BASE_PROGRAM"] = base_program
    env["PATH"] = env.get("PATH", "") + ":/usr/local/go/bin:" + os.path.expanduser("~/.local/bin") + ":" + os.path.expanduser("~/go/bin")
    # Disable the promote script's own catch-up loop: the monitor drives the
    # loop per-range (because in method-B we must rebuild fs.zip per range).
    env["CI_SSA_PROMOTE_CATCH_UP"] = "0"
    if fs_zip_path is not None:
        env["FS_ZIP_PREBUILT"] = "1"

    log(f"Running promote: {new_sha[:8]} (PR={pr_number or 'none'}, "
        f"fs_zip={'prebuilt' if fs_zip_path else 'yak gitefs'})")
    cmd = ["bash", str(script), new_sha, pr_number]
    try:
        result = subprocess.run(
            cmd,
            cwd=str(clone_dir),
            env=env,
            capture_output=True,
            text=True,
            timeout=600,
        )
        if result.stdout:
            for line in result.stdout.splitlines():
                print(f"  [promote] {line}", flush=True)
        if result.stderr:
            for line in result.stderr.splitlines()[-10:]:
                print(f"  [promote:err] {line}", flush=True)

        if result.returncode != 0:
            log(f"promote failed (exit {result.returncode})", "ERROR")
            return False
        return True
    except subprocess.TimeoutExpired:
        log("promote timed out after 600s", "ERROR")
        return False
    except Exception as e:
        log(f"promote exception: {e}", "ERROR")
        return False


def run_promote(
    repo: str,
    worktree: Path,
    data_dir: Path,
    old_sha: str,
    new_sha: str,
    pr_number: str,
    token: str | None,
) -> bool:
    """
    Run promote for the range old_sha..new_sha. Uses method-B (GitHub compare
    + blobs API) to build fs.zip per intermediate range, so no local main
    history is needed. If the range spans multiple commits (multiple PRs merged
    between runs), walk each commit individually so each PR's diff lands in
    its own overlay layer (matches CI behavior).
    Returns True if the final manifest sha == new_sha.
    """
    script = worktree / "scripts" / "ci-ssa" / "promote-base-on-merge.sh"
    if not script.exists():
        log(f"promote script not found: {script}", "ERROR")
        return False

    # 1. Create a plain clone (isolated CWD with symlinks)
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

    # 3. Resolve the intermediate commit chain from old_sha to new_sha.
    commits = get_compare_commits(repo, old_sha, new_sha, token)
    ranges = []
    cur = old_sha
    for c in commits:
        nxt = c["sha"]
        if nxt == cur:
            continue
        ranges.append((cur, nxt, c.get("message", "").splitlines()[0][:60] if c.get("message") else ""))
        cur = nxt
    if cur != new_sha:
        # compare API was partial; append a final catch-all range
        ranges.append((cur, new_sha, "(final catch-all)"))

    log(f"Promote plan: {len(ranges)} range(s) from {old_sha[:8]} to {new_sha[:8]}")
    for i, (a, b, msg) in enumerate(ranges, 1):
        log(f"  [{i}/{len(ranges)}] {a[:8]}..{b[:8]}  {msg}")

    # 4. Walk each range: build fs.zip via compare+blobs API, run promote once.
    for i, (a, b, msg) in enumerate(ranges, 1):
        log(f"=== Range [{i}/{len(ranges)}]: {a[:8]}..{b[:8]} ({msg}) ===")
        fs_zip_path = clone_dir / "fs.zip"
        # Re-read base program from pointer each iteration (promote updated it)
        base_program = (data_dir / "base-program-name").read_text().strip() if (data_dir / "base-program-name").exists() else "ci-yaklang-base"
        count = build_fs_zip_from_compare(repo, a, b, token, fs_zip_path)
        if count < 0:
            # Method-B failed (usually GitHub API rate limit). Fall back to
            # letting the promote script build fs.zip itself via `yak gitefs`.
            # This needs the clone to have both SHAs in its history (full
            # clone + fetch origin main, see prepare_clone).
            log(f"build_fs_zip failed ({count}); falling back to yak gitefs for {a[:8]}..{b[:8]}", "WARN")
            ok = run_promote_once(
                clone_dir, script, b, pr_number,
                None,  # fs_zip_path=None -> promote uses yak gitefs
                base_program, clone_data, worktree,
            )
        else:
            ok = run_promote_once(
                clone_dir, script, b, pr_number,
                fs_zip_path,
                base_program, clone_data, worktree,
            )
        if not ok:
            log(f"promote failed at range {a[:8]}..{b[:8]}", "ERROR")
            return False
        log(f"Range [{i}/{len(ranges)}] complete")

    log("promote completed successfully")
    return True


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
    success = run_promote(repo, worktree, data_dir, manifest_sha, main_head, pr_number, token)

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