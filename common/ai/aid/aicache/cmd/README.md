# Continue control reasonbench

本目录包含两个需要显式运行的在线实验命令，用于验证 native tool-call
能否作为 ReActLoop 的 `continue` / `finish` 控制边界，并减少模型每轮新增的
`reasoning_content`。普通 `go test` 只运行离线协议测试，不会调用模型。

## `tooltailbench`

比较同一段 reasoning 历史在以下消息尾部结构中的续接效果：

- matching tool result 位于请求结尾；
- tool result 后追加 user message；
- user message 位于完整 tool-call pair 之前；
- 普通 stop-JSON 反例。

```bash
go run ./common/ai/aid/aicache/cmd/tooltailbench \
  -model memfit-standard-thinking-free \
  -trials 5 \
  -pause 4s
```

## `reactloopreasonbench`

运行完整的多轮控制链：前 `N-1` 轮必须 `continue`，第 `N` 轮必须
`finish`，控制器返回 `finished / terminated_safely` 后不得再发起模型请求。

实验包含三个 arm：

- `negative_yak_stop_json`：普通 assistant JSON，以 `stop` 结束；
- `positive_native_toolcall`：保留完整 `react_control` 历史；
- `positive_native_toolcall_trimmed_history`：只保留最新一个完整的
  `react_control(action=continue)` 与 matching tool result。

裁剪只会删除已完成的 `react_control(action=continue)` pair，不会删除业务工具
调用或 `finish`。最新 tool result 携带 `accepted_checkpoints` 累计状态快照，
最终 `finish` 必须复述全部 checkpoint，避免把“协议还能继续”误判为“状态没有
丢失”。

```bash
go run ./common/ai/aid/aicache/cmd/reactloopreasonbench \
  -model memfit-standard-thinking-free \
  -only-arm positive_native_toolcall_trimmed_history \
  -trials 5 \
  -rounds 6 \
  -pause 8s \
  -max-tokens 1800 \
  -out /tmp/yak-reactloop-trimmed-history-5x6.json
```

命令使用本机 Yak 网络配置访问 AIBalance。报告默认写入 `/tmp`，权限为
`0600`，其中可能包含模型返回的 reasoning；不要将真实报告提交到仓库。

## 离线验证

```bash
go test ./common/ai/aid/aicache/cmd/reactloopreasonbench \
  ./common/ai/aid/aicache/cmd/tooltailbench \
  -count=1
```
