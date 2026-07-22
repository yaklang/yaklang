#!/usr/bin/env python3
"""
ci-promote-monitor.py — Monitor yaklang/yaklang for PR lifecycle events and
run SSA incremental compile / promote flows locally.

Event-driven model (only processes PRs that change *during* monitoring):
  - open  : new PR opened during monitoring  → run incremental diff scan (CI)
  - merge : PR merged into main during monitoring → run promote (update base)
  - close : PR closed (non-merge) during monitoring → record only, no action

Usage:
  python3 ci-promote-monitor.py [--once] [--interval 300] [--repo yaklang/yaklang]

Environment:
  GITHUB_TOKEN  Optional. Raises API rate limit from 60 to 5000 req/hour.
"""

import argparse
import base64
import json
import os
import subprocess
import sys
import time
import zipfile
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
MAX_EVENTS = 200
API_MAX_RETRIES = 10
API_BASE_WAIT = 120  # seconds, multiplied by attempt number


# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------

def log(msg: str, level: str = "INFO") -> None:
    ts = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    print(f"[{ts}] [{level}] {msg}", flush=True)


def log_short(msg: str) -> None:
    """Compact one-liner for routine idle polls (no level tag)."""
    ts = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    print(f"[{ts}] {msg}", flush=True)


# ---------------------------------------------------------------------------
# Progress bar (tty-aware)
# ---------------------------------------------------------------------------

def show_progress(current: int, total: int, prefix: str = "") -> None:
    """Show a progress bar. Uses \\r on tty; falls back to periodic log on pipes."""
    if total <= 0:
        return
    pct = int(100 * current / total)
    if sys.stdout.isatty():
        bar_len = 20
        filled = int(bar_len * current / total)
        bar = "\u2588" * filled + "\u2591" * (bar_len - filled)
        sys.stdout.write(f"\r{prefix} [{bar}] {current}/{total} ({pct}%)")
        sys.stdout.flush()
        if current >= total:
            sys.stdout.write("\n")
            sys.stdout.flush()
    else:
        # Non-tty (tee/pipe): print every 10% or at completion
        step = max(1, total // 10)
        if current % step == 0 or current >= total:
            log(f"{prefix} {current}/{total} ({pct}%)")


# ---------------------------------------------------------------------------
# GitHub API helpers with long-wait retry
# ---------------------------------------------------------------------------

def github_headers(token: str | None) -> dict:
    h = {"Accept": "application/vnd.github+json.v3"}
    if token:
        h["Authorization"] = f"Bearer {token}"
    return h


def api_request_with_retry(
    url: str,
    token: str | None,
    params: dict | None = None,
    max_retries: int = API_MAX_RETRIES,
    base_wait: int = API_BASE_WAIT,
) -> requests.Response | None:
    """
    GET request with long-wait retry strategy.
    Returns the Response on success (HTTP 200), or None after exhausting retries.
    Wait = base_wait * (attempt + 1), so 120s, 240s, 360s, ... up to 1200s.
    """
    for attempt in range(max_retries):
        try:
            r = requests.get(url, headers=github_headers(token), params=params, timeout=30)
            if r.status_code == 200:
                return r
            if r.status_code == 403:
                # Rate limit or secondary rate limit
                wait = base_wait * (attempt + 1)
                log(f"API 403 (rate limit), waiting {wait}s (attempt {attempt + 1}/{max_retries})", "WARN")
                time.sleep(wait)
                continue
            # Other HTTP errors
            wait = base_wait * (attempt + 1)
            log(f"API HTTP {r.status_code}, retry in {wait}s (attempt {attempt + 1}/{max_retries})", "WARN")
            time.sleep(wait)
        except Exception as e:
            wait = base_wait * (attempt + 1)
            log(f"API error: {e}, retry in {wait}s (attempt {attempt + 1}/{max_retries})", "WARN")
            time.sleep(wait)
    log(f"API failed after {max_retries} retries: {url}", "ERROR")
    return None


def get_main_head(repo: str, token: str | None) -> str:
    """Fetch the latest commit SHA of the main branch via GitHub API."""
    url = f"{GITHUB_API}/repos/{repo}/branches/main"
    r = api_request_with_retry(url, token)
    if r is None:
        raise Exception("branches/main API failed")
    return r.json()["commit"]["sha"]


def get_main_head_from_worktree(worktree: Path) -> str:
    """Fallback for DNS failure: get main HEAD from local git fetch."""
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
    except Exception:
        return ""


def get_compare_commits(repo: str, old_sha: str, new_sha: str, token: str | None) -> list[dict]:
    """Get ordered commits from old_sha (exclusive) to new_sha (inclusive)."""
    url = f"{GITHUB_API}/repos/{repo}/compare/{old_sha}...{new_sha}"
    r = api_request_with_retry(url, token)
    if r is None:
        log("compare API failed, using single range", "WARN")
        return [{"sha": new_sha, "message": ""}]
    data = r.json()
    commits = data.get("commits", [])
    if not commits:
        log("compare returned 0 commits, single range", "WARN")
        return [{"sha": new_sha, "message": ""}]
    return [{"sha": c["sha"], "message": c.get("commit", {}).get("message", "")} for c in commits]


def get_merged_prs_in_range(repo: str, old_sha: str, new_sha: str, token: str | None) -> list[dict]:
    """Fetch PRs merged between old_sha and new_sha."""
    url = f"{GITHUB_API}/repos/{repo}/compare/{old_sha}...{new_sha}"
    r = api_request_with_retry(url, token)
    if r is None:
        log("compare API failed, no PR list", "WARN")
        return []
    data = r.json()
    commits = data.get("commits", [])
    commit_shas = {c["sha"] for c in commits}
    commit_shas.add(new_sha)

    search_url = f"{GITHUB_API}/search/issues"
    params = {
        "q": f"repo:{repo} is:pr is:merged base:main sort:updated",
        "per_page": 20,
    }
    r = api_request_with_retry(search_url, token, params=params)
    if r is None:
        log("search API failed, no merged PR list", "WARN")
        return []

    merged_prs = []
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


def get_open_prs(repo: str, token: str | None) -> list[dict]:
    """Fetch currently open PRs targeting main via pulls API (includes head SHA)."""
    pulls_url = f"{GITHUB_API}/repos/{repo}/pulls"
    params = {
        "state": "open",
        "base": "main",
        "sort": "updated",
        "direction": "desc",
        "per_page": 30,
    }
    r = api_request_with_retry(pulls_url, token, params=params)
    if r is None:
        return []
    items = r.json()
    return [{
        "number": item["number"],
        "title": item["title"],
        "head_sha": item.get("head", {}).get("sha", ""),
        "html_url": item["html_url"],
    } for item in items]


def get_recently_closed_prs(repo: str, token: str | None) -> list[dict]:
    """Fetch recently closed PRs targeting main (includes merged ones)."""
    search_url = f"{GITHUB_API}/search/issues"
    params = {
        "q": f"repo:{repo} is:pr is:closed base:main sort:updated",
        "per_page": 10,
    }
    r = api_request_with_retry(search_url, token, params=params)
    if r is None:
        return []
    items = r.json().get("items", [])
    result = []
    for item in items:
        merged = bool(item.get("pull_request", {}).get("merged_at"))
        result.append({
            "number": item["number"],
            "title": item["title"],
            "merged": merged,
            "merge_commit_sha": item.get("pull_request", {}).get("merge_commit_sha", ""),
            "html_url": item["html_url"],
        })
    return result


# ---------------------------------------------------------------------------
# Manifest
# ---------------------------------------------------------------------------

def read_manifest(data_dir: Path) -> dict | None:
    manifest_path = data_dir / "manifest.json"
    if not manifest_path.exists():
        return None
    try:
        return json.loads(manifest_path.read_text())
    except Exception as e:
        log(f"Failed to read manifest: {e}", "ERROR")
        return None


# ---------------------------------------------------------------------------
# Event log — append PR lifecycle events to events.json (max 200)
# ---------------------------------------------------------------------------

def append_event(data_dir: Path, event: dict) -> None:
    events_path = data_dir / "events.json"
    events = []
    if events_path.exists():
        try:
            events = json.loads(events_path.read_text())
            if not isinstance(events, list):
                events = []
        except Exception:
            events = []
    event["timestamp"] = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    events.append(event)
    if len(events) > MAX_EVENTS:
        events = events[-MAX_EVENTS:]
    try:
        events_path.write_text(json.dumps(events, indent=2, ensure_ascii=False) + "\n")
    except Exception as e:
        log(f"Failed to write events.json: {e}", "ERROR")


# ---------------------------------------------------------------------------
# CI log isolation — ci-ssa-data/ci-logs/{pr_number}/
# ---------------------------------------------------------------------------

def ci_log_dir(data_dir: Path, pr_number: int) -> Path:
    d = data_dir / "ci-logs" / str(pr_number)
    d.mkdir(parents=True, exist_ok=True)
    return d


def timestamp_str() -> str:
    return datetime.now(timezone.utc).strftime("%Y%m%d_%H%M%S")


# ---------------------------------------------------------------------------
# Clone preparation
# ---------------------------------------------------------------------------

def prepare_clone(worktree: Path, new_sha: str) -> Path | None:
    """Create a plain clone of the worktree for running promote."""
    clone_dir = Path(f"/tmp/ci-promote-clone-{new_sha[:8]}")
    if clone_dir.exists():
        return clone_dir
    try:
        subprocess.run(
            ["git", "clone", str(worktree), str(clone_dir)],
            capture_output=True, text=True, timeout=300, check=True,
        )
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


# ---------------------------------------------------------------------------
# Build fs.zip via GitHub compare + blobs API (with progress + retry)
# ---------------------------------------------------------------------------

def build_fs_zip_from_compare(
    repo: str, old_sha: str, new_sha: str, token: str | None, out_path: Path
) -> int:
    """
    Build fs.zip for old_sha..new_sha using GitHub compare + blobs API.
    Returns number of files written, or -1 on failure.
    """
    url = f"{GITHUB_API}/repos/{repo}/compare/{old_sha}...{new_sha}"
    r = api_request_with_retry(url, token)
    if r is None:
        log(f"compare API failed for {old_sha[:8]}...{new_sha[:8]}", "ERROR")
        return -1
    data = r.json()
    files = data.get("files", [])
    log(f"compare {old_sha[:8]}...{new_sha[:8]}: ahead={data.get('ahead_by')} files={len(files)}")

    written = 0
    skipped = 0
    with zipfile.ZipFile(out_path, "w", zipfile.ZIP_DEFLATED) as zf:
        for idx, f in enumerate(files, 1):
            name = f.get("filename", "")
            status = f.get("status", "")
            blob_sha = f.get("sha", "")
            if not name or not blob_sha:
                skipped += 1
                continue
            if status in ("removed",):
                skipped += 1
                continue
            if status not in ("added", "modified", "renamed", "changed"):
                skipped += 1
                continue
            blob_url = f"{GITHUB_API}/repos/{repo}/git/blobs/{blob_sha}"
            br = api_request_with_retry(blob_url, token)
            if br is None:
                skipped += 1
                continue
            blob = br.json()
            content_b64 = blob.get("content", "")
            try:
                content = base64.b64decode(content_b64.replace("\n", ""))
            except Exception:
                skipped += 1
                continue
            zf.writestr(name, content)
            written += 1
            show_progress(idx, len(files), "Downloading files")

    if sys.stdout.isatty() and len(files) > 0:
        # Ensure newline after progress bar if not already printed
        pass

    log(f"fs.zip built: {written} files written, {skipped} skipped")
    return written


# ---------------------------------------------------------------------------
# Run promote (merge event → incremental compile to base)
# ---------------------------------------------------------------------------

def run_promote_once(
    clone_dir: Path,
    script: Path,
    new_sha: str,
    pr_number: str,
    base_program: str,
    clone_data: Path,
    worktree: Path,
    log_file: Path | None,
) -> bool:
    """Run promote-base-on-merge.sh once. Output goes to log_file if given."""
    env = os.environ.copy()
    env["SSA_CI_DATA_DIR"] = str(clone_data)
    env["SSA_DATABASE_RAW"] = str(clone_data / "default-yakssa.db")
    env["CI_SSA_BASE_PROGRAM"] = base_program
    env["PATH"] = env.get("PATH", "") + ":/usr/local/go/bin:" + os.path.expanduser("~/.local/bin") + ":" + os.path.expanduser("~/go/bin")
    env["CI_SSA_PROMOTE_CATCH_UP"] = "0"
    env["FS_ZIP_PREBUILT"] = "1"

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
        # Write output to isolated log file
        if log_file:
            log_file.parent.mkdir(parents=True, exist_ok=True)
            with open(log_file, "w") as f:
                f.write(f"=== promote {new_sha[:8]} (PR={pr_number or 'none'}) ===\n")
                f.write(f"=== exit code: {result.returncode} ===\n\n")
                f.write("--- stdout ---\n")
                f.write(result.stdout or "")
                f.write("\n--- stderr ---\n")
                f.write(result.stderr or "")
        if result.returncode != 0:
            if result.stderr:
                for line in result.stderr.splitlines()[-10:]:
                    print(f"  [promote:err] {line}", flush=True)
            log(f"promote failed (exit {result.returncode}), see {log_file}", "ERROR") if log_file else log(f"promote failed (exit {result.returncode})", "ERROR")
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
    """Run promote for the range old_sha..new_sha via GitHub API (Method B only)."""
    script = worktree / "scripts" / "ci-ssa" / "promote-base-on-merge.sh"
    if not script.exists():
        log(f"promote script not found: {script}", "ERROR")
        return False

    clone_dir = prepare_clone(worktree, new_sha)
    if clone_dir is None:
        log("Failed to prepare clone, aborting promote", "ERROR")
        return False

    # Symlink yak, scripts, data into clone
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

    # Resolve commit chain
    commits = get_compare_commits(repo, old_sha, new_sha, token)
    ranges = []
    cur = old_sha
    for c in commits:
        nxt = c["sha"]
        if nxt == cur:
            continue
        ranges.append((cur, nxt))
        cur = nxt
    if cur != new_sha:
        ranges.append((cur, new_sha))

    log(f"Promote plan: {len(ranges)} range(s) {old_sha[:8]}..{new_sha[:8]}")

    # CI log isolation
    pr_num_int = int(pr_number) if pr_number else 0
    ci_dir = ci_log_dir(data_dir, pr_num_int) if pr_num_int else None
    promote_log = None
    if ci_dir:
        promote_log = ci_dir / f"promote_{timestamp_str()}_{new_sha[:8]}.log"

    total_stages = len(ranges) * 3  # each range: compare+download, compile, verify
    stage = 0

    for i, (a, b) in enumerate(ranges, 1):
        log(f"--- Range [{i}/{len(ranges)}]: {a[:8]}..{b[:8]} ---")

        # Stage: build fs.zip
        stage += 1
        show_progress(stage, total_stages, f"Promote PR#{pr_number or '?'}")
        fs_zip_path = clone_dir / "fs.zip"
        base_program = (data_dir / "base-program-name").read_text().strip() if (data_dir / "base-program-name").exists() else "ci-yaklang-base"
        count = build_fs_zip_from_compare(repo, a, b, token, fs_zip_path)
        if count < 0:
            log(f"build_fs_zip failed for {a[:8]}..{b[:8]}", "ERROR")
            return False

        # Stage: compile
        stage += 1
        show_progress(stage, total_stages, f"Promote PR#{pr_number or '?'}")
        ok = run_promote_once(
            clone_dir, script, b, pr_number,
            base_program, clone_data, worktree, promote_log,
        )
        if not ok:
            log(f"promote failed at range {a[:8]}..{b[:8]}", "ERROR")
            return False

        # Stage: verify (implicit in promote script)
        stage += 1
        show_progress(stage, total_stages, f"Promote PR#{pr_number or '?'}")

    if sys.stdout.isatty():
        print(flush=True)  # newline after progress bar
    log("promote completed successfully")
    return True


# ---------------------------------------------------------------------------
# Run PR scan (open event → incremental diff scan)
# ---------------------------------------------------------------------------

def run_pr_scan(
    repo: str,
    worktree: Path,
    data_dir: Path,
    pr_number: int,
    pr_head_sha: str,
    token: str | None,
) -> bool:
    """
    Run incremental diff scan for a newly opened PR.
    1. Build fs.zip from main..pr_head via GitHub API
    2. Generate scan config via generate-diff-scan-config.sh
    3. Run yak ssa-compile with scan config
    """
    # Get current main HEAD (from manifest, it's the promoted base)
    manifest = read_manifest(data_dir)
    if manifest is None:
        log("No manifest, cannot run PR scan", "ERROR")
        return False
    main_sha = manifest.get("main_sha", "")
    if not main_sha:
        log("manifest main_sha is empty, cannot run PR scan", "ERROR")
        return False

    # CI log dir for this PR
    ci_dir = ci_log_dir(data_dir, pr_number)
    short_sha = pr_head_sha[:8] if pr_head_sha else "unknown"
    scan_log = ci_dir / f"scan_{timestamp_str()}_{short_sha}.log"
    fs_zip_path = ci_dir / "fs.zip"
    scan_config = ci_dir / "scan-config.json"
    scan_result = ci_dir / "scan-result"
    scan_result.mkdir(parents=True, exist_ok=True)

    # Stage 1: build fs.zip
    log(f"PR #{pr_number} scan: building fs.zip {main_sha[:8]}...{short_sha}")
    count = build_fs_zip_from_compare(repo, main_sha, pr_head_sha, token, fs_zip_path)
    if count < 0:
        log(f"PR #{pr_number} scan: build_fs_zip failed", "ERROR")
        return False

    # Stage 2: generate scan config
    gen_script = worktree / "scripts" / "ci-ssa" / "generate-diff-scan-config.sh"
    if not gen_script.exists():
        log(f"generate-diff-scan-config.sh not found: {gen_script}", "ERROR")
        return False

    env = os.environ.copy()
    env["SSA_CI_DATA_DIR"] = str(data_dir)
    env["SSA_DATABASE_RAW"] = str(data_dir / "default-yakssa.db")
    env["PATH"] = env.get("PATH", "") + ":/usr/local/go/bin:" + os.path.expanduser("~/.local/bin") + ":" + os.path.expanduser("~/go/bin")

    try:
        result = subprocess.run(
            ["bash", str(gen_script), str(pr_number), short_sha, str(scan_config)],
            cwd=str(worktree),
            env=env,
            capture_output=True,
            text=True,
            timeout=30,
            check=True,
        )
    except Exception as e:
        log(f"PR #{pr_number} scan: generate config failed: {e}", "ERROR")
        return False

    # Stage 3: run scan
    db_path = data_dir / "default-yakssa.db"
    log(f"PR #{pr_number} scan: running yak ssa-compile...")
    try:
        result = subprocess.run(
            [str(worktree / "yak"), "ssa-compile",
             "--config", str(scan_config),
             "--database", str(db_path),
             "--file-perf-log"],
            cwd=str(ci_dir),
            env=env,
            capture_output=True,
            text=True,
            timeout=300,
        )
        # Write to scan log
        with open(scan_log, "w") as f:
            f.write(f"=== scan PR#{pr_number} {short_sha} ===\n")
            f.write(f"=== exit code: {result.returncode} ===\n\n")
            f.write("--- stdout ---\n")
            f.write(result.stdout or "")
            f.write("\n--- stderr ---\n")
            f.write(result.stderr or "")

        if result.returncode != 0:
            if result.stderr:
                for line in result.stderr.splitlines()[-10:]:
                    print(f"  [scan:err] {line}", flush=True)
            log(f"PR #{pr_number} scan failed (exit {result.returncode}), see {scan_log}", "ERROR")
            return False
        log(f"PR #{pr_number} scan completed, see {scan_log}")
        return True
    except subprocess.TimeoutExpired:
        log(f"PR #{pr_number} scan timed out after 300s", "ERROR")
        return False
    except Exception as e:
        log(f"PR #{pr_number} scan exception: {e}", "ERROR")
        return False


# ---------------------------------------------------------------------------
# Check cycle — merge/close detection + promote
# ---------------------------------------------------------------------------

def check_merge_and_promote(
    repo: str,
    worktree: Path,
    data_dir: Path,
    token: str | None,
) -> bool:
    """
    Check if main HEAD advanced; if so, find merged PRs and run promote.
    Returns True if promote was executed.
    """
    # 1. Get main HEAD
    try:
        main_head = get_main_head(repo, token)
    except Exception as e:
        log(f"GitHub API failed ({e}), falling back to git fetch", "WARN")
        main_head = get_main_head_from_worktree(worktree)
        if not main_head:
            log("Could not determine main HEAD", "ERROR")
            return False

    # 2. Read manifest
    manifest = read_manifest(data_dir)
    if manifest is None:
        log("No manifest found, run weekly full compile first", "ERROR")
        return False

    manifest_sha = manifest.get("main_sha", "")
    manifest_depth = manifest.get("overlay_depth", 0)

    # 3. Idle check
    if main_head == manifest_sha:
        return False

    if not manifest_sha:
        log("manifest main_sha is empty, nothing to compare", "WARN")
        return False

    log(f"main advanced: {manifest_sha[:8]} -> {main_head[:8]} (depth={manifest_depth})")

    # 4. Fetch merged PRs
    try:
        merged_prs = get_merged_prs_in_range(repo, manifest_sha, main_head, token)
    except Exception as e:
        log(f"get_merged_prs failed ({e}), proceeding without PR info", "WARN")
        merged_prs = []

    if merged_prs:
        pr_list = ", ".join(f"#{p['number']}" for p in merged_prs)
        log(f"merged PRs: {pr_list}")
        for pr in merged_prs:
            append_event(data_dir, {
                "type": "merge",
                "pr_number": pr["number"],
                "title": pr.get("title", ""),
                "sha": pr.get("merge_commit_sha", main_head),
                "html_url": pr.get("html_url", ""),
            })
    else:
        log("no PRs found in range (may be direct push or search miss)", "WARN")

    # 5. Run promote
    pr_number = str(merged_prs[-1]["number"]) if merged_prs else ""
    success = run_promote(repo, worktree, data_dir, manifest_sha, main_head, pr_number, token)

    # 6. Verify
    new_manifest = read_manifest(data_dir)
    if new_manifest:
        new_sha = new_manifest.get("main_sha", "")
        new_depth = new_manifest.get("overlay_depth", 0)
        if new_sha == main_head:
            log(f"\u2705 promote verified: main={new_sha[:8]} depth={new_depth}")
        else:
            log(f"\u26a0\ufe0f manifest sha {new_sha[:8]} != main HEAD {main_head[:8]}", "WARN")

    return True


# ---------------------------------------------------------------------------
# Check new open PRs — run CI scan
# ---------------------------------------------------------------------------

def check_new_open_prs(
    repo: str,
    worktree: Path,
    data_dir: Path,
    token: str | None,
    known_open_prs: set[int],
) -> set[int]:
    """
    Detect newly opened PRs since last check.
    Runs CI scan for each new PR.
    Returns updated known_open_prs set.
    """
    current_open = get_open_prs(repo, token)
    current_open_numbers = {pr["number"] for pr in current_open}

    # Find new PRs (in current but not in known)
    new_prs = [pr for pr in current_open if pr["number"] not in known_open_prs]

    for pr in new_prs:
        log(f"PR #{pr['number']} opened: {pr.get('title', '')[:50]}")
        append_event(data_dir, {
            "type": "open",
            "pr_number": pr["number"],
            "title": pr.get("title", ""),
            "sha": pr.get("head_sha", ""),
            "html_url": pr.get("html_url", ""),
        })
        # Run CI scan
        head_sha = pr.get("head_sha", "")
        if head_sha:
            success = run_pr_scan(repo, worktree, data_dir, pr["number"], head_sha, token)
            log(f"PR #{pr['number']} scan {'succeeded' if success else 'failed'}")
        else:
            log(f"PR #{pr['number']} has no head SHA, skipping scan", "WARN")

    # Update known set
    known_open_prs.update(current_open_numbers)
    return known_open_prs


# ---------------------------------------------------------------------------
# Check closed PRs — record close events
# ---------------------------------------------------------------------------

def check_closed_prs(
    repo: str,
    token: str | None,
    data_dir: Path,
    known_closed_prs: set[int],
) -> set[int]:
    """
    Detect newly closed (non-merged) PRs and record close events.
    Returns updated known_closed_prs set.
    """
    closed_prs = get_recently_closed_prs(repo, token)
    for pr in closed_prs:
        pr_number = pr["number"]
        if pr_number in known_closed_prs:
            continue
        if pr.get("merged"):
            # Merged PRs are tracked as merge events, not close events
            known_closed_prs.add(pr_number)
            continue
        log(f"PR #{pr_number} closed: {pr.get('title', '')[:50]}")
        append_event(data_dir, {
            "type": "close",
            "pr_number": pr_number,
            "title": pr.get("title", ""),
            "html_url": pr.get("html_url", ""),
        })
        known_closed_prs.add(pr_number)

    return known_closed_prs


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

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
    token = os.environ.get("GITHUB_TOKEN", "") or None

    log(f"CI Promote Monitor started | repo={args.repo} interval={args.interval}s "
        f"mode={'once' if args.once else 'poll'} token={'yes' if token else 'no'}")

    if not worktree.exists():
        log(f"worktree not found: {worktree}", "ERROR")
        sys.exit(1)
    if not data_dir.exists():
        log(f"data_dir not found: {data_dir}", "ERROR")
        sys.exit(1)

    # Clear events.json on each fresh start
    events_path = data_dir / "events.json"
    if events_path.exists():
        events_path.unlink()
        log("Cleared events.json")
    # Also start with empty events
    events_path.write_text("[]\n")

    # Initialize baselines: record current state so we only process NEW changes
    log("Initializing PR baseline...")
    known_open_prs: set[int] = set()
    known_closed_prs: set[int] = set()

    initial_open = get_open_prs(args.repo, token)
    for pr in initial_open:
        known_open_prs.add(pr["number"])
    initial_closed = get_recently_closed_prs(args.repo, token)
    for pr in initial_closed:
        known_closed_prs.add(pr["number"])
    log(f"Baseline: {len(known_open_prs)} open PRs, {len(known_closed_prs)} closed PRs")

    while True:
        try:
            # 1. Check for new open PRs → run CI scan
            known_open_prs = check_new_open_prs(
                args.repo, worktree, data_dir, token, known_open_prs,
            )

            # 2. Check for closed PRs → record close events
            known_closed_prs = check_closed_prs(
                args.repo, token, data_dir, known_closed_prs,
            )

            # 3. Check main advancement → run promote for merged PRs
            promoted = check_merge_and_promote(
                args.repo, worktree, data_dir, token,
            )

            # 4. Idle log if nothing happened
            if not promoted:
                manifest = read_manifest(data_dir)
                depth = manifest.get("overlay_depth", 0) if manifest else 0
                main_sha = manifest.get("main_sha", "????????")[:8] if manifest else "????????"
                log_short(f"idle: main={main_sha} depth={depth} open_prs={len(known_open_prs)}")

        except KeyboardInterrupt:
            log("Interrupted by user, exiting")
            break
        except Exception as e:
            log(f"check cycle error: {e}", "ERROR")

        if args.once:
            break
        try:
            time.sleep(args.interval)
        except KeyboardInterrupt:
            log("Interrupted by user, exiting")
            break


if __name__ == "__main__":
    main()