#!/usr/bin/env python3
"""Local benchmark runner for Yak Agent and OpenCode — no Harbor, no Docker.

Runs the four ``yak-agent-v1`` tasks directly on the host:

* The Yak AI Agent is driven through the local ``yak ai-http-gateway``
  (same HTTP/SSE flow the Harbor ``YakAgent`` uses), seeded with the
  credentials in ``ai-config.yaml``.
* OpenCode is driven through the local ``opencode run --format=json`` binary
  (Mach-O on macOS is fine — nothing is uploaded into a container).

Each task's challenge server (if any) is started as a plain ``python3``
subprocess on a free host port; the audit-log path and port are redirected
via ``CHALLENGE_AUDIT_LOG`` / ``CHALLENGE_PORT`` (see the patched
``server.py``).  The verifier is run with ``/app/``, ``/logs/verifier/`` and
``/var/log/challenge-audit.jsonl`` rewritten to local temp paths so scoring
works unchanged.

Output: one JSONL record per (task, attempt) with the same fields
``harbor_results_to_jsonl.py`` emits (``task``, ``attempt``, ``reward``,
``duration_sec``, ``tool_event_count``, ``model_event_count``, ``label`` …),
so the result file feeds directly into ``compare_results.py``.

Usage examples::

    # run one task with the local yak engine
    python3 benchmarks/harbor/scripts/run_local.py yak \\
        --task direct-incident-summary --label base

    # run the same task with local opencode
    python3 benchmarks/harbor/scripts/run_local.py opencode \\
        --task direct-incident-summary --label opencode

    # compare two previously-produced JSONL files
    python3 benchmarks/harbor/scripts/run_local.py compare base.jsonl opencode.jsonl

See ``run_local.sh`` for the paired base-vs-candidate convenience wrapper.
"""
from __future__ import annotations

# ---------------------------------------------------------------------------
# idna codec shim — must run BEFORE any socket/urllib import.
#
# Some local Python 3.14 builds (python.org installer on macOS) ship a
# unicodedata.cpython-314-darwin.so whose code signature is rejected by
# Hardened Runtime / AMFI ("library load denied by system policy"). That
# breaks encodings.idna (-> stringprep -> unicodedata), which in turn makes
# socket.getaddrinfo() crash with LookupError: unknown encoding: idna — even
# for a pure numeric IP like 127.0.0.1. Every server task and every health
# check depends on getaddrinfo, so without this shim server tasks cannot
# start on such hosts.
#
# We register a minimal passthrough idna codec. Benchmark traffic only ever
# resolves 127.0.0.1 / localhost, never a real international domain, so an
# ASCII passthrough is correct for this use case and is a no-op on hosts
# where the real idna codec already works (we register only as a fallback).
import codecs as _codecs


def _install_idna_shim() -> None:
    # Probe the native idna path (-> stringprep -> unicodedata). If the
    # unicodedata C extension loads, the real idna codec is fine and we do
    # nothing. If it fails to import (the macOS code-signing case), we
    # register a passthrough shim so socket.getaddrinfo works for 127.0.0.1.
    try:
        import unicodedata  # noqa: F401
        try:
            _codecs.lookup("idna")
            return  # native idna works
        except LookupError:
            pass
    except ImportError:
        pass

    def _encode(input, errors="strict"):
        if isinstance(input, str):
            return (input.encode("ascii", errors), len(input))
        return (input, len(input))

    def _decode(input, errors="strict"):
        if isinstance(input, (bytes, bytearray)):
            return (input.decode("ascii", errors), len(input))
        return (input, len(input))

    info = _codecs.CodecInfo(
        name="idna",
        encode=_encode,
        decode=_decode,
        incrementalencoder=_codecs.IncrementalEncoder,
        incrementaldecoder=_codecs.IncrementalDecoder,
        streamwriter=_codecs.StreamWriter,
        streamreader=_codecs.StreamReader,
    )
    _codecs.register(lambda name: info if name == "idna" else None)


_install_idna_shim()


import argparse
import json
import os
import re
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.request
import uuid
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[3]
DATASET = REPO_ROOT / "benchmarks" / "harbor" / "datasets" / "yak-agent-v1"
DEFAULT_CONFIG = REPO_ROOT / "benchmarks" / "harbor" / "ai-config.yaml"
DEFAULT_OUTPUT = REPO_ROOT / "benchmarks" / "harbor" / "results" / "local"
GRPC_STUBS = REPO_ROOT / "benchmarks" / "harbor" / "agents" / "_grpc_stubs"

GATEWAY_PORT = 18089
GRPC_PORT = 18087
TERMINAL_TYPES = {"completed", "cancelled", "failed", "error", "done"}


def _ensure_idna_sitecustomize() -> Path | None:
    """Write a sitecustomize.py that installs the idna shim, return its dir.

    Used to propagate the shim into the server.py subprocess (which is a
    fresh python invocation and does not import this module). Returns None
    when the real idna codec is healthy, in which case no shim is needed.

    Health check looks at the underlying ``unicodedata`` C extension (the
    real dependency of ``encodings.idna`` via ``stringprep``), not at
    ``codecs.lookup("idna")`` — because this module has already registered
    a shim by the time this is called, which would mask the real breakage.
    """
    try:
        import unicodedata  # noqa: F401
        return None  # native idna path works
    except ImportError:
        pass
    shim_dir = REPO_ROOT / "benchmarks" / "harbor" / "scripts" / "_idna_shim"
    shim_dir.mkdir(parents=True, exist_ok=True)
    (shim_dir / "sitecustomize.py").write_text(
        '"""Auto-loaded idna codec shim for benchmark server subprocesses."""\n'
        "import codecs as _c\n"
        "try:\n"
        '    _c.lookup("idna")\n'
        "except LookupError:\n"
        "    def _e(i, errors='strict'):\n"
        "        return ((i.encode('ascii', errors) if isinstance(i, str) else i), len(i))\n"
        "    def _d(i, errors='strict'):\n"
        "        return ((i.decode('ascii', errors) if isinstance(i, (bytes, bytearray)) else i), len(i))\n"
        "    _c.register(lambda n: _c.CodecInfo(name='idna', encode=_e, decode=_d,"
        " incrementalencoder=_c.IncrementalEncoder, incrementaldecoder=_c.IncrementalDecoder,"
        " streamwriter=_c.StreamWriter, streamreader=_c.StreamReader) if n == 'idna' else None)\n"
    )
    return shim_dir


# The yak ReAct engine signals task completion via these event patterns:
#   - Type=="result" with Content containing "finished":true  (final answer,
#     emitted by yak >= c556de7f2 / "improve lean execution")
#   - structured event with Content containing "LOOP_STALL_DETECTED"
#     (old yak versions that don't emit "result" — the engine itself
#     notices the loop has stalled after the task is done)
#   - structured event with "react_task_now_status":"completed" or
#     "status":"completed" inside an "execution" block (old yak marks
#     completion internally but doesn't surface it as a top-level event)
# Neither old nor new yak sets Type to "completed" as a top-level event.
REACT_TERMINAL_TYPES = TERMINAL_TYPES | {"result"}
# Structured-event Content substrings that signal completion in old yak:
OLD_YAK_COMPLETION_MARKERS = [
    "LOOP_STALL_DETECTED",
    '"react_task_now_status":"completed"',
    '"react_task_now_status": "completed"',
]


# Whether a task ships a challenge HTTP server is detected from the task
# directory layout (see ``is_server_task`` below): any task with
# ``environment/challenge/server.py`` is treated as a server task. This is
# intentionally NOT a hardcoded allowlist so that newly-added benchmark tasks
# are picked up automatically — adding a server task should require only
# creating the server.py file, never editing this runner.



# ---------------------------------------------------------------------------
# Small helpers
# ---------------------------------------------------------------------------

def _red(s: str) -> str: return f"\033[31m{s}\033[0m"
def _green(s: str) -> str: return f"\033[32m{s}\033[0m"
def _bold(s: str) -> str: return f"\033[1m{s}\033[0m"
def _dim(s: str) -> str: return f"\033[2m{s}\033[0m"


def free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def task_dir(task: str) -> Path:
    p = DATASET / task
    if not p.is_dir():
        raise SystemExit(f"unknown task: {task} (not in {DATASET})")
    return p


def is_server_task(task: str) -> bool:
    """True iff the task ships a challenge HTTP server.

    Detected from the presence of ``environment/challenge/server.py`` in the
    task directory. New server tasks are picked up automatically — no
    allowlist edit needed.
    """
    return (task_dir(task) / "environment" / "challenge" / "server.py").is_file()



def list_tasks() -> list[str]:
    return sorted(p.name for p in DATASET.iterdir() if (p / "task.toml").is_file())


# ---------------------------------------------------------------------------
# AI config parsing (minimal YAML — same as gateway_runner.py)
# ---------------------------------------------------------------------------

def parse_ai_config(path: Path) -> dict[str, str]:
    """Parse the tiered-ai-config YAML.

    Returns a dict with the intelligent-tier fields
    (type/api_key/domain/model) used as the main agent model. The optional
    lightweight tier (memfit-light-free built-in) is read separately by
    ``parse_lightweight_config``.
    """
    fields = {"type": "", "api_key": "", "domain": "", "model": ""}
    section = None
    entry_taken = False
    for raw in path.read_text().splitlines():
        s = raw.strip()
        if not s or s.startswith("#"):
            continue
        # track which section we are in
        if s == "intelligent_configs:":
            section = "intelligent"; entry_taken = False; continue
        if s == "lightweight_configs:":
            section = "lightweight"; entry_taken = False; continue
        if s == "vision_configs:":
            section = "vision"; entry_taken = False; continue
        # only parse the FIRST entry of the intelligent section
        if section == "intelligent":
            if s.startswith("- type:"):
                entry_taken = True
            if not entry_taken or ":" not in s:
                continue
            key, _, val = s.lstrip("- ").partition(":")
            key, val = key.strip(), val.strip().strip("\"'")
            if key in fields:
                fields[key] = val
    missing = [k for k, v in fields.items() if not v]
    if missing:
        raise SystemExit(
            f"ai-config.yaml missing fields {missing}; regenerate with "
            f"gen_ai_config_yaml.py (set YAK_AI_TYPE etc.)"
        )
    return fields


def parse_lightweight_config(path: Path) -> dict[str, str] | None:
    """Parse the optional lightweight_configs tier.

    Used for intent/perception sub-agents. If absent, seed_config falls back
    to reusing the intelligent-tier model.
    """
    if not path.is_file():
        return None
    fields: dict[str, str] = {}
    section = None
    entry_taken = False
    for raw in path.read_text().splitlines():
        s = raw.strip()
        if s == "lightweight_configs:":
            section = "lightweight"; entry_taken = False; continue
        if s and not s.startswith("-") and s.endswith(":") and section == "lightweight":
            section = None  # left the lightweight block
            continue
        if section == "lightweight":
            if s.startswith("- "):
                entry_taken = True
            if not entry_taken or ":" not in s:
                continue
            key, _, val = s.lstrip("- ").partition(":")
            fields[key.strip()] = val.strip().strip("\"'")
    if not fields.get("type") or not fields.get("model"):
        return None
    return fields


# ---------------------------------------------------------------------------
# Per-task environment: workdir, server, instruction rewrite, verifier
# ---------------------------------------------------------------------------

class TaskEnv:
    """Prepares a local sandbox for one task run: copied /app files, an
    optional challenge server on a free port, and the rewritten instruction.
    """

    def __init__(self, task: str, run_id: str):
        self.task = task
        self.run_id = run_id
        # Short FIXED path under /tmp. The yak agent's write_file/read_file
        # tools use literal absolute host paths, and models sometimes drop the
        # leading "/" on long random paths — a short deterministic path is
        # easier for the model to reproduce exactly. We assume one run at a
        # time per task (the runner serializes task execution).
        self.root = Path("/tmp/yakbench")
        if self.root.exists():
            shutil.rmtree(self.root)
        self.app_dir = self.root / "app"           # stands in for container /app
        self.logs_dir = self.root / "logs"         # stands in for /logs/verifier
        self.audit_log = self.root / "audit.jsonl"
        self.app_dir.mkdir(parents=True, exist_ok=True)
        (self.logs_dir / "verifier").mkdir(parents=True, exist_ok=True)
        self.server_proc: subprocess.Popen | None = None
        self.server_port: int | None = None

    # -- setup --------------------------------------------------------------
    def setup(self) -> None:
        tdir = task_dir(self.task)
        # copy /app fixtures (incident.log, schema.json, START_HERE.md, ...)
        src_app = tdir / "environment" / "app"
        if src_app.is_dir():
            for f in src_app.iterdir():
                if f.is_file():
                    shutil.copy2(f, self.app_dir / f.name)

        # NOTE: instruction rewriting happens in finalize_instruction() AFTER
        # start_server(), because the server port is only known then.
        self._raw_instruction = (tdir / "instruction.md").read_text()
        self.instruction = self._raw_instruction  # placeholder

    def finalize_instruction(self) -> None:
        """Rewrite the instruction once the server port is known. Must be
        called after start_server() for server-based tasks."""
        self.instruction = self._rewrite_instruction(self._raw_instruction)

    def _rewrite_instruction(self, text: str) -> str:
        # Server tasks: rewrite 127.0.0.1:8080 → actual free port.
        if is_server_task(self.task):
            if self.server_port and self.server_port != 8080:
                text = text.replace("127.0.0.1:8080", f"127.0.0.1:{self.server_port}")
                text = text.replace("localhost:8080", f"127.0.0.1:{self.server_port}")
        # IMPORTANT: do NOT blindly replace "/app/" in the body. The yak
        # agent's write_file tool uses literal absolute host paths (no chroot),
        # and models tend to ignore long /var/folders paths. Instead we keep
        # the original instruction readable and append an explicit "local
        # environment" note telling the agent the exact host paths to use.
        text = text.rstrip()
        if not text.endswith("\n"):
            text += "\n"
        text += (
            "\n---\n"
            "## Local environment (host paths — use these EXACT absolute paths)\n\n"
            f"- Input files are under: `{self.app_dir}/`\n"
            f"- You MUST write every output file (e.g. result.json) to the "
            f"`{self.app_dir}/` directory using its absolute path, e.g. "
            f"`{self.app_dir}/result.json`. Do NOT write to `/app/` or any "
            f"other location — use the full absolute path shown here.\n"
            f"- When reading inputs, use the absolute path, e.g. "
            f"`{self.app_dir}/incident.log`.\n"
        )
        return text

    # -- challenge server ---------------------------------------------------
    def start_server(self) -> None:
        server_py = task_dir(self.task) / "environment" / "challenge" / "server.py"
        if not server_py.is_file():
            return  # not a server task — nothing to start
        self.server_port = free_port()
        self.server_port = free_port()
        env = {
            **os.environ,
            "CHALLENGE_AUDIT_LOG": str(self.audit_log),
            "CHALLENGE_PORT": str(self.server_port),
        }
        # Inherit the idna shim into the server subprocess (see
        # _install_idna_shim above). PYTHONPATH is extended with the shim
        # dir and PYTHONNOUSERSITE is left untouched so sitecustomize loads.
        shim_dir = _ensure_idna_sitecustomize()
        if shim_dir is not None:
            existing = env.get("PYTHONPATH", "")
            env["PYTHONPATH"] = (
                f"{shim_dir}{os.pathsep}{existing}" if existing else str(shim_dir)
            )
        log = (self.root / "server.log").open("wb")
        self.server_proc = subprocess.Popen(
            [sys.executable, str(server_py)],
            stdout=log, stderr=subprocess.STDOUT, env=env,
        )
        # wait for health
        deadline = time.monotonic() + 15.0
        while time.monotonic() < deadline:
            if self.server_proc.poll() is not None:
                raise SystemExit(
                    f"{self.task}: server exited early "
                    f"(see {self.root / 'server.log'})"
                )
            try:
                with urllib.request.urlopen(
                    f"http://127.0.0.1:{self.server_port}/health", timeout=1
                ) as r:
                    if r.status == 200:
                        return
            except Exception:
                time.sleep(0.2)
        raise SystemExit(f"{self.task}: server did not become healthy")

    # -- verifier -----------------------------------------------------------
    def run_verifier(self) -> dict:
        """Run tests/verify.py with its hardcoded paths redirected locally.

        We do a simple source rewrite of /app/, /logs/verifier/ and the
        audit-log path, then exec it in a fresh namespace so the scoring
        logic is untouched.
        """
        verify_py = task_dir(self.task) / "tests" / "verify.py"
        src = verify_py.read_text()
        src = src.replace("/app/", f"{self.app_dir}/")
        src = src.replace("/logs/verifier/", f"{self.logs_dir}/verifier/")
        src = src.replace(
            "/var/log/challenge-audit.jsonl", str(self.audit_log)
        )
        ns: dict = {"__name__": "__verify__", "__file__": str(verify_py)}
        try:
            exec(compile(src, str(verify_py), "exec"), ns)
        except Exception as exc:  # verifier bug → treat as zero reward
            print(f"  {_red('verifier error')}: {exc}", file=sys.stderr)
            return {"outcome": 0.0, "evidence": 0.0, "format": 0.0, "reward": 0.0}
        reward_path = self.logs_dir / "verifier" / "reward.json"
        if reward_path.is_file():
            return json.loads(reward_path.read_text())
        return {"outcome": 0.0, "evidence": 0.0, "format": 0.0, "reward": 0.0}

    # -- cleanup ------------------------------------------------------------
    def teardown(self) -> None:
        if self.server_proc and self.server_proc.poll() is None:
            self.server_proc.send_signal(signal.SIGTERM)
            try:
                self.server_proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.server_proc.kill()
        # keep the sandbox on disk for debugging; printed at the end.


# ---------------------------------------------------------------------------
# Yak agent runner (local gateway + SSE)
# ---------------------------------------------------------------------------

class GatewayClient:
    """Thin REST/SSE client for the local ``yak ai-http-gateway``."""


# ---------------------------------------------------------------------------
# gRPC backend (no HTTP layer)
# ---------------------------------------------------------------------------

def _load_grpc_stubs():
    """Import the generated yakgrpc stubs lazily (only when backend=grpc).

    Returns the (pb, pb_grpc, grpc) triple. Raises SystemExit with a helpful
    message if the stubs aren't generated or grpcio isn't installed.
    """
    if not (GRPC_STUBS / "yakgrpc_pb2.py").is_file():
        raise SystemExit(
            f"gRPC stubs not found at {GRPC_STUBS}\n"
            f"generate them with:\n"
            f"  python3 -m grpc_tools.protoc -Icommon/yakgrpc "
            f"--python_out={GRPC_STUBS} --grpc_python_out={GRPC_STUBS} "
            f"common/yakgrpc/yakgrpc.proto"
        )
    import sys as _sys
    if str(GRPC_STUBS) not in _sys.path:
        _sys.path.insert(0, str(GRPC_STUBS))
    try:
        import grpc as _grpc
        import yakgrpc_pb2 as _pb
        import yakgrpc_pb2_grpc as _pb_grpc
    except ImportError as exc:
        raise SystemExit(
            f"missing Python gRPC deps ({exc}); install with:\n"
            f"  pip3 install grpcio grpcio-tools protobuf"
        ) from exc
    return _pb, _pb_grpc, _grpc


class GrpcClient:
    """Drives the yak AI agent via raw gRPC (``yak grpc`` + StartAIReAct).

    Skips the HTTP gateway entirely. Config is seeded via SetAIGlobalConfig,
    the agent runs via the StartAIReAct bidi stream, and the full trace can
    be pulled afterwards with ExportAILogs.
    """

    def __init__(self, host: str, port: int):
        pb, pb_grpc, grpc = _load_grpc_stubs()
        self._pb = pb
        self._grpc = grpc
        self._channel = grpc.insecure_channel(
            f"{host}:{port}",
            options=[
                ("grpc.max_receive_message_length", 100 * 1024 * 1024),
                ("grpc.max_send_message_length", 100 * 1024 * 1024),
            ],
        )
        self._stub = pb_grpc.YakStub(self._channel)
        self._host = host
        self._port = port

    def close(self) -> None:
        self._channel.close()

    def wait_ready(self, timeout_sec: float = 45.0) -> None:
        try:
            self._grpc.channel_ready_future(self._channel).result(timeout=timeout_sec)
        except self._grpc.FutureTimeoutError as exc:
            raise RuntimeError(
                f"gRPC server not ready at {self._host}:{self._port} "
                f"within {timeout_sec}s"
            ) from exc

    def seed_config(self, cfg: dict[str, str],
                    lightweight: dict[str, str] | None = None) -> None:
        """Seed the tiered AI config.

        ``cfg`` is the intelligent-tier (main agent) model. ``lightweight`` is
        the optional lightweight-tier model used by intent/perception
        sub-agents — defaults to the built-in free ``memfit-light-free``
        (type=aibalance) to match the Memfit UI default. If absent, the main
        model is reused for all tiers.
        """
        intelligent = self._pb.AIModelConfig(
            Provider=self._pb.ThirdPartyApplicationConfig(
                Type=cfg["type"],
                APIKey=cfg["api_key"],
                Domain=cfg["domain"],
            ),
            ModelName=cfg["model"],
        )
        # lightweight tier: prefer the configured free model, else reuse main
        if lightweight:
            light = self._pb.AIModelConfig(
                Provider=self._pb.ThirdPartyApplicationConfig(
                    Type=lightweight["type"],
                    APIKey=lightweight.get("api_key", "any"),
                ),
                ModelName=lightweight["model"],
            )
        else:
            light = intelligent
        agc = self._pb.AIGlobalConfig(
            Enabled=True,
            DisableFallback=True,
            IntelligentModels=[intelligent],
            LightweightModels=[light],
            VisionModels=[light],
        )
        self._stub.SetAIGlobalConfig(agc)

    def run_instruction(self, instruction: str, model: str, service: str,
                        max_iter: int, token_limit: int,
                        timeout_sec: float) -> tuple[list[dict], str, float]:
        """Run one instruction. Returns (events, terminal_type, duration_sec).

        Events are converted to plain dicts so downstream code
        (summarize_events) is identical for the HTTP and gRPC backends.
        """
        import queue
        import threading

        run_id = str(uuid.uuid4())
        inputs: queue.Queue = queue.Queue()
        inputs.put(self._pb.AIInputEvent(
            IsStart=True,
            Params=self._pb.AIStartParams(
                CoordinatorId=run_id,
                UserQuery=instruction,
                AIService=service,
                AIModelName=model,
                UseDefaultAIConfig=False,
                ReviewPolicy="yolo",
                DisallowRequireForUserPrompt=True,
                AllowPlanUserInteract=False,
                EnableAISearchInternet=False,
                EnableSystemFileSystemOperator=True,
                ReActMaxIteration=max_iter,
                AICallTokenLimit=token_limit,
                TimelineSessionID=run_id,
                Source="local-bench-grpc",
            ),
        ))
        inputs.put(self._pb.AIInputEvent(
            IsFreeInput=True,
            FreeInput=instruction,
        ))

        def _input_iter():
            while True:
                ev = inputs.get()
                if ev is None:
                    return
                yield ev

        stream = self._stub.StartAIReAct(_input_iter())
        events: list[dict] = []
        started = time.monotonic()
        terminal = "missing"
        for ev in stream:
            if time.monotonic() - started > timeout_sec:
                terminal = "timeout"
                break
            ev_dict = _grpc_event_to_dict(ev)
            events.append(ev_dict)
            if _is_react_terminal(ev_dict):
                terminal = ev_dict.get("Type", "completed")
                self._drain_tail(stream, events, 2.0)
                break
        inputs.put(None)
        return events, terminal, time.monotonic() - started

    def _drain_tail(self, stream, events: list[dict], max_wait: float) -> None:
        deadline = time.monotonic() + max_wait
        while time.monotonic() < deadline:
            try:
                events.append(_grpc_event_to_dict(next(stream)))
            except StopIteration:
                break

    def export_trace(self, session_id: str, output_path: Path) -> str | None:
        """Export the full trace for a run as a ZIP. Returns the file path."""
        try:
            resp = self._stub.ExportAILogs(self._pb.ExportAILogsRequest(
                SessionID=session_id,
                ExportDataTypes=["output_event", "checkpoints", "timeline", "memory"],
                OutputPath=str(output_path),
            ))
            return resp.FilePath or None
        except self._grpc.RpcError as exc:
            print(f"  {_red('trace export failed')}: {exc.code()}", file=sys.stderr)
            return None


def _grpc_event_to_dict(ev) -> dict:
    """Convert a gRPC AIOutputEvent into the same dict shape the HTTP/SSE
    path produces, so summarize_events works unchanged."""
    import base64
    d: dict = {
        "Type": ev.Type or "",
        "IsStream": bool(ev.IsStream),
        "IsReason": bool(ev.IsReason),
        "Content": "",
        "StreamDelta": "",
    }
    if ev.Content:
        try:
            d["Content"] = base64.b64encode(ev.Content).decode()
        except Exception:
            d["Content"] = ev.Content.decode("utf-8", errors="replace")
    if ev.StreamDelta:
        d["StreamDelta"] = ev.StreamDelta.decode("utf-8", errors="replace")
    if ev.CallToolID:
        d["CallToolID"] = ev.CallToolID
    return d


def _is_react_terminal(ev_dict: dict) -> bool:
    """Return True if this event signals the ReAct loop is finished.

    Supports both new and old yak engine versions:

    **New yak (c556de7f2+):**
      - Type=="result" with Content containing "finished":true

    **Old yak (pre-c556de7f2):**
      - structured event with Content containing "LOOP_STALL_DETECTED"
        (the engine itself notices the loop has stalled after completion)
      - structured event with "react_task_now_status":"completed" or
        "status":"completed" inside an "execution" block
    """
    t = ev_dict.get("Type", "")
    content = _decode_event_content(ev_dict.get("Content"))

    # new yak: explicit result event
    if t == "result":
        return '"finished":true' in content or '"finished": true' in content

    # old yak compat: check structured events for completion markers
    if t == "structured" and content:
        for marker in OLD_YAK_COMPLETION_MARKERS:
            if marker in content:
                return True

    # explicit terminal types
    if t in TERMINAL_TYPES:
        return True

    return False


class GatewayClient:
    """Thin REST/SSE client for the local ``yak ai-http-gateway``."""

    def __init__(self, port: int):
        self.base = f"http://127.0.0.1:{port}/agent"

    def _req(self, method: str, path: str, payload: dict | None = None,
             timeout: float = 30.0) -> dict:
        data = None if payload is None else json.dumps(payload).encode()
        req = urllib.request.Request(
            self.base + path, data=data, method=method,
            headers={"Content-Type": "application/json"},
        )
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                body = r.read()
            return json.loads(body) if body else {}
        except urllib.error.HTTPError as exc:
            body = exc.read().decode("utf-8", errors="replace")
            raise RuntimeError(f"HTTP {exc.code} {method} {path}: {body[:300]}") from exc

    def wait_ready(self, timeout_sec: float = 45.0) -> None:
        deadline = time.monotonic() + timeout_sec
        while time.monotonic() < deadline:
            try:
                self._req("GET", "/setting", timeout=2)
                return
            except Exception:
                time.sleep(0.25)
        raise RuntimeError(f"gateway not ready within {timeout_sec}s")

    def seed_config(self, cfg: dict[str, str],
                    lightweight: dict[str, str] | None = None) -> None:
        # 1) simple setting first (so applySettingToRuntime sees the model)
        self._req("POST", "/setting", {
            "AIService": cfg["type"],
            "AIModelName": cfg["model"],
            "UseDefaultAIConfig": False,
            "ReviewPolicy": "yolo",
            "DisableToolUse": False,
            "DisallowRequireForUserPrompt": True,
            "AllowPlanUserInteract": False,
            "EnableAISearchInternet": False,
            "EnableSystemFileSystemOperator": True,
        })
        # 2) full AIGlobalConfig with credentials (overwrites tiered config)
        light_entry = {
            "Provider": {
                "Type": lightweight["type"],
                "APIKey": lightweight.get("api_key", "any"),
            },
            "ModelName": lightweight["model"],
        } if lightweight else {
            "Provider": {
                "Type": cfg["type"],
                "APIKey": cfg["api_key"],
                "Domain": cfg["domain"],
            },
            "ModelName": cfg["model"],
        }
        self._req("POST", "/setting/aiconfig", {
            "Enabled": True,
            "DisableFallback": True,
            "IntelligentModels": [{
                "Provider": {
                    "Type": cfg["type"],
                    "APIKey": cfg["api_key"],
                    "Domain": cfg["domain"],
                },
                "ModelName": cfg["model"],
            }],
            "LightweightModels": [light_entry],
            "VisionModels": [light_entry],
        })

    def run_instruction(self, instruction: str, model: str, service: str,
                        max_iter: int, token_limit: int,
                        timeout_sec: float) -> tuple[list[dict], str, float]:
        """Returns (events, terminal_type, duration_sec)."""
        run_id = str(uuid.uuid4())
        self._req("POST", "/session", {"run_id": run_id})

        events: list[dict] = []
        ready = threading.Event()
        err: list[Exception] = []

        def _sse() -> None:
            try:
                req = urllib.request.Request(f"{self.base}/run/{run_id}/events")
                with urllib.request.urlopen(req, timeout=timeout_sec) as resp:
                    for raw in resp:
                        line = raw.decode("utf-8", errors="replace").strip()
                        if not line.startswith("data:"):
                            continue
                        ev = json.loads(line[5:].strip())
                        events.append(ev)
                        if ev.get("Type") == "listener_ready":
                            ready.set()
                        if ev.get("Type") in TERMINAL_TYPES:
                            break
            except Exception as exc:
                err.append(exc)
                ready.set()

        t = threading.Thread(target=_sse, daemon=True)
        t.start()
        if not ready.wait(timeout=60.0):
            raise RuntimeError("SSE listener_ready timeout")
        if err:
            raise RuntimeError(f"SSE error: {err[0]}") from err[0]

        started = time.monotonic()
        # start-only event launches the ReAct loop
        self._req("POST", f"/run/{run_id}", {
            "IsStart": True,
            "Params": {
                "CoordinatorId": run_id,
                "UserQuery": instruction,
                "AIService": service,
                "AIModelName": model,
                "UseDefaultAIConfig": False,
                "ReviewPolicy": "yolo",
                "DisallowRequireForUserPrompt": True,
                "AllowPlanUserInteract": False,
                "EnableAISearchInternet": False,
                "EnableSystemFileSystemOperator": True,
                "ReActMaxIteration": max_iter,
                "AICallTokenLimit": token_limit,
                "Source": "local-benchmark",
            },
        })
        # free-input carries the actual instruction
        self._req("POST", f"/run/{run_id}", {
            "IsFreeInput": True,
            "FreeInput": instruction,
            "Params": {"CoordinatorId": run_id, "UserQuery": instruction},
        })
        t.join(timeout=timeout_sec)
        duration = time.monotonic() - started
        terminal = events[-1].get("Type", "missing") if events else "missing"
        if t.is_alive():
            terminal = "timeout"
        return events, terminal, duration


def _decode_event_content(content) -> str:
    """Best-effort decode of an event Content field to a UTF-8 string.

    Handles: already-decoded str, base64-encoded str, raw bytes. Both the
    HTTP/SSE backend (which stores base64 text) and the gRPC backend (which
    stores raw bytes → base64) converge here.
    """
    import base64
    if not content:
        return ""
    if isinstance(content, bytes):
        return content.decode("utf-8", errors="replace")
    if not isinstance(content, str):
        return str(content)
    # try base64 first (yak events are often base64-wrapped JSON)
    try:
        decoded = base64.b64decode(content, validate=True)
        if decoded:
            text = decoded.decode("utf-8", errors="replace")
            # sanity-check: if it looks like JSON or readable text, use it
            if text.strip().startswith(("{", "[")) or text.isprintable():
                return text
    except Exception:
        pass
    return content


def summarize_events(events: list[dict]) -> dict:
    """Build compact efficiency metrics from the yak event stream.

    IMPORTANT counting semantics (must match opencode's _summarize_opencode
    so the two are directly comparable):
      - model_event_count = number of distinct model invocations, NOT the
        number of stream/structured events (one invocation fans out into
        hundreds of stream-token events). We count ``ai_call_summary``
        (preferred) or ``stream_start`` (fallback) — one per LLM call.
      - tool_event_count = number of actual tool calls completed, counted
        from ``tool_call_done`` (preferred) or ``tool_call_status`` with
        status=="done" (fallback).
    """
    # Count model invocations: ai_call_summary is emitted once per LLM call.
    # stream_start is the per-stream fallback (also one per call).
    model_calls = 0
    seen_call_ids: set[str] = set()
    ai_call_summaries = 0
    stream_starts = 0
    tool_calls_done = 0
    tool_status_done_ids: set[str] = set()
    last_consumption: dict = {}
    for ev in events:
        t = str(ev.get("Type", "")).lower()
        if t == "ai_call_summary":
            ai_call_summaries += 1
        elif t == "stream_start":
            stream_starts += 1
        elif t == "tool_call_done":
            tool_calls_done += 1
        elif t == "tool_call_status":
            # count a tool as done when its status flips to "done"
            content = _decode_event_content(ev.get("Content"))
            if '"status":"done"' in content or '"status": "done"' in content:
                # extract call_tool_id to dedup
                call_id = ""
                for tok in ('"call_tool_id":"', '"call_tool_id": "'):
                    idx = content.find(tok)
                    if idx >= 0:
                        call_id = content[idx+len(tok):].split('"', 1)[0]
                        break
                if call_id and call_id not in tool_status_done_ids:
                    tool_status_done_ids.add(call_id)
        elif t == "consumption":
            content = _decode_event_content(ev.get("Content"))
            if content:
                try:
                    last_consumption = json.loads(content)
                except (json.JSONDecodeError, TypeError):
                    pass
    model_events = ai_call_summaries or stream_starts
    tool_events = tool_calls_done or len(tool_status_done_ids)
    out = {"tool_event_count": tool_events, "model_event_count": model_events}
    if last_consumption:
        out["token"] = {
            "input": last_consumption.get("input_consumption", 0),
            "output": last_consumption.get("output_consumption", 0),
            "cache_hit": last_consumption.get("cache_hit_token", 0),
        }
    return out


def run_yak(args: argparse.Namespace, cfg: dict[str, str],
            lightweight: dict[str, str] | None = None) -> list[dict]:
    binary = Path(args.yak_binary).expanduser().resolve()
    if not binary.is_file():
        raise SystemExit(f"yak binary not found: {binary}")

    backend = getattr(args, "backend", "grpc")
    # IMPORTANT: use the default yak home (~/yakit-projects) unless overridden.
    # A fresh empty home has an empty ai_yak_tools table, so the agent would be
    # missing all yak-script tools (do_http_request, write_file, ...). The
    # default home is initialized once by the yak binary and carries the full
    # tool set. See README "Local run" for details.
    home_override = getattr(args, "yak_home", None)
    if home_override:
        home = Path(home_override).expanduser()
        home.mkdir(parents=True, exist_ok=True)
        home_arg = ["--home", str(home)]
        env_home = {"YAKIT_HOME": str(home)}
    else:
        home = None
        home_arg = []  # let yak use its default (~/yakit-projects)
        env_home = {}

    # server log goes to a temp file (never the home dir)
    log_dir = Path(tempfile.mkdtemp(prefix="yak-bench-log-"))
    server_log_path = log_dir / f"{backend}.log"
    server_log = server_log_path.open("wb")
    if backend == "grpc":
        port = getattr(args, "grpc_port", GRPC_PORT)
        cmd = [str(binary), "grpc",
               "--host", "127.0.0.1", "--port", str(port)] + home_arg
        ready_marker = "yak grpc ok"
    else:  # http
        port = GATEWAY_PORT
        cmd = [str(binary), "ai-http-gateway",
               "--host", "127.0.0.1", "--port", str(port)] + home_arg
        ready_marker = None  # polled via HTTP health

    proc = subprocess.Popen(
        cmd, stdout=server_log, stderr=subprocess.STDOUT,
        env={**os.environ, **env_home},
    )
    records: list[dict] = []
    try:
        # wait for the server to come up
        if ready_marker:
            _wait_for_log_marker(server_log, server_log_path, ready_marker, 60.0)
        if backend == "grpc":
            client = GrpcClient("127.0.0.1", port)
        else:
            client = GatewayClient(port)
        client.wait_ready()
        client.seed_config(cfg, lightweight=lightweight)
        home_desc = str(home) if home else "default (~/yakit-projects)"
        print(f"[yak:{backend}] up @ 127.0.0.1:{port}, model={cfg['model']}, "
              f"home={home_desc}", flush=True)

        for task in args.tasks:
            for attempt in range(1, args.attempts + 1):
                rec = _run_one_yak_task(
                    client, task, attempt, args, cfg, label=args.label
                )
                records.append(rec)
        if hasattr(client, "close"):
            client.close()
    finally:
        proc.send_signal(signal.SIGTERM)
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            proc.kill()
        server_log.close()
        shutil.rmtree(log_dir, ignore_errors=True)  # only the log dir, never home
    return records


def _wait_for_log_marker(file_obj, log_path: Path, marker: str,
                         timeout_sec: float) -> None:
    """Poll a server's stdout log file for a readiness marker string."""
    deadline = time.monotonic() + timeout_sec
    file_obj.flush()
    while time.monotonic() < deadline:
        try:
            text = log_path.read_text(errors="replace")
            if marker in text:
                return
        except Exception:
            pass
        time.sleep(0.3)
    raise RuntimeError(
        f"server did not signal '{marker}' within {timeout_sec}s "
        f"(see {log_path})"
    )


def _run_one_yak_task(client, task: str, attempt: int,
                      args: argparse.Namespace, cfg: dict[str, str],
                      label: str) -> dict:
    run_id = str(uuid.uuid4())[:8]
    env = TaskEnv(task, run_id)
    env.setup()
    env.start_server()
    env.finalize_instruction()  # rewrite with the now-known server port
    started = time.monotonic()
    terminal = "missing"
    event_stats: dict = {}
    error: str | None = None
    try:
        backend_tag = getattr(args, "backend", "grpc")
        print(f"\n=== {_bold('yak:'+backend_tag)} {task} attempt {attempt} "
              f"(port={env.server_port or '-'}) ===", flush=True)
        events, terminal, agent_dur = client.run_instruction(
            env.instruction, cfg["model"], cfg["type"],
            max_iter=args.max_iterations, token_limit=args.token_limit,
            timeout_sec=args.timeout,
        )
        event_stats = summarize_events(events)
        # dump trajectory for debugging (kept under sandbox/logs)
        (env.logs_dir / "trajectory.jsonl").write_text(
            "\n".join(json.dumps(e, ensure_ascii=False) for e in events) + "\n"
        )
        # gRPC backend can also export the full server-side trace as a zip
        if hasattr(client, "export_trace"):
            trace_zip = env.logs_dir / "trace.zip"
            path = client.export_trace(run_id, trace_zip)
            if path:
                print(f"  {_dim('trace exported → ' + path)}", flush=True)
        print(f"  terminal={terminal}  events={len(events)}  "
              f"agent_dur={agent_dur:.1f}s", flush=True)
    except Exception as exc:
        error = f"{type(exc).__name__}: {exc}"
        print(f"  {_red('agent error')}: {error}", flush=True)

    duration = round(time.monotonic() - started, 3)
    scores = env.run_verifier()
    env.teardown()

    rec = _build_record(
        task, attempt, label, scores, duration, event_stats,
        terminal=terminal, error=error, sandbox=str(env.root),
    )
    _print_score(rec)
    return rec


# ---------------------------------------------------------------------------
# OpenCode runner
# ---------------------------------------------------------------------------

def run_opencode(args: argparse.Namespace, cfg: dict[str, str]) -> list[dict]:
    binary = Path(args.opencode_binary).expanduser().resolve()
    if not binary.is_file():
        raise SystemExit(f"opencode binary not found: {binary}")
    # opencode wants provider/model; reuse the AI config values so the model
    # matches what Yak used.
    model = f"{cfg['type']}/{cfg['model']}"

    records: list[dict] = []
    for task in args.tasks:
        for attempt in range(1, args.attempts + 1):
            rec = _run_one_opencode_task(
                binary, model, task, attempt, args, cfg, label=args.label
            )
            records.append(rec)
    return records


def _run_one_opencode_task(binary: Path, model: str, task: str, attempt: int,
                           args: argparse.Namespace, cfg: dict[str, str],
                           label: str) -> dict:
    run_id = str(uuid.uuid4())[:8]
    env = TaskEnv(task, run_id)
    env.setup()
    env.start_server()
    env.finalize_instruction()  # rewrite with the now-known server port
    started = time.monotonic()
    error: str | None = None
    event_stats: dict = {}
    rc = -1
    try:
        print(f"\n=== {_bold('opencode')} {task} attempt {attempt} "
              f"(port={env.server_port or '-'}) ===", flush=True)

        # write a minimal opencode.json so the local Mach-O binary picks up
        # the same provider/key as Yak.
        oc_config = env.root / "opencode.json"
        base_url = f"https://{cfg['domain']}/v1" if cfg["type"] == "openai" \
            else f"https://{cfg['domain']}"
        oc_config.write_text(json.dumps({
            "$schema": "https://opencode.ai/config.json",
            "autoupdate": False,
            "share": "disabled",
            "provider": {
                cfg["type"]: {
                    "models": {cfg["model"]: {"name": cfg["model"]}},
                    "options": {"apiKey": cfg["api_key"], "baseURL": base_url},
                }
            },
        }, indent=2))
        oc_config.chmod(0o600)

        oc_env = {
            **os.environ,
            "HOME": str(env.root),
            "XDG_CONFIG_HOME": str(env.root / ".config"),
            "OPENCODE_CONFIG": str(oc_config),
            "OPENCODE_DISABLE_AUTOUPDATE": "true",
        }
        cmd = [
            str(binary), "run", "--format=json", "--auto",
            "--model", model, "--dir", str(env.app_dir),
            "--", env.instruction,
        ]
        proc = subprocess.Popen(
            cmd, cwd=str(env.app_dir), env=oc_env,
            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
            bufsize=1,
        )
        assert proc.stdout is not None
        events: list[dict] = []
        deadline = time.monotonic() + args.timeout
        for line in proc.stdout:
            sys.stdout.write(line)
            sys.stdout.flush()
            try:
                ev = json.loads(line)
                if isinstance(ev, dict):
                    events.append(ev)
            except json.JSONDecodeError:
                pass
            if time.monotonic() > deadline:
                proc.terminate()
                break
        try:
            proc.wait(timeout=10)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait()
        rc = proc.returncode
        event_stats = _summarize_opencode(events)
        print(f"  rc={rc}  events={len(events)}", flush=True)
    except Exception as exc:
        error = f"{type(exc).__name__}: {exc}"
        print(f"  {_red('opencode error')}: {error}", flush=True)

    duration = round(time.monotonic() - started, 3)
    scores = env.run_verifier()
    env.teardown()

    rec = _build_record(
        task, attempt, label, scores, duration, event_stats,
        terminal="completed" if rc == 0 else f"rc={rc}", error=error,
        sandbox=str(env.root),
    )
    _print_score(rec)
    return rec


def _summarize_opencode(events: list[dict]) -> dict:
    finishes = [e for e in events if e.get("type") == "step_finish"]
    tool_events = sum(1 for e in events if e.get("type") == "tool_use")
    token = {"input": 0, "output": 0, "cache_hit": 0}
    for e in finishes:
        part = e.get("part") or {}
        tk = part.get("tokens") or {}
        cache = tk.get("cache") or {}
        token["input"] += int(tk.get("input") or 0)
        token["output"] += int(tk.get("output") or 0)
        token["cache_hit"] += int(cache.get("read") or 0)
    return {
        "tool_event_count": tool_events,
        "model_event_count": len(finishes),
        "token": token,
    }


# ---------------------------------------------------------------------------
# Shared record building + output
# ---------------------------------------------------------------------------

def _build_record(task: str, attempt: int, label: str, scores: dict,
                  duration: float, event_stats: dict, *,
                  terminal: str, error: str | None, sandbox: str) -> dict:
    rec: dict = {
        "task": task,
        "attempt": attempt,
        "reward": round(float(scores.get("reward", 0.0)), 4),
        "outcome": round(float(scores.get("outcome", 0.0)), 4),
        "evidence": round(float(scores.get("evidence", 0.0)), 4),
        "format": round(float(scores.get("format", 0.0)), 4),
        "duration_sec": duration,
        "errored": error is not None,
        "terminal": terminal,
        "label": label,
        "sandbox": sandbox,
    }
    for k in ("tool_event_count", "model_event_count"):
        if k in event_stats:
            rec[k] = int(event_stats[k])
    tok = event_stats.get("token")
    if isinstance(tok, dict):
        rec["input_tokens"] = int(tok.get("input", 0))
        rec["output_tokens"] = int(tok.get("output", 0))
        rec["cache_hit_tokens"] = int(tok.get("cache_hit", 0))
    if error:
        rec["error"] = error
    return rec


def _print_score(rec: dict) -> None:
    color = _green if rec["reward"] >= 0.9 else (
        _red if rec["reward"] < 0.5 else _dim
    )
    print(f"  {color('reward=' + str(rec['reward']))}  "
          f"o={rec['outcome']} e={rec['evidence']} f={rec['format']}  "
          f"dur={rec['duration_sec']}s  "
          f"tools={rec.get('tool_event_count', '-')} "
          f"model={rec.get('model_event_count', '-')}", flush=True)


def write_jsonl(records: list[dict], path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w") as fh:
        for r in records:
            fh.write(json.dumps(r, sort_keys=True) + "\n")
    print(f"\n{_green('wrote')} {len(records)} records → {path}", flush=True)


# ---------------------------------------------------------------------------
# Compare subcommand (thin wrapper around compare_results.py)
# ---------------------------------------------------------------------------

def run_compare(args: argparse.Namespace) -> int:
    script = REPO_ROOT / "benchmarks" / "harbor" / "scripts" / "compare_results.py"
    cmd = [sys.executable, str(script), args.base, args.candidate]
    if args.output:
        cmd += ["--output", str(args.output)]
    return subprocess.run(cmd).returncode


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def _add_run_args(p: argparse.ArgumentParser) -> None:
    p.add_argument("--task", action="append", dest="tasks", default=None,
                   help="task name (repeatable); default: all four tasks")
    p.add_argument("--attempts", type=int, default=1,
                   help="repeats per task (default: 1)")
    p.add_argument("--label", default="local",
                   help="label stamped on each JSONL record")
    p.add_argument("--output", type=Path,
                   default=DEFAULT_OUTPUT / "local-results.jsonl",
                   help="output JSONL path")
    p.add_argument("--config", type=Path, default=DEFAULT_CONFIG,
                   help="tiered-ai-config YAML (default: ai-config.yaml)")
    p.add_argument("--timeout", type=int, default=900,
                   help="per-run timeout in seconds (default: 900)")


def main() -> int:
    parser = argparse.ArgumentParser(
        prog="run_local.py",
        description="Run yak-agent-v1 tasks locally (no Harbor, no Docker).",
    )
    sub = parser.add_subparsers(dest="cmd", required=True)

    # yak
    p_yak = sub.add_parser("yak", help="run tasks via local yak (gRPC or HTTP)")
    _add_run_args(p_yak)
    p_yak.add_argument("--yak-binary", default=os.environ.get(
        "YAK_BINARY_PATH", "/usr/local/bin/yak"))
    p_yak.add_argument("--max-iterations", type=int, default=40)
    p_yak.add_argument("--token-limit", type=int, default=50000)
    p_yak.add_argument("--backend", choices=("grpc", "http"), default="grpc",
                       help="yak backend: 'grpc' (raw gRPC, no HTTP gateway, "
                            "default) or 'http' (ai-http-gateway + SSE)")
    p_yak.add_argument("--grpc-port", type=int, default=GRPC_PORT,
                       help=f"gRPC server port (default {GRPC_PORT})")
    p_yak.add_argument("--yak-home", default=None,
                       help="yak home dir (default: ~/yakit-projects, which "
                            "has the full tool set; override only if you know "
                            "the home DB has been initialized)")

    # opencode
    p_oc = sub.add_parser("opencode", help="run tasks via local opencode")
    _add_run_args(p_oc)
    p_oc.add_argument("--opencode-binary", default=os.environ.get(
        "OPENCODE_BINARY_PATH", str(Path.home() / ".opencode" / "bin" / "opencode")))

    # compare
    p_cmp = sub.add_parser("compare", help="compare two JSONL result files")
    p_cmp.add_argument("base", type=Path)
    p_cmp.add_argument("candidate", type=Path)
    p_cmp.add_argument("--output", type=Path, default=None)

    args = parser.parse_args()

    if args.cmd == "compare":
        return run_compare(args)

    # shared run setup
    if not args.tasks:
        args.tasks = list_tasks()
        print(f"{_dim('no --task given, running all: ' + ', '.join(args.tasks))}")
    # validate task names early
    for t in args.tasks:
        task_dir(t)

    cfg = parse_ai_config(args.config)
    # optional lightweight tier (defaults to the built-in free memfit-light-free
    # to match the Memfit UI; falls back to the main model if not configured)
    lightweight = parse_lightweight_config(args.config)

    if args.cmd == "yak":
        records = run_yak(args, cfg, lightweight=lightweight)
    elif args.cmd == "opencode":
        records = run_opencode(args, cfg)
    else:  # pragma: no cover — argparse enforces
        parser.error(f"unknown command {args.cmd}")

    write_jsonl(records, args.output)

    # summary line
    rewards = [r["reward"] for r in records]
    mean = sum(rewards) / len(rewards) if rewards else 0.0
    print(f"\n{_bold('summary')}  label={args.label}  "
          f"runs={len(records)}  mean_reward={mean:.4f}", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
