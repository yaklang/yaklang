# Branch: feature/ssa2llvm/support_ci_and_mustpass_test

> 分支状态与待办清单（2026-08-25 更新）。换机器继续时先读本文件。

## 目标

让 `common/yak/yaktest/mustpass/files/*.yak`（115 个脚本）全部通过 ssa2llvm AOT 编译 + 原生二进制运行（exit 0、无 panic），并保证 CI + mustpass 测试通过。

## 环境要求（重要）

- **Go 工具链**：必须用 **go 1.26.6**（系统 go 1.27.0 会破坏 elfsplit 的 moduledata 布局，报 `textsectmap len/cap mismatch`）。已下载到 `~/go1.26.6/`，构建时：

  ```sh
  export PATH=/home/wlz/go1.26.6/bin:$PATH GOTOOLCHAIN=local
  ```

- **GOWORK**：`GOWORK=/home/wlz/Developer/yaklang-workspace/feature-ssa2llvm-support_ci_and_mustpass_test/build/ssa2llvm-go.work`（replace go-llvm 到本地 checkout）
- **gcc 16 ICE 绕过**：构建 CLI 时若 pcre2 触发 gcc internal compiler error，加 `CGO_CFLAGS="-O1"`（core tier 通常不需要）
- **构建 CLI**：`bash common/yak/ssa2llvm/scripts/build_cli.sh -o build/ssa2llvm`（覆盖式，不要留版本号）
- **YAKIT_HOME**：跑脚本时用独立目录（如 `/tmp/ssa2llvm-check/yakit`）避免 DB 锁

## 已完成（已 push 的 commit 见 git log）

### 环境/基础设施

- go 1.26.6 工具链适配（moduledata 偏移）
- Boehm GC 自动回收禁用（shadow 被过早回收导致随机失败）→ `yak_runtime_gc` 显式 enable/collect/disable
- lld 崩溃修复：`//export` 文件缺 `import "C"` 导致符号丢失（runtime_globals_aot.go）

### 语言语义修复（grammar_test.yak 全量通过）

- OpEq 常量折叠（int vs int64 类型不匹配 → 6==6 折叠成 false）
- float() 类型区分（CreateFloatType，int()/float() 转换方向）
- 类型转换 runtime 函数（to_int/to_float/to_string/bool_to_string/parse_int/parse_float）
- 负索引 key 符号（to_cstring 保留负数）、字符串索引（resolveField string 分支）
- slice 表达式（slice-of-slice 窗口复制 + `a[::-1]` 负步长反转）
- makeInitialMemberCount 过滤 Undefined/phi 占位（a[-1] 读取不膨胀长度）
- buildSliceCall 的 `[a:b]` 定位（colon 计数）
- ellipsis 展开（`b(a...)` 正序 + 过滤读取占位）
- map 闭包调用（`a["c"](1)` 走动态调用而非 method dispatch）
- break 循环 phi 写回（emitLoopExitPhiWriteback）
- float 算术（1.0 * 2 走 runtime float binop，2.0 位模式 tag-bit 解码）

### fuzztag 模板（x\`...\`）

- 内联 fuzztag 引擎（fuzztag_rt.go）：trim/substr/gb18030/gb18030toUTF8/hexd/crlf/list/list:comma/list:auto/int，嵌套 {{...}} 笛卡尔组合
- 不用 common/mutate（其 consts 依赖污染 runtime init 图导致崩溃）

### 其他

- VULINBOX/VULINBOX_HOST 编译期注入（extern value，mustpass_all_test 用 t.Setenv）
- string/bytes 按内容比较（runtimeValuesEqual）
- 闭包 slice 写回（支配检查 + 变量名匹配 + 懒解析 + entry alloca 中转）
- mustpass_simple/ 回归套件（5 个最小用例 + mustpass_simple_test.go）

## 当前状态

- 全量 mustpass：**55/115 通过**（旧 CLI 数据；新 CLI 下 misc/fuzztag_test/eval/grammar_test 已单独验证通过，实际更高，需重跑确认）
- 回归测试（mustpass_simple + closure + DualRun + ZeroDep）全绿

## 剩余工作（60 个失败脚本，按优先级）

### P0-P2（runtime 语义，可继续修）

- `db_query_plugin.yak`：Test 1.3（float ID 查询）—— float binop 已修，需重验
- `expect_100_continue.yak`、`fuzz-http-request-value.yak`、`fuzz_mutate_post_json_params.yak`、`git_test.yak`、`git_to_sca.yak`、`githack_test.yak`、`java-decompiler.yak`、`jsonschema_builder.yak`、`noautodecode-fuzz.yak`、`poc_no_redirect.yak`、`suricata_match.yak`、`tlsinspect.yak`、`yaklang_programming_complex.yak` 等（exit 255，部分输出后失败）
- `dictutil.yak`（挂起 timeout）
- `fuzz_json_params_no_escape_html.yak`（回归？之前通过，需确认）

### P3（SIGSEGV，约 35 个）

- `jwt`/`jwt_order`/`mixcaller*`/`crawlerx`/`http_lowhttp`/`mitm_*`/`nuclei_*`/`mock_*`/`portscan`/`syntaxflow`/`poc_params_fuzz`/`risk_*`/`browser_brute`/`build_kb_from_file`/`head_chunked_test`/`hook_load_plugin_by_id`/`httpserver_allbasic`/`lowhttp_poc`/`plugin_inherit_proxy`/`rag_question_index`/`yakpoc-cookie-and-ua`/`zip` 等
- 主要模式：调用帧/指针表示问题（crypto/hmac 函数表间接调用崩溃、小地址对象指针错解引用）

### P4（网络/复杂模块）

- `mitm_*`（10 个）、`udp*`（3 个，含 lld 链接崩溃）、`waitAllAsyncCallFinish*`（lld 崩溃）、`rag*`、`omnisearch`、`sandbox`、`nuclei_network*`、`poc_download`（编译失败）等

## 如何继续

1. 重跑全量 mustpass 拿准确基线：

   ```sh
   GOWORK=.../build/ssa2llvm-go.work go test ./common/yak/ssa2llvm/tests/ -run TestMustPass_SSA2LLVM_AllScripts -v -timeout 3h
   ```

2. 修 P0-P2 的 exit 255 类（逐个脚本，用 `build/ssa2llvm run <script> -f main -a` 复现）
3. 每个修复补 mustpass_simple/ 回归用例（用户要求）
4. 最后攻 P3 SIGSEGV（调用帧/指针表示）
5. 全部完成后跑 ssa-test.sh 相关全量 + push

## 注意事项

- 不要用 `go test` 直接跑（AGENTS.md 要求 `scripts/ssa-test.sh`）
- 不要清理 GOCACHE（构建很慢）
- 构建 CLI 覆盖 `build/ssa2llvm`，不要留版本号
- 提交前检查无 `Co-authored-by:` 行
