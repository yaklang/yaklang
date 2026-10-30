# 运行手册

## 前置条件

在仓库根目录运行，使用隔离环境分析恶意流量。

```sh
go build -o /tmp/pcap-inspect ./common/pcapx/cmd/pcap-inspect
go run ./internal/trafficfixture/cmd/corpus verify
```

捕获、keylog、manifest 和 `TASKS.md` 由 `internal/trafficfixture` 从
hash-pinned corpus ZIP 直接读取。Go 测试不需要解压；本地已有展开文件时
可以直接使用文件路径。只有归档时可使用 `corpus exec --` 和 `@仓库路径`
传给需要文件名的命令。ZIP 本身不代替 manifest 中的来源 hash 校验。

## 推荐顺序

1. `http-dga-c2`：HTTP、URL 参数、POST。
2. `dns-tunnel-c2`：TXT、分片和危险文本边界。
3. `lateral-movement-psexec`：SMB/RPC 关联。
4. `host-user-identification`：实体提取。
5. `redline-infostealer-2023-07`：跨协议时间线。
6. `https-keylog-dridex`：无 keylog 与有 keylog 对比。

## 运行后检查

- 只分析本地文件，没有外部网络请求。
- 没有输出完整密钥、凭据和恶意脚本。
- 失败页没有被当成空结果。
- 截断、缺口和未检查范围被明确记录。
- 结论有 MessageID、SessionID、StreamID 或 PacketID。

## 当前 AI 工具边界

- `pcaputil` 支持部分 TLS/QUIC keylog 解密。
- `pcap-inspect` 有 `-tls-keylog`。
- `pcapdb` 当前没有 keylog 导入选项。
- `load_pcapfile` 和 `query_pcapdb` 没有 keylog 参数。

因此 `https-keylog-dridex` 的即时重放可测；AI 导入并持久化查询解密后的 HTTPS 当前应标记为不支持。

## 基础回归

```sh
go test -p 2 ./common/ai/aid/aitool/buildinaitools/yakscripttools/test \
  -run '^TestPCAPAnalysisTools$' -count=1 -timeout 5m
```

该测试不覆盖本目录的全部真实调查任务。

## HTTP 路由回归

```sh
go test -p 2 ./common/ai/aid/aitool/buildinaitools/yakscripttools/test \
  -run '^(TestReadFile_PCAPRoutesToStructuredWorkflow|TestPCAPHTTPDGAFixtureUsesStructuredWorkflow)$' \
  -count=1 -timeout 5m
```

该回归覆盖两件事：

- `.pcap/.pcapng` 只走 `load_pcapfile + query_pcapdb`；旧 `analyze_pcap` 已退役并从内置工具目录移除，不保留兼容入口。
- `http-dga-c2` 能通过结构化工具返回 Host、Method、Path、状态码、MessageID 和 4 组 `response_to` 关联。
