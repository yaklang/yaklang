from __future__ import annotations

import asyncio
import json
import os
import shlex
import signal
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
import uuid
from pathlib import Path

from harbor.agents.base import BaseAgent
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext


GATEWAY_PORT = 18089  # fixed port so we don't collide with other services
GATEWAY_HOME = "/tmp/yak-benchmark-home"
GATEWAY_URL = f"http://127.0.0.1:{GATEWAY_PORT}/agent"
TERMINAL_TYPES = {"completed", "cancelled", "failed", "error", "done"}


class YakAgent(BaseAgent):
    """Run the Yak AI Agent on the **host** machine (not inside Docker).

    Only the challenge environment runs in Docker.  The yak binary and the
    HTTP gateway live on the host, giving the agent full access to the host
    filesystem, tools, and network — exactly like ``opencode`` does.

    Port mapping: if the task environment exposes a service on port 8080,
    Harbor maps it to a random host port.  We detect this mapping and rewrite
    ``127.0.0.1:8080`` in the instruction so the host-side agent can reach
    the Docker-side challenge server.
    """

    SUPPORTS_ATIF = False
    SUPPORTS_WINDOWS = False

    def __init__(self, *args, **kwargs) -> None:
        extra_env = kwargs.get("extra_env") or {}
        self._merged_env = {**os.environ, **extra_env}
        model_name = kwargs.get("model_name")

        super().__init__(*args, **kwargs)

        default_binary = Path(__file__).resolve().parents[1] / "bin" / "yak"
        self._binary_path = Path(
            self._merged_env.get("YAK_BINARY_PATH", str(default_binary))
        ).expanduser().resolve()

        service = self._merged_env.get("YAK_AI_SERVICE", "")
        model = self._merged_env.get("YAK_AI_MODEL", "")
        if model_name and "/" in model_name:
            p_service, p_model = model_name.split("/", 1)
            service = service or p_service
            model = model or p_model
        self._service = service or "openai"
        self._model = model
        self._max_iterations = int(self._merged_env.get("YAK_REACT_MAX_ITERATIONS", "40"))
        self._token_limit = int(self._merged_env.get("YAK_AI_TOKEN_LIMIT", "50000"))
        self._mode = self._merged_env.get("YAK_AGENT_MODE", "react")

        self._gateway_process: subprocess.Popen | None = None
        self._config_path: str | None = None

    @staticmethod
    def name() -> str:
        return "yak-agent"

    def version(self) -> str | None:
        return os.environ.get("YAK_AGENT_VERSION", "local")

    # ------------------------------------------------------------------
    # helpers
    # ------------------------------------------------------------------

    @staticmethod
    def _request_json(method: str, path: str, payload: dict | None = None,
                      timeout: float = 30.0) -> dict:
        data = None if payload is None else json.dumps(payload).encode()
        req = urllib.request.Request(
            GATEWAY_URL + path,
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
            raise RuntimeError(f"HTTP {exc.code} {method} {path}: {body[:300]}") from exc

    @staticmethod
    def _wait_gateway(timeout_sec: float = 30.0) -> None:
        deadline = time.monotonic() + timeout_sec
        while time.monotonic() < deadline:
            try:
                YakAgent._request_json("GET", "/setting")
                return
            except Exception:
                time.sleep(0.25)
        raise RuntimeError(f"Gateway did not become ready within {timeout_sec}s")

    def _seed_ai_config(self) -> None:
        """Seed the tiered AI config into the gateway via HTTP API."""
        if not self._config_path or not os.path.isfile(self._config_path):
            return

        # Parse the YAML config file (same format as before)
        ai_type = ai_key = ai_domain = ai_model = ""
        with open(self._config_path) as fh:
            for raw in fh:
                line = raw.rstrip("\n").strip()
                if not line or line.startswith("#"):
                    continue
                if line.startswith("- "):
                    key, _, val = line.lstrip("- ").partition(":")
                    key, val = key.strip(), val.strip().strip("\"'")
                    if key == "type":      ai_type = val
                    elif key == "api_key": ai_key = val
                    elif key == "domain":  ai_domain = val
                    elif key == "model":   ai_model = val

        if not ai_type or not ai_key or not ai_domain:
            print(f"[yak-agent] WARNING: ai-config.yaml incomplete, API calls may fail", flush=True)
            return

        # Step 1: simple setting (no API key — just service/model name)
        self._request_json("POST", "/setting", {
            "AIService": ai_type,
            "AIModelName": ai_model or ai_type,
            "UseDefaultAIConfig": False,
            "ReviewPolicy": "yolo",
            "DisableToolUse": False,
            "DisallowRequireForUserPrompt": True,
            "AllowPlanUserInteract": False,
            "EnableAISearchInternet": False,
            "EnableSystemFileSystemOperator": True,
        })

        # Step 2: full AIGlobalConfig with API credentials
        self._request_json("POST", "/setting/aiconfig", {
            "Enabled": True,
            "DisableFallback": True,
            "IntelligentModels": [{
                "Provider": {
                    "Type": ai_type,
                    "APIKey": ai_key,
                    "Domain": ai_domain,
                },
                "ModelName": ai_model or ai_type,
            }],
        })
        print(f"[yak-agent] AI config seeded: {ai_type} @ {ai_domain}", flush=True)

    @staticmethod
    def _find_host_port(environment: BaseEnvironment) -> int | None:
        """Try to discover the host port mapped to container port 8080."""
        # Harbor exposes container ports via the environment object.
        # DockerEnvironment has a `_ports` attribute or similar.
        for attr in ("_ports", "ports", "_host_ports", "_port_mappings"):
            val = getattr(environment, attr, None)
            if val and isinstance(val, dict):
                port = val.get(8080) or val.get("8080") or val.get("8080/tcp")
                if port:
                    return int(port)
        # Fallback: try environment-specific discovery
        if hasattr(environment, "port") and callable(getattr(environment, "port", None)):
            try:
                return int(environment.port(8080))
            except Exception:
                pass
        return None

    # ------------------------------------------------------------------
    # Harbor agent lifecycle
    # ------------------------------------------------------------------

    async def setup(self, environment: BaseEnvironment) -> None:
        if not self._binary_path.is_file():
            raise FileNotFoundError(f"yak binary not found: {self._binary_path}")
        if not self._model:
            raise ValueError("YAK_AI_MODEL must be set")

        config_path = Path(
            self._merged_env.get("YAK_AI_CONFIG_FILE", "")
        ).expanduser()
        self._config_path = str(config_path) if config_path.is_file() else None

        # Clean up any previous gateway home
        import shutil
        shutil.rmtree(GATEWAY_HOME, ignore_errors=True)
        os.makedirs(GATEWAY_HOME, exist_ok=True)

        # ------------------------------------------------------------------
        # Sync /app files from Docker → host via docker cp.
        # The agent runs on the host and needs native filesystem access to
        # task fixtures (incident.log, schema.json, etc.).
        # ------------------------------------------------------------------
        self._workspace = Path(GATEWAY_HOME) / "workspace"
        self._workspace.mkdir(parents=True, exist_ok=True)
        self._has_docker_files = False

        # Discover container name: try Harbor env attributes, then docker ps.
        container_name = (
            getattr(environment, "_container_name", None)
            or getattr(environment, "_project_name", None)
        )
        if container_name:
            # Compose project name → container is {project}-main-1
            container_name = f"{container_name}-main-1"

        if not container_name:
            # Fallback: scan docker ps for a container with "main-1" that
            # was recently created (Harbor trial containers).
            try:
                result = subprocess.run(
                    ["docker", "ps", "--format", "{{.Names}}",
                     "--filter", "name=main-1"],
                    capture_output=True, text=True, timeout=5,
                )
                names = [n.strip() for n in result.stdout.splitlines() if n.strip()]
                if names:
                    container_name = names[0]  # take the most recent
            except Exception:
                pass

        if container_name:
            try:
                result = subprocess.run(
                    ["docker", "cp", f"{container_name}:/app/.", str(self._workspace)],
                    capture_output=True, text=True, timeout=15,
                )
                if result.returncode == 0 and list(self._workspace.iterdir()):
                    self._has_docker_files = True
                    print(f"[yak-agent] synced /app from {container_name} → {self._workspace}",
                          flush=True)
            except Exception as exc:
                print(f"[yak-agent] docker cp failed: {exc}", flush=True)

        # Start yak ai-http-gateway on the HOST
        gateway_log_path = Path(GATEWAY_HOME) / "gateway.log"
        gateway_log = gateway_log_path.open("wb")
        self._gateway_process = subprocess.Popen(
            [
                str(self._binary_path),
                "ai-http-gateway",
                "--host", "127.0.0.1",
                "--port", str(GATEWAY_PORT),
                "--home", GATEWAY_HOME,
            ],
            stdout=gateway_log,
            stderr=subprocess.STDOUT,
            env={**os.environ, "YAKIT_HOME": GATEWAY_HOME},
        )

        # Wait for gateway to be ready
        try:
            self._wait_gateway(timeout_sec=45.0)
            print("[yak-agent] gateway started on host", flush=True)
        except RuntimeError:
            self._gateway_process.kill()
            raise

        # Seed AI config
        self._seed_ai_config()

    async def run(
        self,
        instruction: str,
        environment: BaseEnvironment,
        context: AgentContext,
    ) -> None:
        # --- Rewrite paths for host-side execution ---

        # Files: replace /app/ with the local workspace (synced via docker cp)
        if self._has_docker_files:
            ws = str(self._workspace)
            instruction = instruction.replace("/app/", f"{ws}/")
            instruction = instruction.replace("`/app/", f"`{ws}/")

        # Network: discover host port mapping for container port 8080
        host_port = self._find_host_port(environment)
        if host_port and host_port != 8080:
            instruction = instruction.replace("127.0.0.1:8080", f"127.0.0.1:{host_port}")
            instruction = instruction.replace("localhost:8080", f"127.0.0.1:{host_port}")
            print(f"[yak-agent] port mapping: container:8080 → host:{host_port}", flush=True)

        run_id = str(uuid.uuid4())
        self._request_json("POST", "/session", {"run_id": run_id})

        # Open SSE stream (background thread)
        events: list[dict] = []
        final_text: list[str] = []
        error_container: list[Exception] = []
        ready = threading.Event()

        def _sse_reader() -> None:
            try:
                req = urllib.request.Request(f"{GATEWAY_URL}/run/{run_id}/events")
                with urllib.request.urlopen(req, timeout=1800) as resp:
                    for raw_line in resp:
                        line = raw_line.decode("utf-8", errors="replace").strip()
                        if not line.startswith("data:"):
                            continue
                        event = json.loads(line[5:].strip())
                        events.append(event)
                        delta = event.get("StreamDelta") or ""
                        if delta:
                            final_text.append(delta)
                        if event.get("Type") == "listener_ready":
                            ready.set()
                        if event.get("Type") in TERMINAL_TYPES:
                            break
            except Exception as exc:
                error_container.append(exc)
                ready.set()

        sse_thread = threading.Thread(target=_sse_reader, daemon=True)
        sse_thread.start()

        if not ready.wait(timeout=60.0):
            raise RuntimeError("SSE listener_ready timeout")
        if error_container:
            raise RuntimeError(f"SSE error: {error_container[0]}") from error_container[0]

        # Step 1: start-only event (launches ReAct without FreeInput)
        self._request_json("POST", f"/run/{run_id}", {
            "IsStart": True,
            "Params": {
                "CoordinatorId": run_id,
                "UserQuery": instruction,
                "AIService": self._service,
                "AIModelName": self._model,
                "UseDefaultAIConfig": False,
                "ReviewPolicy": "yolo",
                "DisallowRequireForUserPrompt": True,
                "AllowPlanUserInteract": False,
                "EnableAISearchInternet": False,
                "EnableSystemFileSystemOperator": True,
                "ReActMaxIteration": self._max_iterations,
                "AICallTokenLimit": self._token_limit,
                "Source": "harbor-benchmark-v1",
            },
        })

        # Step 2: FreeInput with the task instruction
        self._request_json("POST", f"/run/{run_id}", {
            "IsFreeInput": True,
            "FreeInput": instruction,
            "Params": {
                "CoordinatorId": run_id,
                "UserQuery": instruction,
            },
        })

        # Wait for SSE completion
        sse_thread.join(timeout=1800)
        if sse_thread.is_alive():
            raise RuntimeError("SSE stream timed out")

        terminal_type = events[-1].get("Type", "missing") if events else "missing"
        print(f"[yak-agent] terminal={terminal_type}  events={len(events)}", flush=True)

        if terminal_type != "completed":
            raise RuntimeError(
                f"Agent terminated with status '{terminal_type}' "
                f"(expected 'completed')"
            )

    async def cleanup(self, environment: BaseEnvironment) -> None:
        if self._gateway_process:
            self._gateway_process.send_signal(signal.SIGTERM)
            try:
                self._gateway_process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self._gateway_process.kill()
            self._gateway_process = None

        import shutil
        shutil.rmtree(GATEWAY_HOME, ignore_errors=True)
