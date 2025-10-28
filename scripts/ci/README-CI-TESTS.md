# CI 脚本更新说明

## 概述

根据 `essential-tests.yml` 的矩阵配置更新：
- 所有测试配置（`timeout`、`parallel`、`run`、`skip` 等）现在都通过 `test_configs` 控制
- **gRPC 服务默认启动**：所有测试任务都会自动启动 gRPC 服务
- 只保留 `sync_rule: "1"` 用于控制是否同步 SyntaxFlow 规则

## 主要变更

### 1. 编译脚本 (`compile-tests.sh`)

**变更内容：**
- 添加了去重机制，避免重复编译相同的包
- 使用关联数组 `SEEN_PKGS` 跟踪已处理的包
- 即使 `test_config` 中有重叠的包路径模式（如 `./common/ai/...` 和 `./common/ai/aid/...`），每个包也只会编译一次

**关键代码：**
```bash
declare -A SEEN_PKGS  # 使用关联数组去重
PKGS=()

for dir in "${CONFIG_DIRS[@]}"; do
  while IFS= read -r pkg; do
    if [[ -n "$pkg" && -z "${SEEN_PKGS[$pkg]:-}" ]]; then
      SEEN_PKGS["$pkg"]=1
      PKGS+=("$pkg")
    fi
  done < <(go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' "$dir" 2>/dev/null || true)
done
```

### 2. 运行脚本 (`run-tests.sh`)

**变更内容：**
- 添加了 `parallel` 参数的支持，从 `test_config` 中读取
- 修改了参数传递逻辑：只有在配置中明确设置的参数才会传递给测试二进制
- 如果配置中没有设置 `parallel`、`run`、`skip` 等参数，就不会向测试二进制传递这些参数

**关键变更：**

1. **run_test 函数签名更新：**
```bash
run_test() {
  local bin="$1"
  local pkg_path="$2"
  local timeout="$3"
  local run_pattern="$4"
  local skip_pattern="$5"
  local parallel="$6"        # 新增参数
  local config_source="$7"   # 从第6个变成第7个
```

2. **参数构建逻辑：**
```bash
local args=( "-test.timeout=$timeout" )
[[ -n "$parallel" ]] && args+=("-test.parallel=$parallel")     # 只有非空才添加
[[ "$TEST_VERBOSE" = "1" ]] && args+=("-test.v")
[[ -n "$run_pattern" ]] && args+=("-test.run=$run_pattern")    # 只有非空才添加
[[ -n "$skip_pattern" ]] && args+=("-test.skip=$skip_pattern") # 只有非空才添加
```

3. **配置读取：**
```bash
for ((idx=0; idx<config_count; idx++)); do
  pattern=$(jq -r ".[$idx].package" "$TEST_CONFIG")
  timeout=$(jq -r ".[$idx].timeout // empty" "$TEST_CONFIG")
  run_pattern=$(jq -r ".[$idx].run // empty" "$TEST_CONFIG")
  skip_pattern=$(jq -r ".[$idx].skip // empty" "$TEST_CONFIG")
  parallel=$(jq -r ".[$idx].parallel // empty" "$TEST_CONFIG")  # 新增
  
  # 使用默认值填充空配置
  [[ -z "$timeout" ]] && timeout="$TEST_TIMEOUT"
  # 注意：run_pattern、skip_pattern、parallel 如果配置中没设置，就保持空值
```

### 3. Workflow 文件 (`essential-tests.yml`)

**变更内容：**
- 移除了矩阵级别的所有测试配置参数（`filter`、`timeout`、`parallel`、`run`、`skip`）
- 只保留默认的 `TEST_TIMEOUT` 环境变量
- 所有测试配置现在都从 `test_configs` JSON 读取

**变更前：**
```yaml
- name: ${{ matrix.name }}
  run: |
    export PACKAGE_FILTER_REGEX="${{ matrix.filter }}"
    export TEST_TIMEOUT="${{ matrix.timeout || '2m' }}"
    export TEST_PARALLEL="${{ matrix.parallel || '1' }}"
    export TEST_RUN_PATTERN="${{ matrix.run || '' }}"
    export TEST_SKIP_PATTERN="${{ matrix.skip || '' }}"
    export TEST_CONFIG="/tmp/test_config.json"
    ./scripts/ci/run-tests.sh
```

**变更后：**
```yaml
- name: ${{ matrix.name }}
  run: |
    export TEST_TIMEOUT="2m"
    export TEST_VERBOSE="1"
    export TEST_CONFIG="/tmp/test_config.json"
    ./scripts/ci/run-tests.sh
```

## 配置示例

在 `essential-tests.yml` 的矩阵配置中，测试配置示例：

```yaml
test_configs: |
  [
    {"package": "./common/utils/omap/...", "timeout": "30s", "race": true},
    {"package": "./common/utils", "timeout": "1m"},
    {"package": "./common/ai/aid/...", "timeout": "12m", "parallel": 1},
    {"package": "./common/yakgrpc/...", "timeout": "5m", "run": "TestGRPCMUSTPASS_MITM_*"},
    {"package": "./common/yakgrpc/...", "timeout": "10m", "skip": "^(TestGRPCMUSTPASS_PluginTrace*|TestGRPCMUSTPASS_AnalyzeHTTPFlow*)"}
  ]
```

## 参数说明

### test_config JSON 支持的参数：

| 参数 | 类型 | 说明 | 是否必需 | 默认值 |
|------|------|------|----------|--------|
| `package` | string | 包路径模式 | 是 | - |
| `timeout` | string | 测试超时时间 | 否 | `2m` |
| `parallel` | number | 测试并发数 | 否 | 不设置 |
| `run` | string | 测试过滤模式 (test.run) | 否 | 不设置 |
| `skip` | string | 测试跳过模式 (test.skip) | 否 | 不设置 |
| `race` | boolean | 是否启用 race 检测 | 否 | `false` |
| `retry` | number | 失败时重试次数 | 否 | `0` |
| `retry_delay` | number | 重试延迟（秒） | 否 | `5` |

### 注意事项：

1. **timeout**: 如果不设置，使用全局默认值 `2m`
2. **parallel/run/skip**: 如果不设置，不会向测试二进制传递这些参数
3. **race**: 只影响编译阶段，会为匹配的包添加 `-race` 编译标志
4. **retry**: 测试失败时的重试次数，默认不重试
5. **retry_delay**: 重试之间的延迟秒数，默认 5 秒

## 重试机制

### 使用场景

某些测试可能因为网络波动、资源竞争等临时问题而失败。重试机制可以提高测试的稳定性。

### 配置示例

```json
{
  "package": "./common/utils/lowhttp",
  "timeout": "3m",
  "skip": "TestComputeDigestResponseFromRequest|TestComputeDigestResponseFromRequestEx|TestLowhttpResponse2",
  "retry": 2,
  "retry_delay": 5
}
```

这个配置表示：
- 如果测试失败，最多重试 2 次（总共会执行 3 次）
- 每次重试前等待 5 秒
- 如果任何一次尝试成功，立即停止并标记为通过
- 所有尝试都失败后，才标记为失败

### 运行日志示例

**首次失败时：**
```
FAIL: test_common_utils_lowhttp (exit=1, attempt=1/3)
失败日志摘要：
  --- FAIL: TestLowhttpResponse (0.01s)
  TLS handshake error
```

**重试时：**
```
⚠️  重试测试 (尝试 2/3): test_common_utils_lowhttp
Command: (cd ./common/utils/lowhttp && /tmp/test_binaries/test_common_utils_lowhttp -test.timeout=3m -test.v -test.skip=TestCompute...)
Retry: enabled (max=2, delay=5s, attempt=2)
```

**重试成功时：**
```
PASS: test_common_utils_lowhttp
✅ 重试成功！(在第 2 次尝试)
```

### 最佳实践

1. **谨慎使用**：重试会延长 CI 时间，只对真正不稳定的测试使用
2. **合理设置次数**：通常 1-2 次重试足够，过多重试可能掩盖真实问题
3. **适当延迟**：给系统足够时间恢复，5-10 秒通常合适
4. **监控重试率**：如果某个测试频繁需要重试才能通过，应该修复测试本身

## 优势

1. **避免重复编译**：即使配置中有重叠的包路径，每个包只编译一次
2. **精确控制参数**：只有明确配置的参数才会传递给测试
3. **配置集中管理**：所有测试配置在 `test_configs` 中统一管理
4. **减少环境变量依赖**：矩阵级别不再需要设置各种测试参数
5. **更清晰的日志**：运行时会显示每个配置规则使用的参数
6. **智能重试机制**：支持针对不稳定测试的自动重试，提高 CI 稳定性
7. **简化配置**：gRPC 服务默认启动，无需为每个测试单独配置

