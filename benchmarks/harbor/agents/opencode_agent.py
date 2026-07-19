from __future__ import annotations

import json
import os
import shlex
from pathlib import Path

from harbor.agents.installed.base import with_prompt_template
from harbor.agents.installed.opencode import OpenCode
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext

from .opencode_runner import summarize


class OpenCodeAgent(OpenCode):
    """Run a caller-supplied Linux OpenCode binary in a Harbor task container.

    Harbor's built-in OpenCode adapter installs OpenCode from npm for every
    task. This adapter uploads an already-built binary instead, which makes the
    evaluated OpenCode version explicit and keeps setup independent of npm.
    """

    def __init__(self, *args, **kwargs) -> None:
        extra = kwargs.get("extra_env") or {}
        self._env = {**os.environ, **extra}
        super().__init__(*args, **kwargs)

        default = Path(__file__).resolve().parents[1] / "bin" / "opencode"
        self._binary = Path(
            self._env.get("OPENCODE_BINARY_PATH", str(default))
        ).expanduser().resolve()
        config = self._env.get("OPENCODE_AI_CONFIG_FILE", "")
        self._config = Path(config).expanduser() if config else None
        self._timeout = int(self._env.get("OPENCODE_AGENT_TIMEOUT", "900"))
        self._variant = self._env.get("OPENCODE_VARIANT", "")

    async def install(self, environment: BaseEnvironment) -> None:
        if not self._binary.is_file():
            raise FileNotFoundError(
                "OPENCODE_BINARY_PATH must point to a Linux OpenCode executable"
            )
        if self._binary.read_bytes()[:4] != b"\x7fELF":
            raise ValueError(
                "OPENCODE_BINARY_PATH must be an ELF Linux binary, not a "
                "macOS or Windows executable"
            )

        runner = Path(__file__).with_name("opencode_runner.py")
        result = await environment.exec(
            "mkdir -p /opt/opencode-agent /logs/agent /tmp/opencode-home",
            user="root",
        )
        if result.return_code != 0:
            raise RuntimeError(
                result.stderr or result.stdout or "OpenCode directory setup failed"
            )

        await environment.upload_file(self._binary, "/usr/local/bin/opencode")
        await environment.upload_file(runner, "/opt/opencode-agent/runner.py")
        if self._config is not None and self._config.is_file():
            await environment.upload_file(
                self._config, "/opt/opencode-agent/ai-config.yaml"
            )

        result = await environment.exec(
            "chmod 0755 /usr/local/bin/opencode",
            user="root",
        )
        if result.return_code != 0:
            raise RuntimeError(
                result.stderr or result.stdout or "OpenCode install failed"
            )

    def get_version_command(self) -> str | None:
        return "/usr/local/bin/opencode --version"

    @with_prompt_template
    async def run(
        self,
        instruction: str,
        environment: BaseEnvironment,
        context: AgentContext,
    ) -> None:
        if not self.model_name or "/" not in self.model_name:
            raise ValueError("model must use the provider/model format")

        command = [
            "python3",
            "/opt/opencode-agent/runner.py",
            "--instruction",
            shlex.quote(instruction),
            "--model",
            shlex.quote(self.model_name),
            "--timeout",
            str(self._timeout),
        ]
        if self._variant:
            command.extend(["--variant", shlex.quote(self._variant)])
        if self._config is not None and self._config.is_file():
            command.extend(
                ["--config", "/opt/opencode-agent/ai-config.yaml"]
            )

        await self.exec_as_agent(
            environment,
            command=" ".join(command),
            cwd="/app",
            timeout_sec=self._timeout + 30,
        )

    def populate_context_post_run(self, context: AgentContext) -> None:
        super().populate_context_post_run(context)

        output = self.logs_dir / self._OUTPUT_FILENAME
        summary = self.logs_dir / "benchmark-summary.json"
        if not output.is_file() or summary.is_file():
            return

        events: list[dict] = []
        for line in output.read_text(errors="replace").splitlines():
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            if isinstance(event, dict):
                events.append(event)
        summary.write_text(json.dumps(summarize(events), indent=2) + "\n")
