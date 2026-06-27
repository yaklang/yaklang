# SSA API Discovery（专注模式）

## 作用

- 校验用户给出的**代码根目录**与**靶机**（HTTP(S) 或 `host:port`）；靶机不可达时**记录并继续**。
- 对项目执行 **SSA 编译**（`ssa_api_discovery` 的 InitTask 内联调用）。
- 通过 `discovery_*` 系列 action 将架构、配置、依赖、HTTP 端点、安全机制、业务能力等写入 **`workdir/ssa_discovery/session.sqlite3`**（独立 SQLite，不混入 Yakit 主库）。

## 用户输入建议格式

```text
Code path: /abs/path/to/project
Target: http://127.0.0.1:8080
Language: java
```

`Language` 可选；不填则根据 `pom.xml`、`go.mod` 等自动探测。

## 本地测试

```bash
go test ./common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/... -count=1
```

SSA 全量编译（Spring 样例，较慢、可能拉 Maven 依赖）：

```bash
go test ./common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/... -tags=ssa_discovery_integration -count=1 -timeout=15m
```

样例工程：`testfixtures/minimal_java_webapp/`（`mvn -q test-compile` 应能通过）。

外部 benchmark 极小 Java 样例可参考：`/home/murkfox/yak-ssa-api-discovery/benchmark-repos/flag-harness/spring-web-flags`（无 `pom` 时仅适合手工 SSA 冒烟）。

## 注册名

- 常量：`schema.AI_REACT_LOOP_NAME_SSA_API_DISCOVERY`（`ssa_api_discovery`）
- 需在应用中 `_` import `reactloops/reactinit` 以完成副作用注册。
