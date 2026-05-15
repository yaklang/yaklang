#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
aibalance Tool Call 完整闭环 E2E

验证:
  1) 模型决定调用工具 (assistant.message.tool_calls)
  2) 客户端把 tool 执行结果作为 role=tool 消息回灌
  3) 模型基于 tool 结果输出最终自然语言回答, finish_reason=stop

这才是 codex / OpenAI SDK / langchain / litellm 真正用到的链路, 任何
中转 bug 都会在第二轮"tool result 回灌"时暴露 (上游会拒绝 / hang /
丢回 stop) 。

关键词: tool round-trip, OpenAI SDK 闭环, 中转兼容性
"""
from __future__ import annotations

import json
import os
import sys
import time

from openai import OpenAI


BASE_URL = "https://aibalance.yaklang.com/v1"
KEY_PATH = os.path.expanduser("~/yakit-projects/aibalance-key-z.txt")


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


def fake_weather(city: str) -> dict:
    return {"city": city, "temperature_c": 21, "condition": "sunny", "wind": "north 2 m/s"}


def run_round_trip(client: OpenAI, model: str, stream: bool) -> tuple[bool, str]:
    started = time.time()
    label = f"{model}/{'stream' if stream else 'non-stream'}"
    try:
        # ---------- 第一轮: 让模型决定调用工具 ----------
        first_messages = [
            {
                "role": "user",
                "content": (
                    "I want the current weather in Beijing. Please call "
                    "get_current_weather(city='Beijing'). Do not answer in plain text."
                ),
            }
        ]
        if stream:
            tool_calls: dict[int, dict] = {}
            finish_reason = ""
            stream_resp = client.chat.completions.create(
                model=model,
                messages=first_messages,
                tools=[WEATHER_TOOL],
                tool_choice="auto",
                stream=True,
                max_tokens=512,
            )
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
                tools=[WEATHER_TOOL],
                tool_choice="auto",
                stream=False,
                max_tokens=512,
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

        # ---------- 第二轮: 回灌 tool result, 让模型生成最终自然语言回答 ----------
        second_messages = list(first_messages)
        second_messages.append(
            {
                "role": "assistant",
                "content": "",
                "tool_calls": assistant_tool_calls,
            }
        )
        for tc in assistant_tool_calls:
            try:
                args = json.loads(tc["function"]["arguments"] or "{}")
            except Exception:
                args = {}
            tool_result = fake_weather(args.get("city", "Beijing"))
            second_messages.append(
                {
                    "role": "tool",
                    "tool_call_id": tc["id"],
                    "name": tc["function"]["name"],
                    "content": json.dumps(tool_result, ensure_ascii=False),
                }
            )

        r2 = client.chat.completions.create(
            model=model,
            messages=second_messages,
            tools=[WEATHER_TOOL],
            tool_choice="auto",
            stream=False,
            max_tokens=512,
        )
        if not r2.choices:
            return False, f"[{label}] round2 no choices"
        r2c = r2.choices[0]
        # round2 期望: 模型根据 tool 结果输出自然语言, finish_reason=stop
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


def main() -> int:
    api_key = open(KEY_PATH).read().strip()
    client = OpenAI(api_key=api_key, base_url=BASE_URL, timeout=120)

    # 仅测试已确认工作的模型, 避免在 0-byte provider 上浪费时间
    models = ["z-deepseek-v4-pro", "z-deepseek-v4-flash", "qwen3.5-397b-a17b-free"]

    results = []
    for m in models:
        for s in (True, False):
            ok, detail = run_round_trip(client, m, s)
            results.append((ok, detail))
            print(("PASS" if ok else "FAIL"), detail)

    passed = sum(1 for ok, _ in results if ok)
    print()
    print(f"Tool round-trip summary: {passed}/{len(results)} passed")
    return 0 if passed == len(results) else 1


if __name__ == "__main__":
    sys.exit(main())
