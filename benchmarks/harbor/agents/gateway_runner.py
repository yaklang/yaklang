#!/usr/bin/env python3
"""In-container runner for the Yak AI Agent benchmark harness.

Starts the ``yak ai-http-gateway`` inside the benchmark container, seeds
the AI provider configuration, and executes a single benchmark task via
the gateway's REST/SSE API.

Two execution modes are supported:

``react`` (default)
    Submits the task as a free-input prompt via the HTTP gateway's
    ``POST /agent/run/{run_id}`` endpoint (backed by ``StartAIReAct`` gRPC).

``forgetask``
    Submits the task via a Forge-based workflow using ``StartAITask`` gRPC.
    Requires a Forge definition YAML to be uploaded alongside the runner.
"""
from __future__ import annotations

import argparse
import base64
import json
import os
import signal
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
import uuid
from pathlib import Path


BASE_URL = "http://127.0.0.1:8089/agent"
TERMINAL_TYPES = {"completed", "cancelled", "failed", "error", "done"}


def request_json(
    method: str, path: str, payload: dict | None = None, timeout: float = 30.0
) -> dict:
    """Send a JSON request to the gateway and return the parsed response."""
    data = None if payload is None else json.dumps(payload).encode()
    request = urllib.request.Request(
        BASE_URL + path,
        data=data,
        method=method,
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            body = response.read()
        return json.loads(body) if body else {}
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(
            f"HTTP {exc.code} from {method} {path}: {body[:500]}"
        ) from exc
    except urllib.error.URLError as exc:
        raise RuntimeError(f"Connection error for {method} {path}: {exc}") from exc


def wait_gateway(timeout_sec: float = 60.0) -> None:
    """Poll ``GET /agent/setting`` until the gateway responds."""
    deadline = time.monotonic() + timeout_sec
    last_error = None
    while time.monotonic() < deadline:
        try:
            request_json("GET", "/setting")
            return
        except (OSError, urllib.error.URLError, RuntimeError) as exc:
            last_error = exc
            time.sleep(0.25)
    raise RuntimeError(
        f"Yak AI gateway did not become ready within {timeout_sec}s: {last_error}"
    )


def seed_ai_config(config_path: str) -> dict:
    """Read the tiered-ai-config YAML and POST it to the gateway's HTTP API.

    Returns the parsed configuration dict for use by the caller (e.g. for
    populating the simple ``POST /agent/setting`` fallback).
    """
    if not config_path:
        return {}
    if not os.path.isfile(config_path):
        raise FileNotFoundError(
            f"ai-config.yaml not found at {config_path} "
            "(yak_agent.py should have uploaded it)"
        )

    # Minimal YAML parser for the gen_ai_config_yaml.py output format
    # (deliberately avoids requiring PyYAML in the container).
    ai_type = ai_key = ai_domain = ai_model = ""

    def _lev(line: str) -> int:
        return len(line) - len(line.lstrip())

    section = None
    with open(config_path) as fh:
        for raw in fh:
            line = raw.rstrip("\n")
            stripped = line.strip()
            if not stripped or stripped.startswith("#"):
                continue
            if _lev(line) == 0:
                section = "root"
            elif _lev(line) == 2 and stripped.startswith("- "):
                section = "entry"
            if section == "entry":
                key, _, val = stripped.lstrip("- ").partition(":")
                key, val = key.strip(), val.strip().strip("\"'")
                if key == "type":
                    ai_type = val
                elif key == "api_key":
                    ai_key = val
                elif key == "domain":
                    ai_domain = val
                elif key == "model":
                    ai_model = val

    if not ai_type or not ai_key or not ai_domain:
        raise RuntimeError(
            f"ai-config.yaml is missing required fields "
            f"(type={ai_type!r} key={'***' if ai_key else ''!r} domain={ai_domain!r})"
        )

    parsed = {
        "type": ai_type,
        "api_key": ai_key,
        "domain": ai_domain,
        "model": ai_model or ai_type,
    }

    # ------------------------------------------------------------------
    # Path 1: Seed the simple ai-agent-chat-setting FIRST so
    # applySettingToRuntime fires with the correct AIService/AIModelName.
    # This must come BEFORE the full AIGlobalConfig because
    # applySettingToRuntime overwrites the tiered config.
    # ------------------------------------------------------------------
    setting_payload = {
        "AIService": parsed["type"],
        "AIModelName": parsed["model"],
        "UseDefaultAIConfig": False,
        "ReviewPolicy": "yolo",
        "DisableToolUse": False,
        "DisallowRequireForUserPrompt": True,
        "AllowPlanUserInteract": False,
        "EnableAISearchInternet": False,
        "EnableSystemFileSystemOperator": True,
    }
    request_json("POST", "/setting", setting_payload, timeout=30.0)
    print("[seed] ai-agent-chat-setting seeded via POST /setting", flush=True)

    # ------------------------------------------------------------------
    # Path 2: POST the full AIGlobalConfig to /setting/aiconfig AFTER the
    # simple setting. This overwrites the tiered config with the full
    # version that includes Provider.APIKey and Provider.Domain — required
    # for the agent to actually call the AI API.
    # ------------------------------------------------------------------
    aiconfig_payload = {
        "Enabled": True,
        "DisableFallback": True,
        "IntelligentModels": [
            {
                "Provider": {
                    "Type": parsed["type"],
                    "APIKey": parsed["api_key"],
                    "Domain": parsed["domain"],
                },
                "ModelName": parsed["model"],
            }
        ],
    }
    print(
        f"[seed] posting full AIGlobalConfig: type={ai_type} domain={ai_domain}",
        flush=True,
    )
    request_json("POST", "/setting/aiconfig", aiconfig_payload, timeout=30.0)
    print("[seed] AIGlobalConfig seeded via POST /setting/aiconfig", flush=True)

    return parsed


def decode_bytes(value: str | None) -> str:
    """Base64-decode a proto-bytes field, falling back to the raw string."""
    if not value:
        return ""
    try:
        return base64.b64decode(value).decode("utf-8", errors="replace")
    except (ValueError, UnicodeDecodeError):
        return value


def read_sse(run_id: str) -> tuple[list[dict], threading.Thread]:
    """Open the SSE stream for *run_id* and block until ``listener_ready``.

    MUST be called BEFORE ``POST /run/{run_id}`` so the gateway registers
    the consumer before the React loop starts.
    """

    events: list[dict] = []
    final_text: list[str] = []
    error_container: list[Exception] = []
    ready = threading.Event()

    def _stream() -> None:
        try:
            request = urllib.request.Request(f"{BASE_URL}/run/{run_id}/events")
            with urllib.request.urlopen(request, timeout=1800) as response:
                for raw_line in response:
                    line = raw_line.decode("utf-8", errors="replace").strip()
                    if not line.startswith("data:"):
                        continue
                    event = json.loads(line[5:].strip())
                    events.append(event)
                    content = decode_bytes(event.get("Content"))
                    delta = decode_bytes(event.get("StreamDelta"))
                    if content:
                        event["DecodedContent"] = content
                    if delta:
                        event["DecodedStreamDelta"] = delta
                        final_text.append(delta)
                    event_type = event.get("Type")
                    if event_type == "listener_ready":
                        ready.set()
                    if event_type in TERMINAL_TYPES:
                        break
        except Exception as exc:
            error_container.append(exc)
            ready.set()  # unblock main thread so it can surface the error

    thread = threading.Thread(target=_stream, daemon=True)
    thread.start()

    if not ready.wait(timeout=60.0):
        raise RuntimeError("SSE stream timed out waiting for listener_ready")
    if error_container:
        raise RuntimeError(
            f"SSE stream error: {error_container[0]}"
        ) from error_container[0]

    return events, thread


def wait_sse_completion(
    events: list[dict], thread: threading.Thread, timeout_sec: float = 1800.0
) -> tuple[str, str]:
    """Wait for the SSE background thread to finish.

    Returns ``(final_text, terminal_type)``.
    """
    thread.join(timeout=timeout_sec)
    if thread.is_alive():
        raise RuntimeError("SSE stream did not complete within timeout")

    final_text = "".join(
        e.get("DecodedStreamDelta", "")
        for e in events
        if e.get("DecodedStreamDelta")
    )
    terminal = events[-1].get("Type", "missing") if events else "missing"
    return final_text, terminal


def summarize(events: list[dict], duration_sec: float, final_text: str) -> dict:
    """Build a compact benchmark-summary.json payload.

    Extracts token consumption from the last ``consumption`` event
    (cumulative counters emitted by the gateway every ~15s).
    """
    type_counts: dict[str, int] = {}
    tool_events = 0
    model_events = 0
    last_consumption: dict = {}
    for event in events:
        event_type = str(event.get("Type", "unknown"))
        type_counts[event_type] = type_counts.get(event_type, 0) + 1
        lowered = event_type.lower()
        if "tool" in lowered or "call" in lowered:
            tool_events += 1
        if lowered in {"thought", "stream", "structured"}:
            model_events += 1
        if lowered == "consumption":
            try:
                dc = event.get("DecodedContent", "")
                if dc:
                    last_consumption = json.loads(dc)
            except (json.JSONDecodeError, TypeError):
                pass

    summary = {
        "duration_sec": round(duration_sec, 3),
        "event_count": len(events),
        "tool_event_count": tool_events,
        "model_event_count": model_events,
        "terminal_type": events[-1].get("Type") if events else "missing",
        "event_type_counts": type_counts,
        "final_text_chars": len(final_text),
    }

    # --- Token consumption (from last cumulative consumption event) ---
    if last_consumption:
        summary["token"] = {
            "input": last_consumption.get("input_consumption", 0),
            "output": last_consumption.get("output_consumption", 0),
            "cache_hit": last_consumption.get("cache_hit_token", 0),
        }
        tier = last_consumption.get("tier_consumption")
        if isinstance(tier, dict):
            summary["token"]["tier"] = {
                tn: {
                    "input": td.get("input_consumption", 0),
                    "output": td.get("output_consumption", 0),
                    "cache_hit": td.get("cache_hit_token", 0),
                }
                for tn, td in tier.items()
                if isinstance(td, dict)
            }

    return summary


# ---------------------------------------------------------------------------
# React-mode runner (current default)
# ---------------------------------------------------------------------------

def run_react(args: argparse.Namespace) -> int:
    """Execute a task via the HTTP gateway's ReAct loop (StartAIReAct)."""
    logs = Path("/logs/agent")
    logs.mkdir(parents=True, exist_ok=True)

    gateway_log = (logs / "gateway.log").open("wb")
    process = subprocess.Popen(
        [
            "/usr/local/bin/yak",
            "ai-http-gateway",
            "--host", "127.0.0.1",
            "--port", "8089",
            "--home", "/tmp/yak-agent-home",
        ],
        stdout=gateway_log,
        stderr=subprocess.STDOUT,
        env=os.environ.copy(),
    )

    started = time.monotonic()
    try:
        wait_gateway()
        seed_ai_config("/opt/yak-agent/ai-config.yaml")
        run_id = str(uuid.uuid4())
        request_json("POST", "/session", {"run_id": run_id})

        # Open SSE stream BEFORE posting the run (required by gateway protocol).
        events, sse_thread = read_sse(run_id)

        # Step 1: Send a start-only event (no input payload) to initiate the
        # gRPC stream.  The gateway handler treats IsStart with no
        # FreeInput/IsInteractiveMessage as a "start-only" event and launches
        # the ReAct loop without returning early.
        start_only_payload = {
            "IsStart": True,
            "Params": {
                "CoordinatorId": run_id,
                "UserQuery": args.instruction,
                "AIService": args.service,
                "AIModelName": args.model,
                "UseDefaultAIConfig": False,
                "ReviewPolicy": "yolo",
                "DisallowRequireForUserPrompt": True,
                "AllowPlanUserInteract": False,
                "EnableAISearchInternet": False,
                "EnableSystemFileSystemOperator": True,
                "ReActMaxIteration": args.max_iterations,
                "AICallTokenLimit": args.token_limit,
                "Source": "harbor-benchmark-v1",
            },
        }
        request_json("POST", f"/run/{run_id}", start_only_payload)

        # Step 2: Send the FreeInput with the task instruction as a SEPARATE
        # request.  The gateway handler treats this as a non-start input event
        # and pushes it to the session's input channel, which the ReAct loop
        # picks up.
        input_payload = {
            "IsFreeInput": True,
            "FreeInput": args.instruction,
            "Params": {
                "CoordinatorId": run_id,
                "UserQuery": args.instruction,
            },
        }
        request_json("POST", f"/run/{run_id}", input_payload)

        final_text, terminal = wait_sse_completion(events, sse_thread)
        (logs / "final.txt").write_text(final_text)
        summary = summarize(events, time.monotonic() - started, final_text)
        (logs / "trajectory.jsonl").write_text(
            "\n".join(json.dumps(e, ensure_ascii=False) for e in events) + "\n"
        )
        (logs / "benchmark-summary.json").write_text(
            json.dumps(summary, indent=2, sort_keys=True) + "\n"
        )
        print(f"[react] terminal={terminal} summary={json.dumps(summary)}", flush=True)
        return 0 if terminal == "completed" else 1
    finally:
        process.send_signal(signal.SIGTERM)
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
        gateway_log.close()


# ---------------------------------------------------------------------------
# ForgeTask-mode runner (via StartAITask gRPC)
# ---------------------------------------------------------------------------

def run_forgetask(args: argparse.Namespace) -> int:
    """Execute a task via the Forge-based StartAITask gRPC directly.

    This mode requires a Forge definition file at
    ``/opt/yak-agent/benchmark-forge.yaml`` (uploaded by yak_agent.py).
    It calls ``yak tiered-ai-config`` to seed the profile DB, then invokes
    the forge via ``yak ai-task`` (a thin wrapper around StartAITask).
    """
    logs = Path("/logs/agent")
    logs.mkdir(parents=True, exist_ok=True)

    # Seed the tiered AI config into the profile DB BEFORE starting the gateway,
    # using the yak CLI which writes to the same DB that the gateway reads from.
    config_path = "/opt/yak-agent/ai-config.yaml"
    if not os.path.isfile(config_path):
        raise FileNotFoundError(f"ai-config.yaml not found at {config_path}")

    home = "/tmp/yak-agent-home"
    os.makedirs(home, exist_ok=True)

    print("[forgetask] seeding tiered-ai-config into profile DB", flush=True)
    result = subprocess.run(
        [
            "/usr/local/bin/yak", "tiered-ai-config",
            "--home", home,
            "--config-file", config_path,
            "--enable",
        ],
        capture_output=True,
        text=True,
        timeout=30,
    )
    if result.returncode != 0:
        print(f"[forgetask] tiered-ai-config stderr: {result.stderr}", flush=True)
        # Non-fatal: the gateway also seeds via HTTP API on startup.
    print(f"[forgetask] tiered-ai-config stdout: {result.stdout.strip()}", flush=True)

    gateway_log = (logs / "gateway.log").open("wb")
    process = subprocess.Popen(
        [
            "/usr/local/bin/yak",
            "ai-http-gateway",
            "--host", "127.0.0.1",
            "--port", "8089",
            "--home", home,
        ],
        stdout=gateway_log,
        stderr=subprocess.STDOUT,
        env=os.environ.copy(),
    )

    started = time.monotonic()
    try:
        wait_gateway()
        seed_ai_config(config_path)  # belt-and-suspenders via HTTP API

        run_id = str(uuid.uuid4())
        request_json("POST", "/session", {"run_id": run_id})

        events, sse_thread = read_sse(run_id)

        # ForgeTask start payload: uses ForgeName + ForgeParams instead of
        # FreeInput.  The Forge definition was uploaded alongside the runner.
        start_payload = {
            "IsStart": True,
            "IsFreeInput": False,
            "Params": {
                "CoordinatorId": run_id,
                "UserQuery": args.instruction,
                "ForgeName": "benchmark-task",
                "ForgeParams": {
                    "query": args.instruction,
                },
                "AIService": args.service,
                "AIModelName": args.model,
                "UseDefaultAIConfig": False,
                "ReviewPolicy": "yolo",
                "DisallowRequireForUserPrompt": True,
                "AllowPlanUserInteract": False,
                "EnableAISearchInternet": False,
                "EnableSystemFileSystemOperator": True,
                "ReActMaxIteration": args.max_iterations,
                "AICallTokenLimit": args.token_limit,
                "Source": "harbor-benchmark-v1",
            },
        }
        request_json("POST", f"/run/{run_id}", start_payload)

        final_text, terminal = wait_sse_completion(events, sse_thread)
        (logs / "final.txt").write_text(final_text)
        summary = summarize(events, time.monotonic() - started, final_text)
        (logs / "trajectory.jsonl").write_text(
            "\n".join(json.dumps(e, ensure_ascii=False) for e in events) + "\n"
        )
        (logs / "benchmark-summary.json").write_text(
            json.dumps(summary, indent=2, sort_keys=True) + "\n"
        )
        print(
            f"[forgetask] terminal={terminal} summary={json.dumps(summary)}", flush=True
        )
        return 0 if terminal == "completed" else 1
    finally:
        process.send_signal(signal.SIGTERM)
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
        gateway_log.close()


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def main() -> int:
    parser = argparse.ArgumentParser(
        description="Yak AI Agent benchmark runner (in-container)"
    )
    parser.add_argument("--instruction", required=True,
                        help="Task instruction to send to the agent")
    parser.add_argument("--service", required=True,
                        help="AI provider type (e.g. openai, deepseek)")
    parser.add_argument("--model", required=True,
                        help="Exact model identifier")
    parser.add_argument("--max-iterations", type=int, default=20,
                        help="ReAct max iterations (default: 20)")
    parser.add_argument("--token-limit", type=int, default=20000,
                        help="AI call token limit (default: 20000)")
    parser.add_argument(
        "--mode", choices=("react", "forgetask"), default="react",
        help="Execution mode: 'react' (HTTP gateway ReAct) or "
             "'forgetask' (StartAITask gRPC). Default: react.",
    )
    parser.add_argument(
        "--timeout", type=int, default=1800,
        help="Agent execution timeout in seconds (default: 1800)",
    )
    args = parser.parse_args()

    if args.mode == "forgetask":
        return run_forgetask(args)
    return run_react(args)


if __name__ == "__main__":
    raise SystemExit(main())

