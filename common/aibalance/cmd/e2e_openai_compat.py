#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
aibalance OpenAI 兼容性 E2E 验证

验证场景:
  1. stream=True  + tools         -> 必须能聚合出完整 tool_calls,
     finish_reason 必须是 "tool_calls"
  2. stream=False + tools         -> 完整 chat.completion JSON 必须含 tool_calls,
     finish_reason 必须是 "tool_calls"
  3. stream=True  + 纯文本        -> finish_reason 必须保持 "stop"
  4. stream=False + 纯文本        -> finish_reason 必须保持 "stop"
  5. reasoning_content 透传 (thinking 模型)

覆盖模型 (z-* 系列, 经过 aibalance 中转):
  - z-deepseek-v4-pro      (deepseek thinking + tools, 用户主报场景)
  - z-deepseek-v4-flash    (deepseek 非 thinking)
  - z-kimi-k2.6            (kimi)
  - z-gpt5.4               (openai 兼容)
  - z-sonnet-4-6           (claude 兼容)
  - qwen3.5-397b-a17b-free (qwen)

关键词: aibalance OpenAI 兼容性, finish_reason tool_calls, reasoning_content
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import time
from dataclasses import dataclass, field
from typing import Any

from openai import OpenAI
from openai._exceptions import APIError, APIStatusError, APITimeoutError


BASE_URL = "https://aibalance.yaklang.com/v1"
KEY_PATH = os.path.expanduser("~/yakit-projects/aibalance-key-z.txt")


def read_api_key(path: str) -> str:
    with open(path, "r", encoding="utf-8") as f:
        return f.read().strip()


WEATHER_TOOL = {
    "type": "function",
    "function": {
        "name": "get_current_weather",
        "description": "Get the current weather of a city.",
        "parameters": {
            "type": "object",
            "properties": {
                "city": {"type": "string", "description": "City name in English"},
            },
            "required": ["city"],
        },
    },
}

TIME_TOOL = {
    "type": "function",
    "function": {
        "name": "get_current_time",
        "description": "Get the current time of a timezone.",
        "parameters": {
            "type": "object",
            "properties": {
                "tz": {"type": "string", "description": "IANA timezone name"},
            },
            "required": ["tz"],
        },
    },
}


@dataclass
class CaseResult:
    model: str
    case: str
    ok: bool
    detail: str = ""
    elapsed_s: float = 0.0
    extras: dict[str, Any] = field(default_factory=dict)


def fmt_status(ok: bool) -> str:
    return "PASS" if ok else "FAIL"


def fmt_short(value: Any, n: int = 80) -> str:
    s = str(value).replace("\n", " ").strip()
    if len(s) <= n:
        return s
    return s[: n - 3] + "..."


# ------------------------------------------------------------------
# Case 1: streaming + tools (主修复路径)
# 关键词: 流式 tool_calls 聚合, finish_reason tool_calls
# ------------------------------------------------------------------
def case_stream_tools(client: OpenAI, model: str, timeout_s: float = 60.0) -> CaseResult:
    started = time.time()
    try:
        stream = client.chat.completions.create(
            model=model,
            messages=[
                {
                    "role": "user",
                    "content": (
                        "I need the current weather in Beijing right now. "
                        "Please invoke the get_current_weather tool with city='Beijing'. "
                        "Do not answer in plain text, just call the tool."
                    ),
                },
            ],
            tools=[WEATHER_TOOL],
            tool_choice="auto",
            stream=True,
            max_tokens=512,
            timeout=timeout_s,
        )

        tool_calls: dict[int, dict[str, Any]] = {}
        reasoning_chunks: list[str] = []
        content_chunks: list[str] = []
        finish_reason: str = ""
        chunks_seen = 0

        for chunk in stream:
            chunks_seen += 1
            if not chunk.choices:
                continue
            choice = chunk.choices[0]
            if choice.finish_reason:
                finish_reason = choice.finish_reason
            delta = choice.delta
            if delta is None:
                continue
            if getattr(delta, "content", None):
                content_chunks.append(delta.content)
            rc = getattr(delta, "reasoning_content", None)
            if rc:
                reasoning_chunks.append(rc)
            for tc in (delta.tool_calls or []):
                idx = tc.index if tc.index is not None else 0
                slot = tool_calls.setdefault(
                    idx,
                    {"id": "", "type": "function", "name": "", "arguments": ""},
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

        elapsed = time.time() - started
        ok = (
            finish_reason == "tool_calls"
            and len(tool_calls) >= 1
            and any(slot["name"] for slot in tool_calls.values())
        )
        detail = ""
        if not ok:
            detail = (
                f"finish_reason={finish_reason!r} tool_calls={tool_calls} "
                f"chunks_seen={chunks_seen} content={fmt_short(''.join(content_chunks))!r}"
            )
        return CaseResult(
            model=model,
            case="stream+tools",
            ok=ok,
            detail=detail,
            elapsed_s=elapsed,
            extras={
                "finish_reason": finish_reason,
                "tool_calls": list(tool_calls.values()),
                "reasoning_len": sum(len(s) for s in reasoning_chunks),
                "content_len": sum(len(s) for s in content_chunks),
                "chunks_seen": chunks_seen,
            },
        )
    except (APIError, APIStatusError, APITimeoutError, Exception) as e:
        return CaseResult(
            model=model,
            case="stream+tools",
            ok=False,
            detail=f"exception: {type(e).__name__}: {fmt_short(e, 200)}",
            elapsed_s=time.time() - started,
        )


# ------------------------------------------------------------------
# Case 2: non-stream + tools
# 关键词: 非流式 tool_calls 完整体, application/json 收尾
# ------------------------------------------------------------------
def case_non_stream_tools(client: OpenAI, model: str, timeout_s: float = 60.0) -> CaseResult:
    started = time.time()
    try:
        resp = client.chat.completions.create(
            model=model,
            messages=[
                {
                    "role": "user",
                    "content": (
                        "I need the current weather in Shanghai right now. "
                        "Please invoke the get_current_weather tool with city='Shanghai'. "
                        "Do not answer in plain text, just call the tool."
                    ),
                },
            ],
            tools=[WEATHER_TOOL],
            tool_choice="auto",
            stream=False,
            max_tokens=512,
            timeout=timeout_s,
        )
        elapsed = time.time() - started
        choice = resp.choices[0] if resp.choices else None
        if choice is None:
            return CaseResult(model, "non-stream+tools", False, "no choices in response",
                              elapsed)
        msg = choice.message
        tool_calls = msg.tool_calls or []
        finish_reason = choice.finish_reason or ""
        ok = (
            finish_reason == "tool_calls"
            and len(tool_calls) >= 1
            and bool(tool_calls[0].function.name)
        )
        detail = ""
        if not ok:
            detail = (
                f"finish_reason={finish_reason!r} tool_calls_len={len(tool_calls)} "
                f"content={fmt_short(getattr(msg, 'content', ''))!r}"
            )
        return CaseResult(
            model=model,
            case="non-stream+tools",
            ok=ok,
            detail=detail,
            elapsed_s=elapsed,
            extras={
                "finish_reason": finish_reason,
                "tool_call_first": (
                    {
                        "id": tool_calls[0].id,
                        "name": tool_calls[0].function.name,
                        "arguments": tool_calls[0].function.arguments,
                    }
                    if tool_calls
                    else None
                ),
                "reasoning": fmt_short(getattr(msg, "reasoning_content", "") or "", 120),
                "content": fmt_short(msg.content or "", 120),
            },
        )
    except (APIError, APIStatusError, APITimeoutError, Exception) as e:
        return CaseResult(
            model=model,
            case="non-stream+tools",
            ok=False,
            detail=f"exception: {type(e).__name__}: {fmt_short(e, 200)}",
            elapsed_s=time.time() - started,
        )


# ------------------------------------------------------------------
# Case 3: streaming pure-text (回归 finish_reason=stop)
# 关键词: 防回归 finish_reason stop, 纯文本流式
# ------------------------------------------------------------------
def case_stream_text(client: OpenAI, model: str, timeout_s: float = 30.0) -> CaseResult:
    started = time.time()
    try:
        stream = client.chat.completions.create(
            model=model,
            messages=[
                {
                    "role": "user",
                    "content": (
                        "Reply with exactly one short English sentence introducing yourself. "
                        "Do not call any tool."
                    ),
                }
            ],
            stream=True,
            max_tokens=256,
            timeout=timeout_s,
        )
        finish_reason = ""
        content_chunks: list[str] = []
        reasoning_chunks: list[str] = []
        for chunk in stream:
            if not chunk.choices:
                continue
            choice = chunk.choices[0]
            if choice.finish_reason:
                finish_reason = choice.finish_reason
            if choice.delta and getattr(choice.delta, "content", None):
                content_chunks.append(choice.delta.content)
            rc = getattr(choice.delta, "reasoning_content", None) if choice.delta else None
            if rc:
                reasoning_chunks.append(rc)
        elapsed = time.time() - started
        content = "".join(content_chunks)
        reasoning = "".join(reasoning_chunks)
        # 部分 thinking 模型 max_tokens=256 时 content 可能为空但 reasoning 不为空,
        # 这种情况依然算"流通"——本 case 主要校验 finish_reason 不被错误改成 tool_calls。
        ok = finish_reason == "stop" and (len(content) > 0 or len(reasoning) > 0)
        return CaseResult(
            model=model,
            case="stream+text",
            ok=ok,
            detail="" if ok else f"finish_reason={finish_reason!r} content={fmt_short(content)!r}",
            elapsed_s=elapsed,
            extras={"finish_reason": finish_reason, "content": fmt_short(content, 60)},
        )
    except (APIError, APIStatusError, APITimeoutError, Exception) as e:
        return CaseResult(
            model=model,
            case="stream+text",
            ok=False,
            detail=f"exception: {type(e).__name__}: {fmt_short(e, 200)}",
            elapsed_s=time.time() - started,
        )


# ------------------------------------------------------------------
# Case 4: non-stream pure-text
# 关键词: 非流式纯文本, application/json 完整 chat.completion
# ------------------------------------------------------------------
def case_non_stream_text(client: OpenAI, model: str, timeout_s: float = 30.0) -> CaseResult:
    started = time.time()
    try:
        resp = client.chat.completions.create(
            model=model,
            messages=[
                {
                    "role": "user",
                    "content": (
                        "Reply with exactly one short English sentence introducing yourself. "
                        "Do not call any tool."
                    ),
                }
            ],
            stream=False,
            max_tokens=256,
            timeout=timeout_s,
        )
        elapsed = time.time() - started
        if not resp.choices:
            return CaseResult(model, "non-stream+text", False, "no choices", elapsed)
        choice = resp.choices[0]
        finish_reason = choice.finish_reason or ""
        content = choice.message.content or ""
        reasoning = getattr(choice.message, "reasoning_content", "") or ""
        ok = finish_reason == "stop" and (len(content) > 0 or len(reasoning) > 0)
        return CaseResult(
            model=model,
            case="non-stream+text",
            ok=ok,
            detail="" if ok else f"finish_reason={finish_reason!r} content={fmt_short(content)!r}",
            elapsed_s=elapsed,
            extras={"finish_reason": finish_reason, "content": fmt_short(content, 60)},
        )
    except (APIError, APIStatusError, APITimeoutError, Exception) as e:
        return CaseResult(
            model=model,
            case="non-stream+text",
            ok=False,
            detail=f"exception: {type(e).__name__}: {fmt_short(e, 200)}",
            elapsed_s=time.time() - started,
        )


# ------------------------------------------------------------------
# Case 5: parallel tool_calls (多函数同时调用)
# 关键词: 并行 tool_calls, 多 index 不串号
# ------------------------------------------------------------------
def case_stream_parallel_tools(client: OpenAI, model: str, timeout_s: float = 60.0) -> CaseResult:
    started = time.time()
    try:
        stream = client.chat.completions.create(
            model=model,
            messages=[
                {
                    "role": "user",
                    "content": (
                        "I need both the current weather of Beijing AND the current "
                        "time of Asia/Shanghai. Please call BOTH "
                        "get_current_weather(city='Beijing') AND "
                        "get_current_time(tz='Asia/Shanghai') tools. Do not answer in plain text."
                    ),
                },
            ],
            tools=[WEATHER_TOOL, TIME_TOOL],
            tool_choice="auto",
            stream=True,
            max_tokens=512,
            timeout=timeout_s,
        )
        tool_calls: dict[int, dict[str, Any]] = {}
        finish_reason = ""
        for chunk in stream:
            if not chunk.choices:
                continue
            choice = chunk.choices[0]
            if choice.finish_reason:
                finish_reason = choice.finish_reason
            delta = choice.delta
            if delta is None:
                continue
            for tc in (delta.tool_calls or []):
                idx = tc.index if tc.index is not None else 0
                slot = tool_calls.setdefault(
                    idx,
                    {"id": "", "type": "function", "name": "", "arguments": ""},
                )
                if tc.id:
                    slot["id"] = tc.id
                if tc.function:
                    if tc.function.name:
                        slot["name"] = tc.function.name
                    if tc.function.arguments:
                        slot["arguments"] += tc.function.arguments
        elapsed = time.time() - started
        names = sorted({s["name"] for s in tool_calls.values() if s["name"]})
        ok = (
            finish_reason == "tool_calls"
            and len(tool_calls) >= 1  # 模型可能只调用 1 个, 也可能并行 2 个; 至少要有 1 个
            and bool(names)
        )
        return CaseResult(
            model=model,
            case="stream+parallel-tools",
            ok=ok,
            detail="" if ok else f"finish_reason={finish_reason!r} tool_calls={tool_calls}",
            elapsed_s=elapsed,
            extras={
                "finish_reason": finish_reason,
                "tool_calls_count": len(tool_calls),
                "tool_names": names,
            },
        )
    except (APIError, APIStatusError, APITimeoutError, Exception) as e:
        return CaseResult(
            model=model,
            case="stream+parallel-tools",
            ok=False,
            detail=f"exception: {type(e).__name__}: {fmt_short(e, 200)}",
            elapsed_s=time.time() - started,
        )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--models", nargs="+", default=None,
                        help="Models to test (default: built-in list)")
    parser.add_argument("--cases", nargs="+", default=None,
                        choices=["stream-tools", "non-stream-tools", "stream-text",
                                 "non-stream-text", "parallel-tools"],
                        help="Cases to run (default: all)")
    parser.add_argument("--timeout", type=float, default=60.0)
    args = parser.parse_args()

    api_key = read_api_key(KEY_PATH)
    client = OpenAI(api_key=api_key, base_url=BASE_URL, timeout=args.timeout)

    default_models = [
        "z-deepseek-v4-pro",
        "z-deepseek-v4-flash",
        "z-kimi-k2.6",
        "z-gpt5.4",
        "z-sonnet-4-6",
        "qwen3.5-397b-a17b-free",
    ]
    models = args.models or default_models

    case_table = {
        "stream-tools": case_stream_tools,
        "non-stream-tools": case_non_stream_tools,
        "stream-text": case_stream_text,
        "non-stream-text": case_non_stream_text,
        "parallel-tools": case_stream_parallel_tools,
    }
    cases = args.cases or list(case_table.keys())

    results: list[CaseResult] = []
    print(f"==> Test against {BASE_URL}")
    print(f"    models = {models}")
    print(f"    cases  = {cases}")
    print()
    for model in models:
        print(f"--- model: {model}")
        for case_name in cases:
            fn = case_table[case_name]
            r = fn(client, model, timeout_s=args.timeout)
            results.append(r)
            line = f"  [{fmt_status(r.ok)}] {case_name:<22s}  {r.elapsed_s:6.2f}s"
            if r.extras:
                fr = r.extras.get("finish_reason", "")
                if fr:
                    line += f"  fr={fr}"
                if "tool_names" in r.extras:
                    line += f"  tool_names={r.extras['tool_names']}"
                elif "tool_calls" in r.extras and r.extras["tool_calls"]:
                    names = [tc["name"] for tc in r.extras["tool_calls"]]
                    line += f"  tool_names={names}"
                elif "tool_call_first" in r.extras and r.extras["tool_call_first"]:
                    line += f"  tool={r.extras['tool_call_first']['name']}"
            print(line)
            if not r.ok and r.detail:
                print(f"      detail: {r.detail}")
        print()

    print("=" * 78)
    print("Summary:")
    pass_count = sum(1 for r in results if r.ok)
    print(f"  {pass_count}/{len(results)} passed")
    if pass_count != len(results):
        print()
        print("Failed cases:")
        for r in results:
            if not r.ok:
                print(f"  [{r.model}] {r.case}: {r.detail}")
        return 1
    print("  ALL PASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
