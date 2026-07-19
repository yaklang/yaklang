#!/usr/bin/env python3
"""Run OpenCode non-interactively and emit benchmark-compatible artifacts."""

from __future__ import annotations

import argparse
import json
import os
import queue
import subprocess
import sys
import threading
import time
from pathlib import Path


def scalar(value: str) -> str:
    value = value.strip()
    if value.startswith('"'):
        try:
            parsed = json.loads(value)
            return parsed if isinstance(parsed, str) else value
        except json.JSONDecodeError:
            return value.strip('"')
    return value.strip("'")


def load_config(path: Path) -> dict[str, str]:
    """Read the first intelligent_configs entry from Yak's minimal YAML."""
    fields = {"type": "", "api_key": "", "domain": "", "model": ""}
    in_entry = False
    for raw in path.read_text().splitlines():
        stripped = raw.strip()
        if stripped.startswith("- type:"):
            fields["type"] = scalar(stripped.partition(":")[2])
            in_entry = True
            continue
        if not in_entry or ":" not in stripped:
            continue
        key, _, value = stripped.partition(":")
        if key in fields:
            fields[key] = scalar(value)

    missing = [key for key, value in fields.items() if not value]
    if missing:
        raise RuntimeError(
            f"AI config is missing required fields: {', '.join(missing)}"
        )
    return fields


def base_url(provider: str, domain: str) -> str:
    url = domain.rstrip("/")
    if not url.startswith(("http://", "https://")):
        url = "https://" + url
    if provider == "openai" and url.count("/") == 2:
        url += "/v1"
    return url


def write_config(path: Path, model: str, source: Path | None) -> None:
    provider, model_id = model.split("/", 1)
    entry: dict = {"models": {model_id: {"name": model_id}}}

    if source is not None:
        cfg = load_config(source)
        if cfg["type"] != provider or cfg["model"] != model_id:
            raise RuntimeError(
                "OpenCode model does not match AI config: "
                f"{model!r} != {cfg['type']}/{cfg['model']}"
            )
        entry["options"] = {
            "apiKey": cfg["api_key"],
            "baseURL": base_url(provider, cfg["domain"]),
        }

    config = {
        "$schema": "https://opencode.ai/config.json",
        "autoupdate": False,
        "share": "disabled",
        "provider": {provider: entry},
    }
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(config, indent=2) + "\n")
    path.chmod(0o600)


def summarize(events: list[dict], duration: float | None = None) -> dict:
    text: list[str] = []
    final = ""
    for event in events:
        if event.get("type") == "step_start":
            text = []
        elif event.get("type") == "text":
            text.append(str((event.get("part") or {}).get("text") or ""))
        elif event.get("type") == "step_finish" and text:
            final = "\n".join(text)
    finishes = [
        event.get("part") or {}
        for event in events
        if event.get("type") == "step_finish"
    ]
    token = {
        "input": 0,
        "output": 0,
        "cache_hit": 0,
        "cache_write": 0,
        "reasoning": 0,
    }
    for part in finishes:
        tokens = part.get("tokens") or {}
        cache = tokens.get("cache") or {}
        token["input"] += int(tokens.get("input") or 0)
        token["output"] += int(tokens.get("output") or 0)
        token["cache_hit"] += int(cache.get("read") or 0)
        token["cache_write"] += int(cache.get("write") or 0)
        token["reasoning"] += int(tokens.get("reasoning") or 0)

    if duration is None:
        timestamps = [
            float(event["timestamp"])
            for event in events
            if isinstance(event.get("timestamp"), (int, float))
        ]
        duration = (
            (max(timestamps) - min(timestamps)) / 1000
            if len(timestamps) > 1
            else 0.0
        )

    return {
        "agent": "opencode",
        "duration_sec": round(duration, 3),
        "event_count": len(events),
        "model_event_count": len(finishes),
        "tool_event_count": sum(
            event.get("type") == "tool_use" for event in events
        ),
        "final_text_chars": len(final),
        "token": token,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--instruction", required=True)
    parser.add_argument("--model", required=True)
    parser.add_argument("--config", type=Path)
    parser.add_argument("--variant", default="")
    parser.add_argument("--timeout", type=int, default=900)
    args = parser.parse_args()

    if "/" not in args.model:
        parser.error("--model must use provider/model format")

    logs = Path("/logs/agent")
    logs.mkdir(parents=True, exist_ok=True)
    home = Path("/tmp/opencode-home")
    config = home / ".config" / "opencode" / "opencode.json"
    write_config(config, args.model, args.config)

    env = {
        **os.environ,
        "HOME": str(home),
        "XDG_CONFIG_HOME": str(home / ".config"),
        "XDG_DATA_HOME": str(home / ".local" / "share"),
        "XDG_CACHE_HOME": str(home / ".cache"),
        "OPENCODE_CONFIG": str(config),
        "OPENCODE_DISABLE_AUTOUPDATE": "true",
        "OPENCODE_DISABLE_MODELS_FETCH": "true",
        "OPENCODE_FAKE_VCS": "git",
    }
    command = [
        "/usr/local/bin/opencode",
        "run",
        "--format=json",
        "--thinking",
        "--dangerously-skip-permissions",
        "--model",
        args.model,
        "--dir",
        "/app",
    ]
    if args.variant:
        command.extend(["--variant", args.variant])
    command.extend(["--", args.instruction])

    started = time.monotonic()
    events: list[dict] = []
    output = logs / "opencode.txt"
    with output.open("w") as stream:
        process = subprocess.Popen(
            command,
            cwd="/app",
            env=env,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            bufsize=1,
        )
        assert process.stdout is not None
        lines: queue.Queue[str | None] = queue.Queue()

        def read() -> None:
            for line in process.stdout:
                lines.put(line)
            lines.put(None)

        thread = threading.Thread(target=read, daemon=True)
        thread.start()
        deadline = time.monotonic() + args.timeout
        timed_out = False
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                timed_out = True
                break
            try:
                line = lines.get(timeout=min(0.25, remaining))
            except queue.Empty:
                if process.poll() is not None and not thread.is_alive():
                    break
                continue
            if line is None:
                break
            stream.write(line)
            stream.flush()
            sys.stdout.write(line)
            sys.stdout.flush()
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            if isinstance(event, dict):
                events.append(event)

        if timed_out:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
            code = 124
        else:
            code = process.wait()

        while not lines.empty():
            line = lines.get_nowait()
            if line is None:
                continue
            stream.write(line)
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            if isinstance(event, dict):
                events.append(event)

    final = ""
    current: list[str] = []
    for event in events:
        if event.get("type") == "step_start":
            current = []
        elif event.get("type") == "text":
            current.append(str((event.get("part") or {}).get("text") or ""))
        elif event.get("type") == "step_finish" and current:
            final = "\n".join(current)
    (logs / "final.txt").write_text(final)
    (logs / "benchmark-summary.json").write_text(
        json.dumps(summarize(events, time.monotonic() - started), indent=2) + "\n"
    )
    return code


if __name__ == "__main__":
    raise SystemExit(main())
