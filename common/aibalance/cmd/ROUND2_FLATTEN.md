# Round2 ReAct Flatten 兼容兜底

## 背景

部分上游 wrapper (如线上 `z-deepseek-v4-pro` / `z-deepseek-v4-flash`) 不识别
OpenAI 标准 `tool_calls` round-trip 协议字段。当传统 OpenAI 兼容客户端
(OpenAI Python/Node SDK / Codex / OpenCode / litellm 等) 在 round2 把
`assistant.tool_calls` + `role=tool` 消息回灌给 aibalance 时, wrapper 收到这些
字段就立刻 `finish_reason=stop` 给出空响应, 客户端的工具调用流程在 round2
被截断, 拿不到模型基于工具结果的自然语言总结。

线上 curl 探测确认:

| messages 形态 | 上游响应 |
| --- | --- |
| `[user]` 单轮 | 正常生成 |
| `[user, assistant(text), user]` 3 轮纯文本 | 正常生成 |
| `[user, assistant(tool_calls), tool]` round2 | **空回, 立即 finish_reason=stop** |
| `[user, assistant(tool_calls), user(text)]` (assistant 仍带 tool_calls) | **空回** |
| `[user, assistant(text), user(text)]` 模拟 ReAct 文本 | 正常生成 |

结论: wrapper 一旦看到 `assistant.tool_calls` / `role=tool` 字段就拒绝输出。

`yaklang` 内置 OpenAI client 与 OpenAI Python SDK 走相同 OpenAI tool_calls
协议复现完全一致的空响应 (yak runner / AID 截图能跑通是因为它走 yaklang
自家 prompt 模板协议, 不依赖 OpenAI tool_calls round-trip)。

## 修复方案

在 aibalance 中转层引入一个 **provider/model opt-in 的 round2 flatten 兜底**:
当且仅当客户端 messages 真的包含 round-trip 标记 (`assistant.tool_calls`
非空 或 `role=tool`), 且当前 model/wrapper 命中环境变量白名单时, aibalance
把 round-trip messages 自动改写成 ReAct 文本风格再透传给上游 wrapper:

- `assistant.tool_calls + content=""` -> `assistant.content = "<原文本>\n[tool_call id=... name=...]\n<arguments>\n[/tool_call]"`, 同时清空 tool_calls 字段
- `role=tool` -> `role=user`, content 渲染成 `[tool_result name=... tool_call_id=...]\n<原 content>\n[/tool_result]`

实现细节见:
- `common/aibalance/round2_flatten.go` (纯函数 + env 解析, 无副作用)
- `common/aibalance/server.go` 在 `messagesForUpstream` 计算后注入
- `common/aibalance/round2_flatten_test.go` 13 个单元测试
- `common/aibalance/round2_flatten_integration_test.go` 3 个集成测试覆盖
  hostile wrapper 端到端

## 部署启用

修复**默认关闭**, 不会影响任何已有 provider 的原生 tool_calls 体验。
在 aibalance 部署侧设置环境变量即可启用:

```bash
# 仅对指定 model/wrapper 启用 (推荐, 大小写不敏感, 容忍 , ; 空白多种分隔)
export AIBALANCE_FLATTEN_TOOLCALLS_FOR_MODELS="z-deepseek-v4-pro,z-deepseek-v4-flash"

# 紧急 kill switch: 对所有 provider 启用 (谨慎使用, 会让支持 tool_calls 的 wrapper 也降级到 ReAct)
export AIBALANCE_FLATTEN_TOOLCALLS_ALL=true
```

启用后服务端日志会出现:
```
round2 ReAct flatten applied: model=z-deepseek-v4-pro wrapper=z-deepseek-v4-pro provider=deepseek msgs=3->3 (env-driven)
```

## 部署后验证

部署 + 设置 env 后, 在本地用 OpenAI Python SDK 复跑 e2e_tool_roundtrip.py:

```bash
cd common/aibalance/cmd
python3 -m venv /tmp/aib-e2e-venv
source /tmp/aib-e2e-venv/bin/activate
pip install openai
AIBALANCE_BASE=https://aibalance.yaklang.com/v1 \
  AIBALANCE_KEY_FILE=$HOME/yakit-projects/aibalance-key-z.txt \
  AIBALANCE_MODEL=z-deepseek-v4-pro \
  python e2e_tool_roundtrip.py
```

修复生效后, round2 应拿到模型基于工具结果的自然语言总结
(例如 `"The weather in Beijing is sunny, 21C"`),
而不是之前的 `finish_reason=stop` 空 chunk。
