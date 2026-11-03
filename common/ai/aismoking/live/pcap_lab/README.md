# PCAP agent 开发脚手架

开发 AI 用它启动真实 ReAct、阅读轨迹、交给独立 AI 复盘，再修改工具/Skill 并重跑。没有固定工具路径或评分 gate；命令行与其他内置工具保持可用。`finished` 只表示运行结束，不代表分析正确。

完整工作流、真实测试目标、汇报格式和当前能力边界见
[`docs/design/pcap-analysis-agent-tdd.md`](../../../../../docs/design/pcap-analysis-agent-tdd.md)。

下一批真实 agent 测试、工具改进、协议缺口和验收状态统一维护在
[`docs/design/pcap-analysis-todo.md`](../../../../../docs/design/pcap-analysis-todo.md)。

从任意目录执行（Python 3 标准库、Go 和当前仓库构建依赖）：

```sh
python3 common/ai/aismoking/live/pcap_lab/run.py run \
  --capture testdata/enterprise-traffic-analysis/http-dga-c2/capture.pcapng \
  --output /tmp/pcap-lab-baseline

python3 common/ai/aismoking/live/pcap_lab/run.py inspect \
  --output /tmp/pcap-lab-baseline

python3 common/ai/aismoking/live/pcap_lab/run.py review \
  --source /tmp/pcap-lab-baseline \
  --binary /tmp/pcap-lab-baseline/yak \
  --output /tmp/pcap-lab-review
```

示例相对路径按仓库根目录书写。省略 `--binary` 时构建当前工作树；复测改动需重新构建，不能直接复用旧二进制。提供二进制时记录其 SHA256，但不声称它与工作树一致。`--task-file` 接受自然语言问题；`--timeout`（默认600秒）、`--iterations`（默认40）设置预算。每次 `--output` 必须是新目录。复盘可传 `--baseline` 对照另一次运行。

默认从 `$YAKIT_HOME/yakit-profile-plugin.db`（未设置时为 `~/yakit-projects/yakit-profile-plugin.db`）读取 GUI 的 `AI_GLOBAL_CONFIG`，可用 `--profile` 指定实际数据库。通过 SQLite 只读连接复制这一条设置到运行目录的独立 profile，不复制历史、工具缓存或流量目录。模型分层、路由、provider 参数保留原样；没有配置或只有旧 provider 引用时明确报错。当前不复制 GUI 的自定义插件、MCP 配置、外部 Skill 或历史记忆，属于内置能力基线，不等同于完整 GUI 会话。

每轮自动创建独立 home、工作目录和 session，样本复制为中性名称。源 profile 不做迁移或写入。运行目录权限0700，内部 home 含API配置，不能上传或提交；轨迹也可能包含流量敏感信息，只作本地开发材料。工作目录不是操作系统沙箱，任务范围由任务指令约束。此脚本会调用真实模型并产生费用，不加入 CI。

产物：

- `run.json`：版本、构建来源、配置摘要、预算、状态；不包含API密钥。
- `task.txt`、`answer.md`：实际问题及最终回答（运行中断时可能没有回答）。
- `events.jsonl`：按序记录事件和流式输出，运行中可查看。
- `trace.md`、`calls.json`：离线生成的阅读索引，长内容明确标注截断；原始事件保持完整。
- `timeline.txt`、`evidence.txt`、`streams.json`：运行时上下文和输出；工具产物路径见事件。
- `engine.log`、`result.json`：引擎诊断和执行结果。
- 复盘运行的 `workspace/feedback.md`：独立 AI 的建议，其本身的轨迹也完整保存。

`inspect` 不请求模型，可在运行中或结束后重建索引。超时或 Ctrl-C 会停止该轮进程组并保留已有记录。当前优先串行运行；不要让多次实验共享 output/home。
