#!/usr/bin/env python3
"""Generate a tiered AI config file for Yak Agent benchmarks.

The gateway resolves its AI provider from a tiered config persisted in the
profile DB.  Without a seeded config, ``AIService=openai`` silently falls back
to the built-in free ``memfit-*-free`` model, making any benchmark reward
meaningless.

This script reads credentials from environment variables and emits a
simple YAML file consumed by ``gateway_runner.py`` at container startup.
The runner parses the YAML and posts the config to the gateway's HTTP API
(``POST /agent/setting/aiconfig``) as a full ``AIGlobalConfig`` payload.

Required env:
  YAK_AI_API_KEY    provider API key (kept out of git)
  YAK_AI_DOMAIN     API host (gateway appends /v1/chat/completions)
                    (e.g. "api.openai.com" or "api.deepseek.com")
  YAK_AI_MODEL      exact model id, pinned identically for base + candidate

Optional env:
  YAK_AI_TYPE       provider type (default: "openai")
  YAK_BENCHMARK_DISABLE_FALLBACK  "1" = disable free-model fallback (default)

Output format (minimal YAML)::

    enabled: true
    routing_policy: ""
    disable_fallback: true
    intelligent_configs:
      - type: openai
        api_key: sk-...
        domain: api.openai.com
        model: gpt-5.2

Security: the output file is written with mode 0600 (owner read/write only).
"""
from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path


def esc(value: str) -> str:
    """Minimal YAML scalar escaping for values with special characters."""
    if value == "" or any(c in value for c in ":#{}[]&,*?|<>=!%@`\"\n\\"):
        return '"' + value.replace("\\", "\\\\").replace('"', '\\"') + '"'
    return value


def render_yaml(
    ai_type: str,
    api_key: str,
    domain: str,
    model: str,
    disable_fallback: bool,
    *,
    lightweight_model: str = "",
    vision_model: str = "",
) -> str:
    """Render the tiered-ai-config in the format gateway_runner.py expects."""
    lines = [
        "enabled: true",
        "routing_policy: \"\"",
        f"disable_fallback: {'true' if disable_fallback else 'false'}",
        "intelligent_configs:",
        f"  - type: {esc(ai_type)}",
        f"    api_key: {esc(api_key)}",
        f"    domain: {esc(domain)}",
        f"    model: {esc(model)}",
    ]
    # Optionally include lightweight / vision tier entries using the same
    # provider but different model names.
    if lightweight_model:
        lines.append("lightweight_configs:")
        lines.append(f"  - type: {esc(ai_type)}")
        lines.append(f"    api_key: {esc(api_key)}")
        lines.append(f"    domain: {esc(domain)}")
        lines.append(f"    model: {esc(lightweight_model)}")
    if vision_model:
        lines.append("vision_configs:")
        lines.append(f"  - type: {esc(ai_type)}")
        lines.append(f"    api_key: {esc(api_key)}")
        lines.append(f"    domain: {esc(domain)}")
        lines.append(f"    model: {esc(vision_model)}")
    return "\n".join(lines) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Generate tiered AI config for Yak Agent benchmarks"
    )
    parser.add_argument(
        "--output", "-o",
        type=Path,
        default=Path(__file__).resolve().parents[1] / "ai-config.yaml",
        help="Output path (default: benchmarks/harbor/ai-config.yaml)",
    )
    parser.add_argument(
        "--stdout",
        action="store_true",
        help="Write to stdout instead of a file",
    )
    parser.add_argument(
        "--lightweight-model",
        default="",
        help="Optional model for lightweight tier",
    )
    parser.add_argument(
        "--vision-model",
        default="",
        help="Optional model for vision tier",
    )
    args = parser.parse_args()

    ai_type = os.environ.get("YAK_AI_TYPE", "openai").strip()
    api_key = os.environ.get("YAK_AI_API_KEY", "").strip()
    domain = os.environ.get("YAK_AI_DOMAIN", "").strip()
    model = os.environ.get("YAK_AI_MODEL", "").strip()
    disable_fallback = (
        os.environ.get("YAK_BENCHMARK_DISABLE_FALLBACK", "1").strip() != "0"
    )

    missing = [
        name
        for name, value in (
            ("YAK_AI_API_KEY", api_key),
            ("YAK_AI_DOMAIN", domain),
            ("YAK_AI_MODEL", model),
        )
        if not value
    ]
    if missing:
        print(f"missing required env: {', '.join(missing)}", file=sys.stderr)
        return 2

    yaml_text = render_yaml(
        ai_type, api_key, domain, model, disable_fallback,
        lightweight_model=args.lightweight_model,
        vision_model=args.vision_model,
    )

    if args.stdout:
        sys.stdout.write(yaml_text)
        return 0

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(yaml_text)
    args.output.chmod(0o600)
    print(f"wrote tiered-ai-config to {args.output}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
