# gRPC / coreplugin 测试路径

## 分层与边界

- 纯逻辑：直接调用函数。例如 reasoning effort 探测通过函数参数传入 AI 调用，不创建 Server、数据库或网络连接。
- Handler + 存储：使用独立内存 SQLite，迁移实际表，只替换 Server 实例的在线服务边界。必须断言 SQL 过滤、序列化内容、真实保存记录和失败结果；不能替换 GORM、数据库访问器或保存函数。
- gRPC 契约：通过 `newInMemoryClient` / `NewLocalClientAndServerWithTempDatabase` 使用 bufconn。保留 protobuf、流、EOF、deadline、取消等行为，不抢占 TCP 端口，不固定 sleep。独立客户端由 `t.Cleanup` 关闭；临时数据库先关闭所有连接，再删除目录。
- 完整插件：`NewLocalClient` 在本地与 CI 都使用进程内 gRPC。靶场 HTTP、DNSLog 协议服务等真实网络边界仍使用操作系统分配的端口。DNSLog 服务只返回本轮实际观察到的回连；未知 token 不会访问公网。Fastjson 使用靶场的 DNS 解析观察入口，运行原始插件源码，不改写 risk 调用。

持久化 SSA 搜索测试在查询前移除当前程序的编译缓存，确保通过数据库重载进行验证。进程内服务和编译器共享缓存，不能把进程隔离当成隐含的测试前置条件。

`NewLocalClient` 是包级共享客户端，适用于仍依赖全局数据库的旧集成测试，不应被单个测试关闭。新测试优先使用独立 Server 和数据库。不要在 `t.Parallel` 中修改全局配置；需要全局运行时配置的 AI 测试仍串行运行。

禁止引入修改运行时指令的 mock 库、全局函数替换或禁用编译优化来维持测试。替身只应模拟外部服务，未配置的调用应立即失败。不要把真实 SQL 和持久化操作一起 mock 掉。

## 快速回归

普通编译优化即可运行，不需要额外 GCFLAGS 或环境绕过。使用临时 YAKIT_HOME，避免旧数据库影响集成测试：

```sh
export YAKIT_HOME="$(mktemp -d)"
./scripts/ci/check-test-dependencies.sh

go test ./common/yakgrpc -count=3 -shuffle=on -timeout=4m \
  -run '^(TestInMemoryClient|TestUploadPayloadToOnline|TestDownloadPayload|TestUploadHotPatchTemplateToOnline|TestDownloadHotPatchTemplate|TestProbeReasoningEffort.*|TestIsLikelyErrorResponse|TestUploadSyntaxFlowRule|TestDownloadSyntaxFlowRule|TestSyntaxFlowOnlineFailures|TestGetApiKey_ReplaceAPIKeys|TestDoHTTPFlowsToOnline|TestSSARiskFeedbackToOnline)$'

go test ./common/coreplugin -count=1 -timeout=6m \
  -run '^(TestLocalDNSLog|TestGRPCMUSTPASS_SSRF.*|TestGRPCMUSTPASS_Shiro.*|TestGRPCMUSTPASS_Fastjson.*)$'
```

竞态检测：给第一条 `go test` 增加 `-race`，并将 `-count` 改为 1。不要并行启动使用同一个 YAKIT_HOME 的独立测试进程。

传输启动开销：

```sh
go test ./common/yakgrpc -run '^$' -bench '^BenchmarkInMemoryClient$' -benchtime=20x -benchmem
```

该 benchmark 测量传输启动和关闭，不包括数据库迁移。旧辅助函数每次至少固定等待 1 秒；现在通过阻塞 dial 确认就绪，不再有这部分固定等待。整套集成测试还包括插件扫描时间，不能把这个收益当成全套耗时比例。

## CI

Essential Tests 复用已编译的测试二进制，新增独立的 deterministic boundaries 分组，不对这些测试重试；广泛回归分组跳过已分组的相同用例，避免重复执行。yakgrpc 与 coreplugin 的 prepared suites 设置 `SUITE_NEEDS_GRPC=0`，不再启动外部 yak grpc 或等待 ready 日志。需要同步规则的分组仍独立执行 `yak sync-rule`。

编译 yakgrpc 之前检查源文件、模块声明和两个包的测试依赖图，阻止运行时补丁库重新进入。原有完整扫描、MITM、Fuzzer、SSA、coreplugin 分组继续运行，IPC/CLI 启动测试继续覆盖实际进程和端口。

## 回归测试揭示的功能问题

- SyntaxFlow 下载原来忽略传入的数据库，写入全局 profile 库。保存现在使用调用方的库；表驱动测试检查新增、更新、跳过、冲突和 dirty 标记。
- SSA 风险反馈上传失败原来返回成功。现在返回错误并取消数据库生产者，调用方可以发现失败。
- coreplugin 流异常原来可能继续检查部分风险并让负向用例通过。现在非 EOF 错误直接使该次测试失败。
