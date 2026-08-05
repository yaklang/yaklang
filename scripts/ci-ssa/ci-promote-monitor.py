#!/usr/bin/env python3
"""
ci-promote-monitor.py — Monitor yaklang/yaklang for PR lifecycle events and
run SSA incremental compile / promote flows locally.

Event-driven model (only processes PRs that change *during* monitoring):
  - open  : PR first seen during monitoring
            - startup baseline: record hash only, no CI (zero→one init)
            - new PR after startup: record hash + run CI
  - push  : open PR's head SHA changed (new commits pushed) → update sha, run CI
  - merge : PR merged into main during monitoring → run promote (update base)
  - close : PR closed (non-merge) during monitoring → record only, no action

Data files (all map-structured, keyed by PR number string):
  events.json      — {"active": {...}, "inactive": {...}}
                      active:   state="open"
                      inactive: state="close" or "merge"
  scan-queue.json  — {"active": {...}, "inactive": {...}}
                      active:   status=pending/running/pending_merge/merge_verifying
                      inactive: status=completed/failed/merge_done
  merge-queue.json — list of PRs awaiting promote (unchanged)

Each open PR is tracked by its head SHA. When the SHA changes (simulating a
PR pushing new commits), a new CI scan is triggered. Old diff programs for
that PR are cleaned up at the start of the new scan (Stage 0). PRs open at
startup have their hashes recorded without running CI (baseline init); new
PRs that appear during monitoring run CI on first detection.

Usage:
  python3 ci-promote-monitor.py [--once] [--interval 300] [--repo yaklang/yaklang]

Environment:
  GITHUB_TOKEN  Optional. Raises API rate limit from 60 to 5000 req/hour.
"""

import argparse
import base64
import glob
import json
import os
import subprocess
import tempfile
import threading
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
DEFAULT_INTERVAL = 120  # 2 minutes
# Derive worktree from this script's location (scripts/ci-ssa → repo root).
_SCRIPTS_DIR = Path(__file__).resolve().parent
DEFAULT_WORKTREE = str(_SCRIPTS_DIR.parents[1])
DEFAULT_DATA_DIR = "./ci-ssa-data"
API_MAX_RETRIES = 10
API_BASE_WAIT = 60  # seconds, multiplied by attempt number


def _build_env(base: dict | None = None) -> dict:
    """Build an environment dict with PATH augmented for Go tooling.

    Uses tempdir / homedir expansion instead of hard-coded absolute paths.
    """
    import shutil
    env = (base or os.environ).copy()
    extra = [
        os.path.expanduser("~/.local/bin"),
        os.path.expanduser("~/go/bin"),
    ]
    # If go is not on PATH, try to locate its bin dir.
    if not shutil.which("go"):
        go_bin = os.path.expanduser("~/.local/go/bin")
        if os.path.isdir(go_bin):
            extra.insert(0, go_bin)
    env["PATH"] = env.get("PATH", "") + ":" + ":".join(extra)
    return env

TUI_MODE = False  # set by --tui flag
_LOG_BUFFER: list[str] = []  # ring buffer for TUI event log
_LOG_BUFFER_MAX = 50

# Prefetch thread for scan queue: while scanning PR #N, downloads fs.zip for
# PR #N+1 in the background so the next execute_pr_scan skips the IO wait.
_prefetch_thread: threading.Thread | None = None
_prefetch_pr_number: int | None = None  # which PR is being prefetched


# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------

def log(msg: str, level: str = "INFO") -> None:
    ts = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    line = f"[{ts}] [{level}] {msg}"
    if TUI_MODE:
        _LOG_BUFFER.append(line)
        if len(_LOG_BUFFER) > _LOG_BUFFER_MAX:
            _LOG_BUFFER.pop(0)
    else:
        print(line, flush=True)


def log_short(msg: str) -> None:
    """Compact one-liner for routine idle polls (no level tag)."""
    ts = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S")
    line = f"[{ts}] {msg}"
    if TUI_MODE:
        _LOG_BUFFER.append(line)
        if len(_LOG_BUFFER) > _LOG_BUFFER_MAX:
            _LOG_BUFFER.pop(0)
    else:
        print(line, flush=True)


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


def get_pr_status(repo: str, pr_number: int, token: str | None) -> dict | None:
    """
    Query a single PR's final status via pulls API.
    Returns dict with: number, state, merged, merge_commit_sha, title, html_url.
    Returns None on API failure.
    """
    url = f"{GITHUB_API}/repos/{repo}/pulls/{pr_number}"
    r = api_request_with_retry(url, token)
    if r is None:
        return None
    data = r.json()
    merged = bool(data.get("merged_at"))
    return {
        "number": data["number"],
        "state": data.get("state", ""),  # "open", "closed"
        "merged": merged,
        "merge_commit_sha": data.get("merge_commit_sha", ""),
        "title": data.get("title", ""),
        "html_url": data.get("html_url", ""),
    }


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


def advance_manifest_sha(data_dir: Path, new_sha: str) -> bool:
    """Advance manifest.main_sha to new_sha without running promote.

    Used when main has advanced but there is nothing to promote (no merged PR
    ran CI, or no diff program in DB). Keeps base_program_name and depth as-is
    so the next poll doesn't keep retrying the same range forever.
    """
    manifest = read_manifest(data_dir)
    if manifest is None:
        return False
    manifest["main_sha"] = new_sha
    manifest["updated_at"] = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    try:
        (data_dir / "manifest.json").write_text(
            json.dumps(manifest, indent=2, ensure_ascii=False) + "\n"
        )
        return True
    except Exception as e:
        log(f"Failed to advance manifest main_sha: {e}", "ERROR")
        return False


def list_programs(worktree: Path, data_dir: Path) -> list[str]:
    """List all program names in the SSA database."""
    yak_bin = worktree / "yak"
    db_path = data_dir / "default-yakssa.db"
    if not yak_bin.exists() or not db_path.exists():
        return []
    try:
        result = subprocess.run(
            [str(yak_bin), "ssa-program", "--database", str(db_path)],
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
    except Exception:
        return []


def pr_has_diff_program(worktree: Path, data_dir: Path, pr_number: int) -> bool:
    """Check if a diff program exists in the DB for this PR number."""
    programs = list_programs(worktree, data_dir)
    prefix = f"ci-yaklang-diff-pr-{pr_number}-"
    return any(p.startswith(prefix) for p in programs)


# ---------------------------------------------------------------------------
# Event log — PR lifecycle state tracked in events.json (map by PR number)
# ---------------------------------------------------------------------------

def read_events(data_dir: Path) -> dict:
    """Read events.json as a map structure.

    Returns {"active": {pr_number_str: {...}}, "inactive": {pr_number_str: {...}}}.
    active = open PRs; inactive = closed/merged PRs.
    """
    path = data_dir / "events.json"
    if not path.exists():
        return {"active": {}, "inactive": {}}
    try:
        data = json.loads(path.read_text())
        if not isinstance(data, dict):
            return {"active": {}, "inactive": {}}
        if "active" not in data:
            data["active"] = {}
        if "inactive" not in data:
            data["inactive"] = {}
        return data
    except Exception:
        return {"active": {}, "inactive": {}}


def write_events(data_dir: Path, events: dict) -> None:
    """Write events.json map structure."""
    path = data_dir / "events.json"
    try:
        path.write_text(json.dumps(events, indent=2, ensure_ascii=False) + "\n")
    except Exception as e:
        log(f"Failed to write events.json: {e}", "ERROR")


def upsert_event(
    data_dir: Path,
    pr_number: int,
    state: str,
    sha: str = "",
    title: str = "",
    html_url: str = "",
) -> None:
    """Insert or update a PR's event entry.

    state="open" → placed in active map.
    state="close" or "merge" → moved to inactive map.
    """
    events = read_events(data_dir)
    key = str(pr_number)
    now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    entry = {
        "pr_number": pr_number,
        "state": state,
        "sha": sha,
        "title": title,
        "html_url": html_url,
        "updated_at": now,
    }
    if state == "open":
        events["active"][key] = entry
        events["inactive"].pop(key, None)
    else:
        # close or merge → move to inactive
        events["inactive"][key] = entry
        events["active"].pop(key, None)
    write_events(data_dir, events)


def get_pr_sha_from_events(events: dict, pr_number: int) -> str | None:
    """Get the recorded SHA for a PR from events.json."""
    key = str(pr_number)
    entry = events["active"].get(key)
    if entry is None:
        entry = events["inactive"].get(key)
    if entry is None:
        return None
    return entry.get("sha", "")


def rebuild_pr_hashes_from_events(events: dict) -> dict[int, str]:
    """Rebuild pr_hashes from events active map (open PRs)."""
    pr_hashes: dict[int, str] = {}
    for key, entry in events.get("active", {}).items():
        sha = entry.get("sha", "")
        if sha:
            pr_number = entry.get("pr_number", int(key))
            pr_hashes[pr_number] = sha
    return pr_hashes


# ---------------------------------------------------------------------------
# Merge queue — persistent promote queue (merge-queue.json)
# Ensures each merged PR is promoted individually and its diff programs
# are cleaned up, even when multiple PRs merge between polls.
# ---------------------------------------------------------------------------

def read_merge_queue(data_dir: Path) -> list[dict]:
    """Read the merge queue from merge-queue.json."""
    path = data_dir / "merge-queue.json"
    if not path.exists():
        return []
    try:
        queue = json.loads(path.read_text())
        if not isinstance(queue, list):
            return []
        return queue
    except Exception:
        return []


def write_merge_queue(data_dir: Path, queue: list[dict]) -> None:
    """Write the merge queue to merge-queue.json."""
    path = data_dir / "merge-queue.json"
    try:
        path.write_text(json.dumps(queue, indent=2, ensure_ascii=False) + "\n")
    except Exception as e:
        log(f"Failed to write merge-queue.json: {e}", "ERROR")


def add_to_merge_queue(data_dir: Path, prs: list[dict]) -> int:
    """Add merged PRs to the queue. Skips PRs already queued.

    Args:
        prs: list of dicts with keys 'number', 'merge_commit_sha' (or 'sha'),
             'title', 'html_url'.

    Returns the number of PRs actually added.
    """
    queue = read_merge_queue(data_dir)
    existing = {item["pr_number"] for item in queue}
    now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    added = 0
    for pr in prs:
        pr_number = pr["number"]
        if pr_number in existing:
            continue
        queue.append({
            "pr_number": pr_number,
            "merge_sha": pr.get("merge_commit_sha", pr.get("sha", "")),
            "title": pr.get("title", ""),
            "html_url": pr.get("html_url", ""),
            "status": "pending",
            "added_at": now,
            "promoted_at": None,
        })
        existing.add(pr_number)
        added += 1
    if added:
        write_merge_queue(data_dir, queue)
    return added


def remove_from_merge_queue(data_dir: Path, pr_number: int) -> None:
    """Remove a PR from the merge queue after successful promote."""
    queue = read_merge_queue(data_dir)
    queue = [item for item in queue if item["pr_number"] != pr_number]
    write_merge_queue(data_dir, queue)


# ---------------------------------------------------------------------------
# Scan queue — persistent CI scan queue (scan-queue.json)
# Ensures PR hash changes are processed one at a time, serially.
# ---------------------------------------------------------------------------

def read_scan_queue(data_dir: Path) -> dict:
    """Read scan-queue.json as a map structure.

    Returns {"active": {pr_number_str: {...}}, "inactive": {pr_number_str: {...}}}.
    active = pending/running; inactive = completed/failed.
    """
    path = data_dir / "scan-queue.json"
    if not path.exists():
        return {"active": {}, "inactive": {}}
    try:
        data = json.loads(path.read_text())
        if not isinstance(data, dict):
            return {"active": {}, "inactive": {}}
        if "active" not in data:
            data["active"] = {}
        if "inactive" not in data:
            data["inactive"] = {}
        return data
    except Exception:
        return {"active": {}, "inactive": {}}


def write_scan_queue(data_dir: Path, queue: dict) -> None:
    """Write scan-queue.json map structure."""
    path = data_dir / "scan-queue.json"
    try:
        path.write_text(json.dumps(queue, indent=2, ensure_ascii=False) + "\n")
    except Exception as e:
        log(f"Failed to write scan-queue.json: {e}", "ERROR")


def add_to_scan_queue(data_dir: Path, prs: list[dict]) -> int:
    """Add PRs to the scan queue active map with status=pending.

    Args:
        prs: list of dicts with keys 'number', 'head_sha', 'title', 'html_url'.

    Returns the number of PRs actually added.
    """
    queue = read_scan_queue(data_dir)
    existing = set(queue["active"].keys()) | set(queue["inactive"].keys())
    now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    added = 0
    for pr in prs:
        pr_number = pr["number"]
        key = str(pr_number)
        if key in existing:
            continue
        queue["active"][key] = {
            "pr_number": pr_number,
            "head_sha": pr.get("head_sha", ""),
            "title": pr.get("title", ""),
            "html_url": pr.get("html_url", ""),
            "status": "pending",
            "added_at": now,
            "scanned_at": None,
        }
        existing.add(key)
        added += 1
    if added:
        write_scan_queue(data_dir, queue)
    return added


def update_scan_status(data_dir: Path, pr_number: int, status: str) -> None:
    """Update a PR's scan status and move between active/inactive maps.

    Active statuses (kept in active map):
        pending, running, pending_merge, merge_verifying
    Inactive statuses (moved to inactive map):
        completed, failed, merge_done
    """
    queue = read_scan_queue(data_dir)
    key = str(pr_number)
    now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    item = queue["active"].get(key)
    if item is None:
        item = queue["inactive"].get(key)
    if item is None:
        return
    item["status"] = status
    if status in ("completed", "failed", "merge_done"):
        item["scanned_at"] = now
        queue["active"].pop(key, None)
        queue["inactive"][key] = item
    else:
        # pending / running / pending_merge / merge_verifying → active
        queue["inactive"].pop(key, None)
        queue["active"][key] = item
    write_scan_queue(data_dir, queue)


def remove_from_scan_queue(data_dir: Path, pr_number: int) -> None:
    """Remove a PR from the scan queue entirely (both active and inactive)."""
    queue = read_scan_queue(data_dir)
    key = str(pr_number)
    queue["active"].pop(key, None)
    queue["inactive"].pop(key, None)
    write_scan_queue(data_dir, queue)


def get_pending_scan_items(data_dir: Path) -> list[dict]:
    """Return all pending items from the active map, sorted by added_at."""
    queue = read_scan_queue(data_dir)
    pending = [
        item for item in queue["active"].values()
        if item.get("status") == "pending"
    ]
    pending.sort(key=lambda x: x.get("added_at", ""))
    return pending


def get_running_scan_item(data_dir: Path) -> dict | None:
    """Return the running item from the active map, if any."""
    queue = read_scan_queue(data_dir)
    for item in queue["active"].values():
        if item.get("status") == "running":
            return item
    return None


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
    """Create a working directory for running promote.

    No git clone is needed — promote-base-on-merge.sh uses a prebuilt fs.zip
    (built via GitHub compare API) and only needs yak, scripts, and the SSA
    data directory symlinked in. All file-change information comes from the
    GitHub API, not from a local git repository.
    """
    clone_dir = Path(tempfile.gettempdir()) / f"ci-promote-work-{new_sha[:8]}"
    if clone_dir.exists():
        return clone_dir
    try:
        clone_dir.mkdir(parents=True, exist_ok=True)
        return clone_dir
    except Exception as e:
        log(f"failed to create promote work dir: {e}", "ERROR")
        if clone_dir.exists():
            subprocess.run(["rm", "-rf", str(clone_dir)], check=False)
        return None


def clean_old_clones(keep: Path) -> None:
    """Remove all ci-promote-work-* dirs in temp dir except the one given."""
    pattern = str(Path(tempfile.gettempdir()) / "ci-promote-work-*")
    for d in glob.glob(pattern):
        p = Path(d)
        if p != keep and p.is_dir():
            subprocess.run(["rm", "-rf", str(p)], check=False)
            log(f"Cleaned old clone: {p}")


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
    env = _build_env(env)
    env["CI_SSA_PROMOTE_CATCH_UP"] = "0"
    env["FS_ZIP_PREBUILT"] = "1"

    cmd = ["bash", str(script), new_sha, pr_number]
    try:
        # Real-time log: stream stdout+stderr to file only (not terminal)
        if log_file:
            log_file.parent.mkdir(parents=True, exist_ok=True)
        proc = subprocess.Popen(
            cmd,
            cwd=str(clone_dir),
            env=env,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
        )
        with open(log_file, "w") if log_file else open(os.devnull, "w") as f:
            f.write(f"=== promote {new_sha[:8]} (PR={pr_number or 'none'}) ===\n\n")
            for line in proc.stdout:
                f.write(line)
                f.flush()
            proc.wait(timeout=600)
        rc = proc.returncode
        if rc != 0:
            log(f"promote failed (exit {rc}), see {log_file}", "ERROR") if log_file else log(f"promote failed (exit {rc})", "ERROR")
            return False
        return True
    except subprocess.TimeoutExpired:
        proc.kill()
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

    # Single range: build fs.zip for the entire old_sha..new_sha range at once.
    # No need to split per-commit — the SSA incremental compiler handles the
    # full diff in one pass. Per-commit splitting was slow (64 ranges = 64
    # downloads + 64 compiles) and caused cleanup to stack up.
    ranges = [(old_sha, new_sha)]

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

    # Clean up old clone directories (keep only the latest one for debugging)
    clean_old_clones(clone_dir)

    return True


# ---------------------------------------------------------------------------
# Run PR scan (open event → incremental diff scan)
# ---------------------------------------------------------------------------

def prepare_pr_scan(
    repo: str,
    worktree: Path,
    data_dir: Path,
    pr_number: int,
    pr_head_sha: str,
    token: str | None,
) -> dict | None:
    """
    Prepare a PR scan: cleanup, build fs.zip, write scan config (IO-bound).

    Returns a context dict with all paths/env needed by execute_pr_scan,
    or None on failure.
    """
    manifest = read_manifest(data_dir)
    if manifest is None:
        log("No manifest, cannot run PR scan", "ERROR")
        return None
    main_sha = manifest.get("main_sha", "")
    if not main_sha:
        log("manifest main_sha is empty, cannot run PR scan", "ERROR")
        return None

    ci_dir = ci_log_dir(data_dir, pr_number)
    short_sha = pr_head_sha[:8] if pr_head_sha else "unknown"
    fs_zip_path = ci_dir / "fs.zip"
    scan_config = ci_dir / "scan-config.json"

    worktree_abs = worktree.resolve()
    data_dir_abs = data_dir.resolve()
    ci_dir_abs = ci_dir.resolve()
    fs_zip_abs = fs_zip_path.resolve()
    scan_config_abs = scan_config.resolve()
    db_path_abs = data_dir_abs / "default-yakssa.db"

    env = os.environ.copy()
    env["SSA_CI_DATA_DIR"] = str(data_dir_abs)
    env["SSA_DATABASE_RAW"] = str(db_path_abs)
    env = _build_env(env)

    # Stage 0: Clean up previous diff programs for this PR.
    cleanup_script = worktree_abs / "scripts" / "ci-ssa" / "cleanup-programs.sh"
    if cleanup_script.exists():
        log(f"PR #{pr_number} scan: cleaning previous diff programs")
        subprocess.run(
            ["bash", str(cleanup_script), "pr", str(pr_number)],
            cwd=str(worktree_abs),
            env=env,
            capture_output=True,
            text=True,
            timeout=300,
            check=False,
        )

    # Stage 1: build fs.zip
    log(f"PR #{pr_number} scan: building fs.zip {main_sha[:8]}...{short_sha}")
    count = build_fs_zip_from_compare(repo, main_sha, pr_head_sha, token, fs_zip_abs)
    if count < 0:
        log(f"PR #{pr_number} scan: build_fs_zip failed", "ERROR")
        return None

    # Stage 2: generate scan config (inline — fill diff-code-scan.json template)
    template_path = worktree_abs / "scripts" / "ci-ssa" / "diff-code-scan.json"
    if not template_path.exists():
        log(f"diff-code-scan.json template not found: {template_path}", "ERROR")
        return None

    diff_name = f"ci-yaklang-diff-pr-{pr_number}-{short_sha}"
    # Read current base from manifest (or base-program-name pointer file),
    # NOT from CI_SSA_BASE_PROGRAM env — that env var is only set during
    # promote and may be stale. The manifest's base_program_name is the
    # authoritative current base after promotes.
    base_program = manifest.get("base_program_name", "")
    if not base_program:
        pointer = data_dir / "base-program-name"
        if pointer.exists():
            base_program = pointer.read_text().strip()
    if not base_program:
        base_program = "ci-yaklang-base"
    # Inject into env so cleanup-programs.sh and yak use the correct base
    env["CI_SSA_BASE_PROGRAM"] = base_program
    try:
        with open(template_path, "r", encoding="utf-8") as f:
            cfg = json.load(f)
        cfg["BaseInfo"]["program_names"] = [diff_name]
        cfg["SSACompile"]["base_program_name"] = base_program
        cfg["SSACompile"]["enable_incremental_compile"] = True
        with open(scan_config_abs, "w", encoding="utf-8") as f:
            json.dump(cfg, f, indent=2)
    except Exception as e:
        log(f"PR #{pr_number} scan: generate config failed: {e}", "ERROR")
        return None
    log(f"PR #{pr_number} scan: config written (program={diff_name} base={base_program})")

    # Mark as prepared (write head_sha marker for cache validation).
    prepared_marker = ci_dir_abs / "prepared.sha"
    prepared_marker.write_text(pr_head_sha)

    return {
        "pr_number": pr_number,
        "head_sha": pr_head_sha,
        "short_sha": short_sha,
        "worktree_abs": worktree_abs,
        "ci_dir_abs": ci_dir_abs,
        "scan_config_abs": scan_config_abs,
        "db_path_abs": db_path_abs,
        "env": env,
    }


def execute_pr_scan(ctx: dict) -> bool:
    """
    Execute the CPU-bound scan stage (yak code-scan) using a prepared context.
    Returns True on success.
    """
    pr_number = ctx["pr_number"]
    short_sha = ctx["short_sha"]
    worktree_abs = ctx["worktree_abs"]
    ci_dir_abs = ctx["ci_dir_abs"]
    scan_config_abs = ctx["scan_config_abs"]
    db_path_abs = ctx["db_path_abs"]
    env = ctx["env"]

    compile_log = ci_dir_abs / f"scan_{timestamp_str()}_{short_sha}.log"
    # Wall-clock budget for the entire scan. --rule-timeout/--rule-work-limit
    # protect individual rules, but a stuck process can hang the stdout reader
    # forever. This outer timeout kills the process if it exceeds the budget.
    SCAN_TIMEOUT = 600  # 10 minutes
    log(f"PR #{pr_number} scan: running yak code-scan (compile + rules)...")
    try:
        proc = subprocess.Popen(
            [str(worktree_abs / "yak"), "code-scan",
             "--config", str(scan_config_abs),
             "--database", str(db_path_abs),
             "--rule-perf-log",
             "--rule-timeout", "10m",
             "--rule-work-limit", "200000",
             "--format", "irify",
             "--output", str(ci_dir_abs / "risk"),
             "--file-perf-log"],
            cwd=str(ci_dir_abs),
            env=env,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
        )
        import time as _t
        start_ts = _t.monotonic()
        with open(compile_log, "w") as f:
            f.write(f"=== code-scan PR#{pr_number} {short_sha} ===\n\n")
            # Read stdout with timeout: if the process exceeds SCAN_TIMEOUT,
            # kill it instead of blocking forever in the for-loop.
            timed_out = False
            while True:
                line = proc.stdout.readline()
                if line == "":
                    # EOF — process closed stdout
                    break
                f.write(line)
                f.flush()
                elapsed = _t.monotonic() - start_ts
                if elapsed > SCAN_TIMEOUT:
                    timed_out = True
                    f.write(f"\n=== TIMEOUT: killed after {int(elapsed)}s ===\n")
                    f.flush()
                    break
            if timed_out:
                proc.kill()
                proc.wait(timeout=10)
                log(f"PR #{pr_number} scan timed out after {int(elapsed)}s, killed", "ERROR")
                return False
            proc.wait(timeout=30)
        rc = proc.returncode

        if rc != 0:
            log(f"PR #{pr_number} scan failed (exit {rc}), see {compile_log}", "ERROR")
            return False
        log(f"PR #{pr_number} scan completed, see {compile_log}")
        return True
    except subprocess.TimeoutExpired:
        proc.kill()
        log(f"PR #{pr_number} scan timed out after 600s", "ERROR")
        return False
    except Exception as e:
        log(f"PR #{pr_number} scan exception: {e}", "ERROR")
        return False


def is_pr_prepared(data_dir: Path, pr_number: int, head_sha: str) -> bool:
    """Check if a PR's fs.zip + config are already prepared for this head_sha."""
    ci_dir = data_dir / "ci-logs" / str(pr_number)
    fs_zip = ci_dir / "fs.zip"
    marker = ci_dir / "prepared.sha"
    if not fs_zip.exists() or not marker.exists():
        return False
    return marker.read_text().strip() == head_sha


def run_pr_scan(
    repo: str,
    worktree: Path,
    data_dir: Path,
    pr_number: int,
    pr_head_sha: str,
    token: str | None,
) -> bool:
    """
    Prepare + execute a PR scan in one call (no prefetch).
    Kept for compatibility; process_scan_queue uses prepare/execute separately.
    """
    if is_pr_prepared(data_dir, pr_number, pr_head_sha):
        ci_dir = ci_log_dir(data_dir, pr_number)
        worktree_abs = worktree.resolve()
        data_dir_abs = data_dir.resolve()
        ci_dir_abs = ci_dir.resolve()
        db_path_abs = data_dir_abs / "default-yakssa.db"
        env = os.environ.copy()
        env["SSA_CI_DATA_DIR"] = str(data_dir_abs)
        env["SSA_DATABASE_RAW"] = str(db_path_abs)
        env = _build_env(env)
        ctx = {
            "pr_number": pr_number,
            "head_sha": pr_head_sha,
            "short_sha": pr_head_sha[:8] if pr_head_sha else "unknown",
            "worktree_abs": worktree_abs,
            "ci_dir_abs": ci_dir_abs,
            "scan_config_abs": (ci_dir_abs / "scan-config.json").resolve(),
            "db_path_abs": db_path_abs,
            "env": env,
        }
        log(f"PR #{pr_number} scan: fs.zip already prepared, skipping download")
    else:
        ctx = prepare_pr_scan(repo, worktree, data_dir, pr_number, pr_head_sha, token)
        if ctx is None:
            return False
    return execute_pr_scan(ctx)


# ---------------------------------------------------------------------------
# Check cycle — merge/close detection + promote queue
# ---------------------------------------------------------------------------

def fix_manifest_database_url(data_dir: Path) -> None:
    """Fix manifest database url after promote.

    The promote script runs inside a work dir, so the manifest's database.url
    may point to a temp work dir.  Rewrite it to the real path.
    """
    manifest = read_manifest(data_dir)
    if not manifest:
        return
    db_url = manifest.get("database", {}).get("url", "")
    real_db_path = str((data_dir / "default-yakssa.db").resolve())
    real_db_size = (data_dir / "default-yakssa.db").stat().st_size if (data_dir / "default-yakssa.db").exists() else 0
    if "ci-promote-work-" in db_url or db_url != f"local://{real_db_path}":
        manifest["database"]["url"] = f"local://{real_db_path}"
        manifest["database"]["size_bytes"] = real_db_size
        manifest_path = data_dir / "manifest.json"
        manifest_path.write_text(json.dumps(manifest, indent=2, ensure_ascii=False) + "\n")
        log(f"Fixed manifest database url -> {real_db_path}")


def process_merge_queue(
    repo: str,
    worktree: Path,
    data_dir: Path,
    token: str | None,
) -> bool:
    """Process the merge queue: promote one PR at a time.

    For each pending item, reads the current manifest.main_sha as old_sha,
    promotes old_sha..merge_sha, cleans up that PR's diff programs, then
    removes it from the queue. Stops on first failure (retry next cycle).

    Returns True if at least one PR was promoted.
    """
    queue = read_merge_queue(data_dir)
    if not queue:
        return False

    promoted_any = False
    for item in queue:
        if item.get("status") != "pending":
            continue

        pr_number = item["pr_number"]
        merge_sha = item["merge_sha"]
        if not merge_sha:
            log(f"queue item PR #{pr_number} has no merge_sha, removing", "WARN")
            remove_from_merge_queue(data_dir, pr_number)
            continue

        # Read current manifest main_sha as old_sha (may have advanced by a
        # previous item in this loop).
        manifest = read_manifest(data_dir)
        if manifest is None:
            log("No manifest, cannot process queue", "ERROR")
            break
        old_sha = manifest.get("main_sha", "")
        if not old_sha:
            log("manifest main_sha is empty, cannot process queue", "ERROR")
            break

        if old_sha == merge_sha:
            # Already promoted (e.g. previous cycle handled it); just clean up.
            log(f"PR #{pr_number} already at merge_sha {merge_sha[:8]}, removing from queue")
            remove_from_merge_queue(data_dir, pr_number)
            update_scan_status(data_dir, pr_number, "merge_done")
            promoted_any = True
            continue

        log(f"Processing queue: PR #{pr_number} promote {old_sha[:8]} -> {merge_sha[:8]}")

        # Mark scan-queue as merge_verifying (promote in progress)
        update_scan_status(data_dir, pr_number, "merge_verifying")

        success = run_promote(
            repo, worktree, data_dir, old_sha, merge_sha, str(pr_number), token,
        )

        if success:
            # Fix manifest database url (promote ran in clone dir).
            fix_manifest_database_url(data_dir)

            # Verify promote landed.
            new_manifest = read_manifest(data_dir)
            if new_manifest:
                new_sha = new_manifest.get("main_sha", "")
                new_depth = new_manifest.get("overlay_depth", 0)
                if new_sha == merge_sha:
                    log(f"✅ PR #{pr_number} promote verified: main={new_sha[:8]} depth={new_depth}")
                else:
                    log(f"⚠️ PR #{pr_number}: manifest sha {new_sha[:8]} != merge_sha {merge_sha[:8]}", "WARN")

            # Remove from queue + mark merge_done in scan-queue.
            remove_from_merge_queue(data_dir, pr_number)
            update_scan_status(data_dir, pr_number, "merge_done")
            promoted_any = True
            log(f"PR #{pr_number} promote done, removed from queue")
        else:
            log(f"PR #{pr_number} promote failed, will retry next cycle", "ERROR")
            break  # stop processing further items; next cycle retries

    return promoted_any


# ---------------------------------------------------------------------------
# Process scan queue — run CI scan for one PR at a time (serial)
# ---------------------------------------------------------------------------

def process_scan_queue(
    repo: str,
    worktree: Path,
    data_dir: Path,
    token: str | None,
) -> bool:
    """Process the scan queue: run CI scan for one PR at a time (serial).

    Sets status=running before executing, then completed/failed after.
    While scanning PR #N, prefetches fs.zip for PR #N+1 in background.

    Returns True if a scan was executed (success or failure).
    """
    global _prefetch_thread, _prefetch_pr_number

    # Don't start if something is already running
    running = get_running_scan_item(data_dir)
    if running is not None:
        return True  # a scan is in progress (shouldn't happen in serial mode)

    pending = get_pending_scan_items(data_dir)
    if not pending:
        return False

    current = pending[0]
    pr_number = current["pr_number"]
    head_sha = current["head_sha"]
    if not head_sha:
        log(f"scan queue item PR #{pr_number} has no head_sha, marking failed", "WARN")
        update_scan_status(data_dir, pr_number, "failed")
        return True

    # Mark as running
    update_scan_status(data_dir, pr_number, "running")
    log(f"Processing scan queue: PR #{pr_number} (sha={head_sha[:8]}) status=running")

    # Check if already prepared (by a previous prefetch cycle)
    if is_pr_prepared(data_dir, pr_number, head_sha):
        log(f"PR #{pr_number} scan: fs.zip already prepared, skipping download")
        ci_dir = ci_log_dir(data_dir, pr_number)
        worktree_abs = worktree.resolve()
        data_dir_abs = data_dir.resolve()
        ci_dir_abs = ci_dir.resolve()
        db_path_abs = data_dir_abs / "default-yakssa.db"
        env = os.environ.copy()
        env["SSA_CI_DATA_DIR"] = str(data_dir_abs)
        env["SSA_DATABASE_RAW"] = str(db_path_abs)
        env = _build_env(env)
        # Inject current base from manifest (same as prepare_pr_scan)
        manifest = read_manifest(data_dir)
        base_program = manifest.get("base_program_name", "") if manifest else ""
        if not base_program:
            pointer = data_dir / "base-program-name"
            if pointer.exists():
                base_program = pointer.read_text().strip()
        if not base_program:
            base_program = "ci-yaklang-base"
        env["CI_SSA_BASE_PROGRAM"] = base_program
        ctx = {
            "pr_number": pr_number,
            "head_sha": head_sha,
            "short_sha": head_sha[:8],
            "worktree_abs": worktree_abs,
            "ci_dir_abs": ci_dir_abs,
            "scan_config_abs": (ci_dir_abs / "scan-config.json").resolve(),
            "db_path_abs": db_path_abs,
            "env": env,
        }
    else:
        ctx = prepare_pr_scan(repo, worktree, data_dir, pr_number, head_sha, token)
        if ctx is None:
            log(f"PR #{pr_number} scan prepare failed, marked failed (no retry)", "ERROR")
            update_scan_status(data_dir, pr_number, "failed")
            return True

    # Launch background prefetch for the next pending item (if any and not
    # already prefetched / not currently running).
    if len(pending) >= 2:
        nxt = pending[1]
        nxt_pr = nxt["pr_number"]
        nxt_sha = nxt["head_sha"]
        prefetch_active = (
            _prefetch_thread is not None and _prefetch_thread.is_alive()
        )
        already_prefetched = is_pr_prepared(data_dir, nxt_pr, nxt_sha) if nxt_sha else False
        if nxt_sha and not prefetch_active and not already_prefetched:
            _prefetch_pr_number = nxt_pr
            log(f"Prefetching next scan: PR #{nxt_pr} (sha={nxt_sha[:8]}) in background")

            def _do_prefetch(n_pr, n_sha):
                try:
                    prepare_pr_scan(repo, worktree, data_dir, n_pr, n_sha, token)
                    log(f"Prefetch complete: PR #{n_pr} fs.zip ready")
                except Exception as e:
                    log(f"Prefetch failed: PR #{n_pr}: {e}", "WARN")
                finally:
                    global _prefetch_thread, _prefetch_pr_number
                    _prefetch_thread = None
                    _prefetch_pr_number = None

            _prefetch_thread = threading.Thread(
                target=_do_prefetch, args=(nxt_pr, nxt_sha), daemon=True,
            )
            _prefetch_thread.start()

    # Execute scan (CPU-bound, blocks here ~10 min)
    success = execute_pr_scan(ctx)

    if success:
        update_scan_status(data_dir, pr_number, "completed")
        log(f"PR #{pr_number} scan done, marked completed")
    else:
        update_scan_status(data_dir, pr_number, "failed")
        log(f"PR #{pr_number} scan failed, marked failed (no retry)", "ERROR")

    return True


def check_merge_and_promote(
    repo: str,
    worktree: Path,
    data_dir: Path,
    token: str | None,
) -> bool:
    """
    Check if main HEAD advanced; if so, find merged PRs and queue them
    for promote. Returns True if any PRs were enqueued.
    Actual promote is handled by process_merge_queue() in the main loop.
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

    # 4. Fetch merged PRs — try GitHub API first, fall back to DB scan
    try:
        merged_prs = get_merged_prs_in_range(repo, manifest_sha, main_head, token)
    except Exception as e:
        log(f"get_merged_prs failed ({e}), proceeding without PR info", "WARN")
        merged_prs = []

    # Collect merged PRs that have a diff program (ran CI) → eligible for promote
    prs_to_queue = []

    if merged_prs:
        pr_list = ", ".join(f"#{p['number']}" for p in merged_prs)
        log(f"merged PRs: {pr_list}")
        prs_with_ci = [pr for pr in merged_prs if pr_has_diff_program(worktree, data_dir, pr["number"])]
        prs_without_ci = [pr for pr in merged_prs if not pr_has_diff_program(worktree, data_dir, pr["number"])]

        for pr in merged_prs:
            has_ci = pr in prs_with_ci
            upsert_event(data_dir, pr["number"], "merge",
                         pr.get("merge_commit_sha", main_head),
                         pr.get("title", ""), pr.get("html_url", ""))

        if not prs_with_ci:
            log(f"no merged PR has diff program (CI not run), skipping promote", "WARN")
            for pr in prs_without_ci:
                log(f"  PR #{pr['number']} merged but no diff program found, skipped")
            advance_manifest_sha(data_dir, main_head)
            log(f"manifest main_sha advanced to {main_head[:8]} (no-op promote)")
            return False

        prs_to_queue = prs_with_ci
        if prs_without_ci:
            skipped = ", ".join("#{}".format(p["number"]) for p in prs_without_ci)
            log(f"skipping {len(prs_without_ci)} PR(s) without diff program: {skipped}")
    else:
        # search API didn't match (squash merge, API miss, etc.)
        # Fall back: scan DB for any diff program — if found, query status
        log("no merged PR matched via search API, scanning DB for diff programs", "WARN")
        programs = list_programs(worktree, data_dir)
        diff_prs = set()
        for p in programs:
            if p.startswith("ci-yaklang-diff-pr-"):
                rest = p[len("ci-yaklang-diff-pr-"):]
                parts = rest.split("-", 1)
                try:
                    diff_prs.add(int(parts[0]))
                except (ValueError, IndexError):
                    pass
        if not diff_prs:
            log("no diff program in DB, skipping promote (no CI ran for this range)", "WARN")
            advance_manifest_sha(data_dir, main_head)
            log(f"manifest main_sha advanced to {main_head[:8]} (no-op promote)")
            return False
        pr_list = ", ".join(f"#{n}" for n in sorted(diff_prs))
        log(f"diff programs found for PR(s): {pr_list}")
        # Query each PR's status — only merged PRs should be queued for promote.
        merged_prs_with_ci = []
        for n in sorted(diff_prs):
            status = get_pr_status(repo, n, token)
            if status is None:
                log(f"  PR #{n}: status unknown, skipping", "WARN")
                continue
            if status["merged"]:
                merged_prs_with_ci.append(n)
                upsert_event(data_dir, n, "merge",
                             status.get("merge_commit_sha", "") or main_head,
                             status.get("title", ""), status.get("html_url", ""))
                log(f"  PR #{n}: merged, has diff program → will queue")
            else:
                log(f"  PR #{n}: {status['state']} (not merged), skipping promote for this PR")
        if not merged_prs_with_ci:
            log("no merged PR with diff program found, skipping promote", "WARN")
            advance_manifest_sha(data_dir, main_head)
            log(f"manifest main_sha advanced to {main_head[:8]} (no-op promote)")
            return False
        # Build prs_to_queue from fallback (query status already done)
        for n in merged_prs_with_ci:
            status = get_pr_status(repo, n, token)
            if status:
                prs_to_queue.append({
                    "number": n,
                    "merge_commit_sha": status.get("merge_commit_sha", "") or main_head,
                    "title": status.get("title", ""),
                    "html_url": status.get("html_url", ""),
                })

    # 5. Queue all eligible merged PRs for individual promote
    # Sort by merge_commit_sha to ensure linear promote order
    prs_to_queue.sort(key=lambda pr: pr.get("merge_commit_sha", ""))
    added = add_to_merge_queue(data_dir, prs_to_queue)
    if added:
        queued = ", ".join(f"#{pr['number']}" for pr in prs_to_queue)
        log(f"queued {added} PR(s) for promote: {queued}")
        # Mark scan-queue status as pending_merge for each queued PR
        for pr in prs_to_queue:
            update_scan_status(data_dir, pr["number"], "pending_merge")

    return added > 0


# ---------------------------------------------------------------------------
# Check open PRs for hash changes — run CI scan on push
# ---------------------------------------------------------------------------

def check_open_pr_pushes(
    repo: str,
    worktree: Path,
    data_dir: Path,
    token: str | None,
    pr_hashes: dict[int, str],
    baseline_prs: set[int],
) -> dict[int, str]:
    """
    Check all currently open PRs for head SHA changes.
    - First time a PR is seen during monitoring (new PR):
      record hash + enqueue CI scan.
    - First time a PR is seen during baseline init (already open at startup):
      record hash, no CI (zero→one init from baseline).
    - If an open PR's hash changed: update events 'open' sha, enqueue CI scan.
    - If a previously-open PR is no longer open: it was merged or closed
      (handled by merge/close checks), remove from tracking.
    Returns updated pr_hashes dict.
    """
    current_open = get_open_prs(repo, token)
    current_open_numbers = {pr["number"] for pr in current_open}

    prs_to_scan = []  # collect PRs that need CI scan

    for pr in current_open:
        pr_number = pr["number"]
        head_sha = pr.get("head_sha", "")
        old_sha = pr_hashes.get(pr_number)

        if old_sha is None:
            # First time seeing this PR
            pr_hashes[pr_number] = head_sha
            if pr_number in baseline_prs:
                # Part of startup baseline — record hash only, no CI
                log(f"PR #{pr_number} opened: {pr.get('title', '')[:50]} "
                    f"sha={head_sha[:8]} (baseline init, no CI)")
                upsert_event(data_dir, pr_number, "open", head_sha,
                             pr.get("title", ""), pr.get("html_url", ""))
            else:
                # New PR appeared during monitoring — enqueue CI
                log(f"PR #{pr_number} opened: {pr.get('title', '')[:50]} "
                    f"sha={head_sha[:8]} (new PR, enqueuing CI)")
                upsert_event(data_dir, pr_number, "open", head_sha,
                             pr.get("title", ""), pr.get("html_url", ""))
                if head_sha:
                    prs_to_scan.append(pr)
                else:
                    log(f"PR #{pr_number} has no head SHA, skipping scan", "WARN")
        elif old_sha != head_sha:
            # Hash changed — new commits pushed to this PR
            log(f"PR #{pr_number} pushed: {old_sha[:8]} -> {head_sha[:8]} "
                f"{pr.get('title', '')[:40]}")
            upsert_event(data_dir, pr_number, "open", head_sha,
                         pr.get("title", ""), pr.get("html_url", ""))
            pr_hashes[pr_number] = head_sha
            if head_sha:
                prs_to_scan.append(pr)
            else:
                log(f"PR #{pr_number} has no head SHA, skipping scan", "WARN")
        else:
            # Hash unchanged — no action
            pass

    # Enqueue all PRs that need CI scan
    if prs_to_scan:
        added = add_to_scan_queue(data_dir, prs_to_scan)
        if added:
            queued = ", ".join(f"#{pr['number']}" for pr in prs_to_scan)
            log(f"queued {added} PR(s) for CI scan: {queued}")

    # Log a compact summary of all open PRs and their hashes
    if pr_hashes:
        summary_parts = []
        for pr in sorted(current_open, key=lambda p: p["number"]):
            n = pr["number"]
            h = pr_hashes.get(n, "")[:8]
            summary_parts.append(f"#{n}={h}")
        log_short(f"open PRs ({len(current_open)}): {' '.join(summary_parts)}")

    # Remove PRs that are no longer open — query final status and handle
    gone = set(pr_hashes.keys()) - current_open_numbers
    for n in gone:
        old = pr_hashes.pop(n)
        # Also remove from scan queue if present (PR no longer open)
        remove_from_scan_queue(data_dir, n)
        # Query PR status to distinguish merge vs close
        status = get_pr_status(repo, n, token)
        if status is None:
            log(f"PR #{n} no longer open (was {old[:8]}), status unknown, removed from tracking", "WARN")
            continue
        if status["merged"]:
            # PR was merged — record merge event, promote will be handled by
            # check_merge_and_promote when it detects main advancement
            log(f"PR #{n} merged (was {old[:8]}), removed from tracking")
            upsert_event(data_dir, n, "merge",
                         status.get("merge_commit_sha", ""),
                         status.get("title", ""), status.get("html_url", ""))
        else:
            # PR was closed (non-merge) — record close event + clean up diff programs
            log(f"PR #{n} closed (was {old[:8]}), cleaning diff programs")
            upsert_event(data_dir, n, "close", "",
                         status.get("title", ""), status.get("html_url", ""))
            # Clean up diff programs for this closed PR
            cleanup_script = worktree / "scripts" / "ci-ssa" / "cleanup-programs.sh"
            if cleanup_script.exists():
                env = os.environ.copy()
                env["SSA_CI_DATA_DIR"] = str(data_dir.resolve())
                env["SSA_DATABASE_RAW"] = str((data_dir / "default-yakssa.db").resolve())
                env = _build_env(env)
                subprocess.run(
                    ["bash", str(cleanup_script), "pr", str(n)],
                    cwd=str(worktree),
                    env=env,
                    capture_output=True,
                    text=True,
                    timeout=300,
                    check=False,
                )
                log(f"PR #{n} diff programs cleaned")

    return pr_hashes


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
        upsert_event(data_dir, pr_number, "close", "",
                     pr.get("title", ""), pr.get("html_url", ""))
        known_closed_prs.add(pr_number)

    return known_closed_prs


def reconcile_db_diff_programs(
    repo: str,
    worktree: Path,
    data_dir: Path,
    token: str | None,
    pr_hashes: dict[int, str],
    baseline_prs: set[int],
) -> tuple[dict[int, str], set[int]]:
    """
    Scan DB for leftover diff programs from a previous monitor run.
    For each PR that has a diff program in the DB:
    - If PR is still open: add to pr_hashes + baseline_prs (skip first CI,
      diff program already exists from previous run)
    - If PR is merged: record merge event (promote will handle it)
    - If PR is closed: clean up diff programs from DB
    Returns updated (pr_hashes, baseline_prs).
    """
    programs = list_programs(worktree, data_dir)
    diff_prs: set[int] = set()
    for p in programs:
        if p.startswith("ci-yaklang-diff-pr-"):
            rest = p[len("ci-yaklang-diff-pr-"):]
            parts = rest.split("-", 1)
            try:
                diff_prs.add(int(parts[0]))
            except (ValueError, IndexError):
                pass

    if not diff_prs:
        log("No leftover diff programs in DB")
        return pr_hashes, baseline_prs

    log(f"Found {len(diff_prs)} leftover diff program(s) in DB: "
        f"{', '.join(f'#{n}' for n in sorted(diff_prs))}")

    env = os.environ.copy()
    env["SSA_CI_DATA_DIR"] = str(data_dir.resolve())
    env["SSA_DATABASE_RAW"] = str((data_dir / "default-yakssa.db").resolve())
    env = _build_env(env)
    cleanup_script = worktree / "scripts" / "ci-ssa" / "cleanup-programs.sh"

    # Fetch open PRs once (instead of per-PR) to avoid N redundant API calls.
    all_open_prs = get_open_prs(repo, token)
    open_pr_map = {pr["number"]: pr for pr in all_open_prs}

    for pr_number in sorted(diff_prs):
        status = get_pr_status(repo, pr_number, token)
        if status is None:
            log(f"  PR #{pr_number}: status unknown, keeping diff program", "WARN")
            continue

        if status["state"] == "open":
            # PR still open — add to tracking with current hash, skip first CI
            pr = open_pr_map.get(pr_number)
            head_sha = pr.get("head_sha", "") if pr else ""
            pr_hashes[pr_number] = head_sha
            baseline_prs.add(pr_number)
            log(f"  PR #{pr_number}: still open (sha={head_sha[:8]}), "
                f"added to tracking (baseline, no CI)")
        elif status["merged"]:
            log(f"  PR #{pr_number}: merged, recording merge event + queuing for promote")
            upsert_event(data_dir, pr_number, "merge",
                         status.get("merge_commit_sha", ""),
                         status.get("title", ""), status.get("html_url", ""))
            # Queue for promote so process_merge_queue handles it in the
            # main loop (ensures proper cleanup + individual overlay layer).
            add_to_merge_queue(data_dir, [{
                "number": pr_number,
                "merge_commit_sha": status.get("merge_commit_sha", ""),
                "title": status.get("title", ""),
                "html_url": status.get("html_url", ""),
            }])
            update_scan_status(data_dir, pr_number, "pending_merge")
        else:
            # Closed (non-merge) — clean up diff programs
            log(f"  PR #{pr_number}: closed, cleaning diff programs")
            upsert_event(data_dir, pr_number, "close", "",
                         status.get("title", ""), status.get("html_url", ""))
            if cleanup_script.exists():
                subprocess.run(
                    ["bash", str(cleanup_script), "pr", str(pr_number)],
                    cwd=str(worktree),
                    env=env,
                    capture_output=True,
                    text=True,
                    timeout=300,
                    check=False,
                )
                log(f"  PR #{pr_number}: diff programs cleaned")

    return pr_hashes, baseline_prs


# ---------------------------------------------------------------------------
# curses TUI rendering (htop-style queue monitor)
# ---------------------------------------------------------------------------

def _render_tui(stdscr, data_dir: Path, pr_hashes: dict[int, str]) -> None:
    """Render the TUI: status bar + scan queue + merge queue + event log."""
    import curses

    stdscr.erase()
    h, w = stdscr.getmaxyx()
    y = 0

    def addline(text: str, attr=curses.A_NORMAL, max_w: int = None):
        nonlocal y
        if y >= h - 1:
            return
        mw = max_w or w
        stdscr.addnstr(y, 0, text[:mw], mw, attr)
        y += 1

    # --- Header ---
    manifest = read_manifest(data_dir)
    main_sha = manifest.get("main_sha", "????????")[:8] if manifest else "????????"
    depth = manifest.get("overlay_depth", 0) if manifest else 0
    base = manifest.get("base_program_name", "????") if manifest else "????"
    now = datetime.now(timezone.utc).strftime("%H:%M:%S")
    addline(f" CI SSA Monitor  [{now}]  q=quit", curses.A_REVERSE)
    addline(f" main={main_sha}  depth={depth}  base={base}  open_prs={len(pr_hashes)}")

    # --- Scan Queue ---
    sq = read_scan_queue(data_dir)
    sq_active = sq["active"]
    sq_inactive = sq["inactive"]
    # Sort active by added_at, inactive by scanned_at
    active_items = sorted(sq_active.values(), key=lambda x: x.get("added_at", ""))
    inactive_items = sorted(sq_inactive.values(), key=lambda x: x.get("scanned_at", "") or x.get("added_at", ""), reverse=True)

    def _age(item):
        added = item.get("added_at", "")
        if not added:
            return ""
        try:
            dt = datetime.fromisoformat(added.replace("Z", "+00:00"))
            elapsed = (datetime.now(timezone.utc) - dt).total_seconds()
            m, s = divmod(int(elapsed), 60)
            return f"  {m:02d}:{s:02d}"
        except Exception:
            return ""

    status_icon = {
        "pending": "⏳",
        "running": "▶",
        "completed": "✓",
        "failed": "✗",
        "pending_merge": "⇄",
        "merge_verifying": "⇄",
        "merge_done": "✓",
    }

    addline("")
    addline(f" CI Scan Queue (active={len(active_items)} inactive={len(inactive_items)})", curses.A_BOLD)
    max_active = max(1, h // 4)
    max_inactive = max(1, h // 6)
    if active_items:
        for item in active_items[:max_active]:
            pr = item["pr_number"]
            sha = item.get("head_sha", "")[:8]
            title = item.get("title", "")[:30]
            st = item.get("status", "?")
            icon = status_icon.get(st, "?")
            marker = "►" if st == "running" else " "
            addline(f" {marker}{icon} #{pr:<6} {sha}  {st:<14} {title}{_age(item)}")
    if inactive_items:
        for item in inactive_items[:max_inactive]:
            pr = item["pr_number"]
            sha = item.get("head_sha", "")[:8]
            title = item.get("title", "")[:30]
            st = item.get("status", "?")
            icon = status_icon.get(st, "?")
            addline(f"   {icon} #{pr:<6} {sha}  {st:<14} {title}")
    if not active_items and not inactive_items:
        addline("   (empty)")

    # --- Merge Queue ---
    mq = read_merge_queue(data_dir)
    addline("")
    addline(f" Merge Queue ({len(mq)} pending)", curses.A_BOLD)
    if mq:
        for item in mq[:h // 4]:
            pr = item["pr_number"]
            sha = item.get("merge_sha", "")[:8]
            title = item.get("title", "")[:40]
            marker = "►" if item == mq[0] else " "
            addline(f" {marker} #{pr:<6} {sha}  {title}")
    else:
        addline("   (empty)")

    # --- Events ---
    ev = read_events(data_dir)
    ev_active = sorted(ev["active"].values(), key=lambda x: int(x.get("pr_number", 0)))
    ev_inactive = sorted(ev["inactive"].values(), key=lambda x: x.get("updated_at", ""), reverse=True)
    addline("")
    addline(f" Events (active={len(ev_active)} inactive={len(ev_inactive)})", curses.A_BOLD)
    max_ev = max(1, h // 6)
    for item in ev_active[:max_ev]:
        pr = item.get("pr_number", 0)
        st = item.get("state", "?")
        sha = item.get("sha", "")[:8]
        title = item.get("title", "")[:30]
        addline(f"   #{pr:<6} {st:<6} {sha}  {title}")
    for item in ev_inactive[:max_ev]:
        pr = item.get("pr_number", 0)
        st = item.get("state", "?")
        sha = item.get("sha", "")[:8]
        title = item.get("title", "")[:30]
        addline(f"   #{pr:<6} {st:<6} {sha}  {title} (inactive)")
    if not ev_active and not ev_inactive:
        addline("   (empty)")

    # --- Event Log ---
    addline("")
    addline(" Recent Log", curses.A_BOLD)
    for line in _LOG_BUFFER[-(h - y - 2):]:
        addline(f" {line}")

    stdscr.refresh()


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
    parser.add_argument("--tui", action="store_true", help="Enable curses TUI (htop-style queue monitor)")
    args = parser.parse_args()

    global TUI_MODE
    TUI_MODE = args.tui

    worktree = Path(os.path.expanduser(args.worktree))
    data_dir = Path(os.path.expanduser(args.data_dir))
    token = os.environ.get("GITHUB_TOKEN", "") or None

    log(f"CI Promote Monitor started | repo={args.repo} interval={args.interval}s "
        f"mode={'once' if args.once else 'poll'} token={'yes' if token else 'no'}"
        f"{' tui=on' if args.tui else ''}")

    if not worktree.exists():
        log(f"worktree not found: {worktree}", "ERROR")
        sys.exit(1)
    if not data_dir.exists():
        log(f"data_dir not found: {data_dir}", "ERROR")
        sys.exit(1)

    # --- Fresh start: clear events.json + queues, rebuild from DB ---
    log("Fresh start: clearing events.json + queues...")
    scan_queue_path = data_dir / "scan-queue.json"
    merge_queue_path = data_dir / "merge-queue.json"
    write_events(data_dir, {"active": {}, "inactive": {}})
    write_scan_queue(data_dir, {"active": {}, "inactive": {}})
    if merge_queue_path.exists():
        merge_queue_path.write_text("[]\n")

    # Fetch current open PRs from GitHub
    log("Fetching open PRs from GitHub...")
    initial_open = get_open_prs(args.repo, token)
    open_pr_map: dict[int, dict] = {pr["number"]: pr for pr in initial_open}

    # Scan DB for existing diff programs — these PRs have CI results.
    log("Scanning DB for existing diff programs...")
    db_programs = list_programs(worktree, data_dir)
    db_diff_prs: set[int] = set()
    for p in db_programs:
        if p.startswith("ci-yaklang-diff-pr-"):
            rest = p[len("ci-yaklang-diff-pr-"):]
            parts = rest.split("-", 1)
            try:
                db_diff_prs.add(int(parts[0]))
            except (ValueError, IndexError):
                pass
    log(f"DB has {len(db_diff_prs)} diff program(s): "
        f"{', '.join(f'#{n}' for n in sorted(db_diff_prs)) if db_diff_prs else 'none'}")

    pr_hashes: dict[int, str] = {}
    baseline_prs: set[int] = set()

    # Build scan-queue and events for PRs with DB diff programs
    prs_to_scan: list[dict] = []
    now = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    for pr_number in sorted(db_diff_prs):
        pr = open_pr_map.get(pr_number)
        if pr is None:
            # PR no longer open — merged or closed, skip (reconcile handles)
            continue
        head_sha = pr.get("head_sha", "")
        pr_hashes[pr_number] = head_sha
        baseline_prs.add(pr_number)
        short_sha = head_sha[:8] if head_sha else ""
        scanned_sha = ""
        for p in db_programs:
            if p.startswith(f"ci-yaklang-diff-pr-{pr_number}-"):
                scanned_sha = p[len(f"ci-yaklang-diff-pr-{pr_number}-"):]
                break

        # Record in events as open
        upsert_event(data_dir, pr_number, "open", head_sha, pr.get("title", ""), pr.get("html_url", ""))

        if scanned_sha and short_sha and scanned_sha == short_sha:
            # DB diff program sha matches current GitHub head — CI is current
            log(f"  PR #{pr_number}: CI current (sha={short_sha})")
            # Mark as completed in scan-queue inactive
            queue = read_scan_queue(data_dir)
            queue["inactive"][str(pr_number)] = {
                "pr_number": pr_number,
                "head_sha": head_sha,
                "title": pr.get("title", ""),
                "html_url": pr.get("html_url", ""),
                "status": "completed",
                "added_at": now,
                "scanned_at": now,
            }
            write_scan_queue(data_dir, queue)
        else:
            # Hash mismatch — PR was pushed after last CI scan
            log(f"  PR #{pr_number}: hash mismatch (DB={scanned_sha} vs GitHub={short_sha}), enqueuing CI")
            if head_sha:
                prs_to_scan.append(pr)

    # All other open PRs (no diff program in DB) — record as baseline only.
    # No CI scan on fresh start; scanning is triggered later when the
    # monitor detects a hash change (check_open_pr_pushes).
    for pr_number, pr in sorted(open_pr_map.items()):
        if pr_number in db_diff_prs:
            continue  # already handled above
        head_sha = pr.get("head_sha", "")
        pr_hashes[pr_number] = head_sha
        baseline_prs.add(pr_number)
        log(f"  PR #{pr_number}: no CI in DB, recorded as baseline (sha={head_sha[:8]})")
        upsert_event(data_dir, pr_number, "open", head_sha, pr.get("title", ""), pr.get("html_url", ""))

    # Enqueue all PRs that need CI scan
    if prs_to_scan:
        added = add_to_scan_queue(data_dir, prs_to_scan)
        if added:
            queued = ", ".join(f"#{pr['number']}" for pr in prs_to_scan)
            log(f"Queued {added} PR(s) for CI scan: {queued}")

    # Track closed PRs
    known_closed_prs: set[int] = set()
    initial_closed = get_recently_closed_prs(args.repo, token)
    for pr in initial_closed:
        known_closed_prs.add(pr["number"])
    log(f"Baseline: {len(pr_hashes)} tracked PRs, {len(known_closed_prs)} closed PRs")

    # Reconcile leftover DB diff programs for merged/closed PRs
    log("Reconciling DB diff programs...")
    pr_hashes, baseline_prs = reconcile_db_diff_programs(
        args.repo, worktree, data_dir, token, pr_hashes, baseline_prs,
    )

    # Start TUI if requested
    if args.tui:
        import curses
        curses.wrapper(lambda stdscr: _run_loop(stdscr, args, worktree, data_dir, token,
                                                pr_hashes, baseline_prs, known_closed_prs))
    else:
        _run_loop(None, args, worktree, data_dir, token,
                  pr_hashes, baseline_prs, known_closed_prs)


def _run_loop(stdscr, args, worktree, data_dir, token,
              pr_hashes, baseline_prs, known_closed_prs):
    """Main polling loop. stdscr is None in non-TUI mode."""
    import time as _time

    while True:
        try:
            # 1. Check open PRs for hash changes → enqueue CI scan
            pr_hashes = check_open_pr_pushes(
                args.repo, worktree, data_dir, token, pr_hashes, baseline_prs,
            )

            # 2. Check for closed PRs → record close events
            known_closed_prs = check_closed_prs(
                args.repo, token, data_dir, known_closed_prs,
            )

            # 3. Check main advancement → enqueue merged PRs for promote
            enqueued = check_merge_and_promote(
                args.repo, worktree, data_dir, token,
            )

            # 4. Process merge queue first (fast, ~1 min), then scan queue
            #    Promote must happen before scan so scans use the updated base.
            promoted = process_merge_queue(
                args.repo, worktree, data_dir, token,
            )
            scanned = process_scan_queue(
                args.repo, worktree, data_dir, token,
            )

            # 5. Render TUI or idle log
            if stdscr is not None:
                _render_tui(stdscr, data_dir, pr_hashes)
            else:
                if not enqueued and not promoted and not scanned:
                    manifest = read_manifest(data_dir)
                    depth = manifest.get("overlay_depth", 0) if manifest else 0
                    main_sha = manifest.get("main_sha", "????????")[:8] if manifest else "????????"
                    sq = read_scan_queue(data_dir)
                    mq = read_merge_queue(data_dir)
                    sq_a = len(sq["active"])
                    sq_i = len(sq["inactive"])
                    qinfo = (f" scan_q=active:{sq_a}/inactive:{sq_i}"
                             f" merge_q={len(mq)}") if (sq_a or sq_i or mq) else ""
                    log_short(f"idle: main={main_sha} depth={depth} open_prs={len(pr_hashes)}{qinfo}")

        except KeyboardInterrupt:
            log("Interrupted by user, exiting")
            break
        except Exception as e:
            log(f"check cycle error: {e}", "ERROR")

        if args.once:
            break
        try:
            # If a scan ran this cycle, it already consumed significant time
            # (up to SCAN_TIMEOUT). Skip the sleep to avoid adding extra delay.
            if not scanned and stdscr is not None:
                import curses
                stdscr.timeout(args.interval * 1000)
                key = stdscr.getch()
                if key == ord('q') or key == 27:  # q or ESC
                    break
            elif not scanned:
                _time.sleep(args.interval)
            # else: scan consumed enough time, loop immediately
        except KeyboardInterrupt:
            log("Interrupted by user, exiting")
            break


if __name__ == "__main__":
    main()