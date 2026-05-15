#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
aibalance Tool Call 完整闭环 E2E (参数化矩阵)

通过环境变量配置 aibalance 端点 + 多模型矩阵, 对每个 model 跑:
  矩阵 = stream(2) x tool_count(单 tool / 并行 2 tool) = 4 case

每个 case 都做一次完整 round-trip:
  1) round1: 模型决定调用工具 (assistant.tool_calls + finish_reason=tool_calls)
  2) round2: 客户端把 tool 执行结果回灌 (role=tool), 模型生成自然语言回答

这才是 codex / OpenAI SDK / langchain / litellm 真正用到的链路, 任何中转 bug
都会在第二轮"tool result 回灌"或"并行 tool"上暴露.

环境变量:
  AIBALANCE_BASE              aibalance 端点 (默认 http://127.0.0.1:8080/v1)
  AIBALANCE_KEY               aibalance API key (优先于 AIBALANCE_KEY_FILE)
  AIBALANCE_KEY_FILE          aibalance API key 文件路径
  AIBALANCE_MODEL             逗号分隔的 model 列表 (覆盖默认 native/dumb/真 deepseek)
  AIBALANCE_ROUND_TRIP_TIMEOUT 单次请求超时秒 (默认 30)
  AIBALANCE_ONLY_STREAM        only test stream=True (跳过非流式)
  AIBALANCE_ONLY_SINGLE        only test 单 tool (跳过并行 tool)

关键词: aibalance tool round-trip matrix, parameterized e2e, native + dumb + real
"""
from __future__ import annotations

import json
import os
import sys
import time
from typing import Any

from openai import OpenAI


WEATHER_TOOL = {
    "type": "function",
    "function": {
        "name": "get_current_weather",
        "description": "Get the current weather of a city.",
        "parameters": {
            "type": "object",
            "properties": {"city": {"type": "string"}},
            "required": ["city"],
        },
    },
}

NEWS_TOOL = {
    "type": "function",
    "function": {
        "name": "fetch_latest_news",
        "description": "Fetch latest news for a topic.",
        "parameters": {
            "type": "object",
            "properties": {"topic": {"type": "string"}},
            "required": ["topic"],
        },
    },
}


def fake_weather(city: str) -> dict:
    return {"city": city, "temperature_c": 21, "condition": "sunny", "wind": "north 2 m/s"}


def fake_news(topic: str) -> dict:
    return {"topic": topic, "headlines": ["headline-1", "headline-2"]}


def collect_tool_calls_streaming(stream_resp) -> tuple[dict[int, dict], str]:
    """从 OpenAI streaming response 累积 tool_calls."""
    tool_calls: dict[int, dict] = {}
    finish_reason = ""
    for chunk in stream_resp:
        if not chunk.choices:
            continue
        ch = chunk.choices[0]
        if ch.finish_reason:
            finish_reason = ch.finish_reason
        if ch.delta is None:
            continue
        for tc in (ch.delta.tool_calls or []):
            idx = tc.index if tc.index is not None else 0
            slot = tool_calls.setdefault(
                idx, {"id": "", "type": "function", "name": "", "arguments": ""}
            )
            if tc.id:
                slot["id"] = tc.id
            if tc.type:
                slot["type"] = tc.type
            if tc.function:
                if tc.function.name:
                    slot["name"] = tc.function.name
                if tc.function.arguments:
                    slot["arguments"] += tc.function.arguments
    return tool_calls, finish_reason


def collect_content_streaming(stream_resp) -> tuple[str, str]:
    """从 OpenAI streaming response 累积 content."""
    parts: list[str] = []
    finish_reason = ""
    for chunk in stream_resp:
        if not chunk.choices:
            continue
        ch = chunk.choices[0]
        if ch.finish_reason:
            finish_reason = ch.finish_reason
        if ch.delta is None:
            continue
        if ch.delta.content:
            parts.append(ch.delta.content)
    return "".join(parts), finish_reason


def run_round_trip(
    client: OpenAI,
    model: str,
    *,
    stream: bool,
    parallel_tools: bool,
    timeout: int,
) -> tuple[bool, str]:
    started = time.time()
    label = (
        f"{model}/{'stream' if stream else 'non-stream'}/"
        f"{'parallel-tools' if parallel_tools else 'single-tool'}"
    )
    tools = [WEATHER_TOOL, NEWS_TOOL] if parallel_tools else [WEATHER_TOOL]
    if parallel_tools:
        user_prompt = (
            "I need TWO things in parallel: (1) current weather in Beijing via "
            "get_current_weather(city='Beijing'), and (2) latest news about AI via "
            "fetch_latest_news(topic='AI'). Please call BOTH tools now. Do not answer in plain text."
        )
    else:
        user_prompt = (
            "I want the current weather in Beijing. Please call "
            "get_current_weather(city='Beijing'). Do not answer in plain text."
        )
    try:
        first_messages = [{"role": "user", "content": user_prompt}]
        if stream:
            stream_resp = client.chat.completions.create(
                model=model,
                messages=first_messages,
                tools=tools,
                tool_choice="auto",
                stream=True,
                max_tokens=512,
                timeout=timeout,
            )
            tool_calls, finish_reason = collect_tool_calls_streaming(stream_resp)
            if finish_reason != "tool_calls" or not tool_calls:
                return False, (
                    f"[{label}] round1 expected tool_calls, got finish_reason="
                    f"{finish_reason!r}, tool_calls={tool_calls}"
                )
            assistant_tool_calls = [
                {
                    "id": v["id"] or f"call_{i}",
                    "type": v["type"] or "function",
                    "function": {"name": v["name"], "arguments": v["arguments"]},
                }
                for i, (_, v) in enumerate(sorted(tool_calls.items()))
            ]
        else:
            r1 = client.chat.completions.create(
                model=model,
                messages=first_messages,
                tools=tools,
                tool_choice="auto",
                stream=False,
                max_tokens=512,
                timeout=timeout,
            )
            if not r1.choices:
                return False, f"[{label}] round1 no choices"
            r1c = r1.choices[0]
            if r1c.finish_reason != "tool_calls" or not r1c.message.tool_calls:
                return False, (
                    f"[{label}] round1 expected tool_calls, got finish_reason="
                    f"{r1c.finish_reason!r}, message.tool_calls={r1c.message.tool_calls}"
                )
            assistant_tool_calls = [
                {
                    "id": tc.id,
                    "type": tc.type,
                    "function": {
                        "name": tc.function.name,
                        "arguments": tc.function.arguments,
                    },
                }
                for tc in r1c.message.tool_calls
            ]

        if parallel_tools and len(assistant_tool_calls) < 2:
            # 部分模型只会调一个 tool, 这里降级允许但记录 warning
            print(
                f"[WARN] {label}: parallel_tools expected 2 tool calls, "
                f"got {len(assistant_tool_calls)}"
            )

        second_messages = list(first_messages)
        second_messages.append(
            {"role": "assistant", "content": "", "tool_calls": assistant_tool_calls}
        )
        for tc in assistant_tool_calls:
            try:
                args = json.loads(tc["function"]["arguments"] or "{}")
            except Exception:
                args = {}
            name = tc["function"]["name"]
            if name == "get_current_weather":
                tool_result: Any = fake_weather(args.get("city", "Beijing"))
            elif name == "fetch_latest_news":
                tool_result = fake_news(args.get("topic", "AI"))
            else:
                tool_result = {"echo": args}
            second_messages.append(
                {
                    "role": "tool",
                    "tool_call_id": tc["id"],
                    "name": name,
                    "content": json.dumps(tool_result, ensure_ascii=False),
                }
            )

        if stream:
            stream_resp2 = client.chat.completions.create(
                model=model,
                messages=second_messages,
                tools=tools,
                tool_choice="auto",
                stream=True,
                max_tokens=512,
                timeout=timeout,
            )
            content, fr2 = collect_content_streaming(stream_resp2)
            if fr2 != "stop":
                return False, (
                    f"[{label}] round2 expected finish_reason=stop, got {fr2!r}; "
                    f"content={content[:160]!r}"
                )
            if not content.strip():
                return False, f"[{label}] round2 final content empty"
            return True, (
                f"[{label}] OK in {time.time()-started:.2f}s, "
                f"final={content.strip()[:140]!r}"
            )
        else:
            r2 = client.chat.completions.create(
                model=model,
                messages=second_messages,
                tools=tools,
                tool_choice="auto",
                stream=False,
                max_tokens=512,
                timeout=timeout,
            )
            if not r2.choices:
                return False, f"[{label}] round2 no choices"
            r2c = r2.choices[0]
            if r2c.finish_reason != "stop":
                return False, (
                    f"[{label}] round2 expected finish_reason=stop, got "
                    f"{r2c.finish_reason!r}; content={r2c.message.content!r}"
                )
            content = (r2c.message.content or "").strip()
            if not content:
                return False, f"[{label}] round2 final content empty"
            elapsed = time.time() - started
            return True, f"[{label}] OK in {elapsed:.2f}s, final={content[:140]!r}"
    except Exception as e:
        return False, f"[{label}] exception: {type(e).__name__}: {str(e)[:200]}"


def load_api_key() -> str:
    env_key = os.environ.get("AIBALANCE_KEY", "").strip()
    if env_key:
        return env_key
    path = os.environ.get(
        "AIBALANCE_KEY_FILE",
        os.path.expanduser("~/yakit-projects/aibalance-key-z.txt"),
    )
    if not os.path.exists(path):
        raise SystemExit(
            f"AIBALANCE_KEY env not set and key file not found: {path}\n"
            f"Set AIBALANCE_KEY=xxx or AIBALANCE_KEY_FILE=/path/to/key"
        )
    return open(path).read().strip()


def main() -> int:
    api_key = load_api_key()
    base_url = os.environ.get("AIBALANCE_BASE", "http://127.0.0.1:8080/v1")
    timeout = int(os.environ.get("AIBALANCE_ROUND_TRIP_TIMEOUT", "30"))
    client = OpenAI(api_key=api_key, base_url=base_url, timeout=timeout)

    env_models = os.environ.get("AIBALANCE_MODEL", "").strip()
    if env_models:
        models = [m.strip() for m in env_models.replace(";", ",").split(",") if m.strip()]
    else:
        models = ["mock-native", "mock-dumb"]

    only_stream = os.environ.get("AIBALANCE_ONLY_STREAM", "").lower() in ("1", "true", "yes")
    only_single = os.environ.get("AIBALANCE_ONLY_SINGLE", "").lower() in ("1", "true", "yes")

    streams = [True] if only_stream else [True, False]
    tool_modes = [False] if only_single else [False, True]  # single, parallel

    print(f"aibalance E2E Tool Round-trip Matrix")
    print(f"  base_url = {base_url}")
    print(f"  models   = {models}")
    print(f"  streams  = {streams}")
    print(f"  tool_modes = {['single' if not p else 'parallel' for p in tool_modes]}")
    print()

    results = []
    for m in models:
        for s in streams:
            for p in tool_modes:
                ok, detail = run_round_trip(
                    client, m,
                    stream=s, parallel_tools=p, timeout=timeout,
                )
                results.append((ok, detail))
                print(("PASS" if ok else "FAIL"), detail)
    passed = sum(1 for ok, _ in results if ok)
    print()
    print(f"Tool round-trip summary: {passed}/{len(results)} passed")
    return 0 if passed == len(results) else 1


if __name__ == "__main__":
    sys.exit(main())
