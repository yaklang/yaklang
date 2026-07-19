#!/usr/bin/env python3
"""Local smoke test for the Yak AI Agent HTTP gateway.

Starts a local ``yak ai-http-gateway`` process and validates the full
HTTP API flow::

    gateway health → session create → SSE open → run submit → events → complete

Does NOT require an AI provider or API key — it validates protocol
mechanics only.  The agent run will fail at the AI call stage (no
provider configured), but the gateway wiring is verified.

Prerequisites:
    A ``yak`` binary must be available at the path given by
    ``YAK_BINARY_PATH`` (default: ``../../bin/yak``).

Usage::

    YAK_BINARY_PATH=/path/to/yak python3 benchmarks/harbor/scripts/smoke_gateway_test.py
"""
from __future__ import annotations

import argparse
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


BASE_URL = "http://127.0.0.1:18089/agent"
GATEWAY_PORT = 18089
GATEWAY_HOME = "/tmp/yak-smoke-gateway-home"

TERMINAL_TYPES = {"completed", "cancelled", "failed", "error", "done"}


def _red(text: str) -> str:
    return f"\033[31m{text}\033[0m"


def _green(text: str) -> str:
    return f"\033[32m{text}\033[0m"


def _bold(text: str) -> str:
    return f"\033[1m{text}\033[0m"


def request_json(
    method: str, path: str, payload: dict | None = None, timeout: float = 10.0
) -> dict:
    data = None if payload is None else json.dumps(payload).encode()
    req = urllib.request.Request(
        BASE_URL + path,
        data=data,
        method=method,
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = resp.read()
        return json.loads(body) if body else {}
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        return {"_error": exc.code, "_body": body[:500]}


def wait_gateway(timeout_sec: float = 30.0) -> None:
    deadline = time.monotonic() + timeout_sec
    while time.monotonic() < deadline:
        try:
            request_json("GET", "/setting")
            return
        except Exception:
            time.sleep(0.2)
    raise RuntimeError(f"Gateway did not become ready within {timeout_sec}s")


def run_tests(yak_binary: Path) -> dict[str, bool]:
    """Run the gateway smoke tests. Returns {test_name: passed}."""
    results: dict[str, bool] = {}

    # Clean up from any previous run
    if os.path.exists(GATEWAY_HOME):
        import shutil
        shutil.rmtree(GATEWAY_HOME, ignore_errors=True)
    os.makedirs(GATEWAY_HOME, exist_ok=True)

    # Start gateway
    gateway_log_path = Path("/tmp/yak-smoke-gateway.log")
    gateway_log = gateway_log_path.open("wb")
    process = subprocess.Popen(
        [
            str(yak_binary),
            "ai-http-gateway",
            "--host", "127.0.0.1",
            "--port", str(GATEWAY_PORT),
            "--home", GATEWAY_HOME,
        ],
        stdout=gateway_log,
        stderr=subprocess.STDOUT,
        env={**os.environ, "YAKIT_HOME": GATEWAY_HOME},
    )

    try:
        # ------------------------------------------------------------------
        # Test 1: Gateway starts and responds
        # ------------------------------------------------------------------
        print(f"  {_bold('Test 1:')} Gateway health...", end=" ", flush=True)
        try:
            wait_gateway(timeout_sec=30.0)
            print(_green("OK"))
            results["gateway_startup"] = True
        except RuntimeError as exc:
            print(_red(f"FAIL ({exc})"))
            results["gateway_startup"] = False
            return results

        # ------------------------------------------------------------------
        # Test 2: GET /setting returns valid JSON
        # ------------------------------------------------------------------
        print(f"  {_bold('Test 2:')} GET /setting...", end=" ", flush=True)
        try:
            setting = request_json("GET", "/setting")
            assert isinstance(setting, dict), f"expected dict, got {type(setting)}"
            assert "ReviewPolicy" in setting or "review_policy" in setting, \
                "ReviewPolicy not in response"
            print(_green("OK"))
            results["get_setting"] = True
        except Exception as exc:
            print(_red(f"FAIL ({exc})"))
            results["get_setting"] = False

        # ------------------------------------------------------------------
        # Test 3: POST /session creates a session
        # ------------------------------------------------------------------
        print(f"  {_bold('Test 3:')} POST /session...", end=" ", flush=True)
        run_id = str(uuid.uuid4())
        try:
            session_resp = request_json("POST", "/session", {"run_id": run_id})
            assert session_resp.get("run_id") == run_id, \
                f"run_id mismatch: {session_resp.get('run_id')} != {run_id}"
            print(_green("OK"))
            results["create_session"] = True
        except Exception as exc:
            print(_red(f"FAIL ({exc})"))
            results["create_session"] = False
            run_id = None

        if run_id is None:
            return results

        # ------------------------------------------------------------------
        # Test 4: GET /session/all lists sessions
        # ------------------------------------------------------------------
        print(f"  {_bold('Test 4:')} GET /session/all...", end=" ", flush=True)
        try:
            sessions_resp = request_json("GET", "/session/all")
            sessions = sessions_resp.get("sessions", [])
            found = any(s.get("run_id") == run_id for s in sessions)
            assert found, f"session {run_id} not in session list"
            print(_green("OK"))
            results["list_sessions"] = True
        except Exception as exc:
            print(_red(f"FAIL ({exc})"))
            results["list_sessions"] = False

        # ------------------------------------------------------------------
        # Test 5: SSE stream opens and receives listener_ready
        # ------------------------------------------------------------------
        print(f"  {_bold('Test 5:')} SSE listener_ready...", end=" ", flush=True)
        sse_events: list[dict] = []
        sse_ready = threading.Event()
        sse_error: Exception | None = None

        def _sse_reader() -> None:
            nonlocal sse_error
            try:
                req = urllib.request.Request(f"{BASE_URL}/run/{run_id}/events")
                with urllib.request.urlopen(req, timeout=30) as resp:
                    for raw_line in resp:
                        line = raw_line.decode("utf-8", errors="replace").strip()
                        if not line.startswith("data:"):
                            continue
                        event = json.loads(line[5:].strip())
                        sse_events.append(event)
                        if event.get("Type") == "listener_ready":
                            sse_ready.set()
                        if event.get("Type") in TERMINAL_TYPES:
                            break
            except Exception as exc:
                sse_error = exc
                sse_ready.set()

        sse_thread = threading.Thread(target=_sse_reader, daemon=True)
        sse_thread.start()

        if sse_ready.wait(timeout=15.0):
            if sse_error:
                print(_red(f"FAIL (SSE error: {sse_error})"))
                results["sse_listener_ready"] = False
            else:
                print(_green("OK"))
                results["sse_listener_ready"] = True
        else:
            print(_red("FAIL (timeout)"))
            results["sse_listener_ready"] = False

        # ------------------------------------------------------------------
        # Test 6: POST /run triggers the ReAct loop and receives events
        # ------------------------------------------------------------------
        print(f"  {_bold('Test 6:')} POST /run event flow...", end=" ", flush=True)
        try:
            start_payload = {
                "IsStart": True,
                "IsFreeInput": True,
                "FreeInput": "Say hello and write the result to /tmp/smoke-test.txt",
                "Params": {
                    "CoordinatorId": run_id,
                    "UserQuery": "Say hello",
                    "AIService": "openai",
                    "AIModelName": "gpt-test",
                    "UseDefaultAIConfig": False,
                    "ReviewPolicy": "yolo",
                    "DisallowRequireForUserPrompt": True,
                    "AllowPlanUserInteract": False,
                    "EnableAISearchInternet": False,
                    "ReActMaxIteration": 3,
                    "AICallTokenLimit": 1000,
                    "Source": "smoke-gateway-test",
                },
            }
            run_resp = request_json("POST", f"/run/{run_id}", start_payload)
            assert "_error" not in run_resp, \
                f"POST /run failed: {run_resp.get('_body', run_resp.get('_error', ''))}"
            print(_green("OK"))
            results["post_run"] = True
        except Exception as exc:
            print(_red(f"FAIL ({exc})"))
            results["post_run"] = False

        # ------------------------------------------------------------------
        # Test 7: Terminal event received
        # ------------------------------------------------------------------
        print(f"  {_bold('Test 7:')} Terminal event...", end=" ", flush=True)
        sse_thread.join(timeout=30.0)
        if sse_events:
            terminal = sse_events[-1].get("Type", "missing")
            # Without a real AI provider, expect "failed" or "error"
            if terminal in TERMINAL_TYPES:
                print(_green(f"OK (terminal={terminal})"))
                results["terminal_event"] = True
            else:
                print(_red(f"FAIL (last event type={terminal}, not terminal)"))
                results["terminal_event"] = False
        else:
            print(_red("FAIL (no SSE events received)"))
            results["terminal_event"] = False

    finally:
        process.send_signal(signal.SIGTERM)
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
        gateway_log.close()
        # Clean up
        import shutil
        shutil.rmtree(GATEWAY_HOME, ignore_errors=True)
        gateway_log_path.unlink(missing_ok=True)

    return results


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Smoke test the Yak AI Agent HTTP gateway (no AI provider needed)"
    )
    parser.add_argument(
        "--yak-binary",
        type=Path,
        default=os.environ.get(
            "YAK_BINARY_PATH",
            str(Path(__file__).resolve().parents[1] / "bin" / "yak"),
        ),
        help="Path to the yak binary (default: $YAK_BINARY_PATH or ../../bin/yak)",
    )
    args = parser.parse_args()

    yak_binary = args.yak_binary.expanduser().resolve()
    if not yak_binary.is_file():
        print(
            f"{_red('ERROR:')} yak binary not found at {yak_binary}",
            file=sys.stderr,
        )
        print(
            "Build it or set YAK_BINARY_PATH to the correct location.",
            file=sys.stderr,
        )
        return 1

    print(f"Using yak binary: {yak_binary}")
    print(f"Gateway port: {GATEWAY_PORT}")
    print()

    results = run_tests(yak_binary)

    print()
    passed = sum(1 for v in results.values() if v)
    total = len(results)
    if passed == total:
        print(f"{_green(f'All {total} gateway smoke tests passed')}")
        return 0
    else:
        failed = total - passed
        print(f"{_red(f'{failed}/{total} gateway smoke tests failed')}")
        for name, ok in results.items():
            status = _green("PASS") if ok else _red("FAIL")
            print(f"  {status}: {name}")
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
