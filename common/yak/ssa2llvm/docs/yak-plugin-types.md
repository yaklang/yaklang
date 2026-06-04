# Yak 插件类型与原生 CLI 运行

本文档说明 `ssa2llvm` 如何把 Yakit 插件脚本编译为**可独立运行的原生二进制**，以及 `--plugin-type` 外壳、`cli` 与 `yakit` 在 AOT 模式下的行为边界。

## 1. 设计目标

在 Yakit UI 里，插件通常以几种固定形态出现：

| Yakit 类型 | `ssa2llvm` `--plugin-type` | 说明 |
|------------|---------------------------|------|
| 原生 Yak 脚本 | `yak`（默认） | 不注入外壳，脚本自带 `cli` / `main` 逻辑 |
| 编码/加解密插件 | `codec` | 注入 `__ssa2llvm_codec_main` 入口 |
| 端口扫描插件 | `port-scan` | 注入 `__ssa2llvm_portscan_main` 入口 |
| MITM 插件 | — | **尚未支持**，编译时报错 |

目标不是复刻整个 YakVM，而是让「带 `cli` + `handle` + `yakit.Code` 的插件」在命令行下能 **AOT 编译并跑通**。

实现位置：

- `common/yak/ssa2llvm/compiler/plugin_type.go`：外壳源码拼接与入口函数名
- `common/yak/ssa2llvm/compiler/api.go`：编译前 `wrapYakPluginSource`
- `common/yak/ssa2llvm/runtime/runtime_go/runtime_cli.go`：进程启动时注入 `os.Args` 给 `cli`
- `common/yak/ssa2llvm/runtime/runtime_go/runtime_yaklib.go`：`VirtualYakitClient`，把 `yakit.*` 打到 stdout

## 2. CLI 参数

编译后的二进制与 Yak 脚本一样，在启动时解析 `os.Args`：

- `cli.String` / `cli.Int` / `cli.Bool` / `cli.StringSlice` 等照常工作
- `cli.check()` 在校验失败时退出并打印 Usage
- 短选项需在脚本里显式声明，例如 `cli.setShortName("t")`；**不会**自动把 `-t` 映射到 `target`

传参方式：

```bash
# 直接运行已编译二进制
./out.bin --target /path/to/project --language php

# 或用 ssa2llvm run（`--` 之后传给二进制）
./ssa2llvm run plugin.yak --plugin-type yak -- --target /path --language php
```

## 3. 插件外壳行为

### 3.1 `yak`（默认）

不修改用户源码。适合：

- 自带 `main` 或顶层逻辑的 coreplugin（如「SSA 项目探测」）
- 任意需要完整脚本控制的工具

编译 coreplugin 示例：

```bash
./ssa2llvm compile \
  -o ./build/ssa-detect \
  "./common/coreplugin/base-yak-plugin/SSA 项目探测.yak" \
  --plugin-type yak

mkdir -p .db && export YAKIT_HOME="$PWD/.db"
./build/ssa-detect --target /path/to/project --language php
```

注意：

- 使用脚本里声明的长选项名（如 `--target`），不要假设未声明的短选项可用
- 建议设置独立的 `YAKIT_HOME`（worktree 内可用 `.db/`），避免与其它实例争用 SQLite

### 3.2 `codec`

用户只需提供：

```yak
handle = func(param) {
    return codec.EncodeBase64(param)  // 或其它 codec.* 
}
```

编译器自动包一层：

1. `cli.String("param", ...)` + `cli.check()`
2. `handle(__ssa2llvm_param)`
3. `println` 结果

示例：

```bash
./ssa2llvm compile -o codec.bin plugin.yak --plugin-type codec
./codec.bin --param yaklang
# 输出 Base64 等 handle 返回值
```

### 3.3 `port-scan`

用户只需提供：

```yak
handle = func(result) {
    // result 为一次「扫描结果」形状的对象
}
```

外壳用 CLI 拼出与 Yakit 端口扫描插件约定一致的结构体（`Target`、`Port`、`Fingerprint` 等），再调用 `handle`。

```bash
./ssa2llvm compile -o scan.bin plugin.yak --plugin-type port-scan
./scan.bin --target 127.0.0.1 --port 3306 --service mysql
```

**限制**：外壳**不会**自动发包或 SYN 扫描；若要在原生二进制里真扫端口，须在 `handle` 或脚本中调用 `synscan` / `poc` 等库。

## 4. yakit 输出

AOT 二进制内嵌 `VirtualYakitClient`（见 `runtime_yaklib.go`）：

| API | 路径 | 终端输出示例 |
|-----|------|----------------|
| `yakit.Info` / `Warn` / `Error` / `Debug` | 专用 `abi.FuncID` | `[yakit][info] ...` |
| `yakit.Code` | 通用 `yaklib` 派发 | `[code] {...}` |
| `yakit.AutoInitYakit()` | yaklib | 初始化虚拟客户端 |

`yakit.Code` 用于向 Yakit UI 回传 JSON；在 CLI 下会打印可读的 `[code]` 行，而不是 ExecResult 原始结构。

测试参考：

- `tests/runtime_operators_test.go`：`TestRuntimeOperator_YakitCodePrintsReadableTerminalOutput`
- `tests/yaklib_ssa_exports_test.go`：map / side-effect / `ssa.NewConfig` + `yakit.Code` 系列
- `tests/coreplugin_compile_test.go`：`TestCorePlugin_RunSSADetectProject`

## 5. 与 YakVM 的差异（使用边界）

- **可以**：编已知模式的脚本、codec/port-scan 外壳插件、部分 coreplugin、`ssa.*` / `codec.*` 等已接好的 yaklib
- **不可以**：假定任意 Yakit 插件无需修改即可编译；**mitm** 类型；完整替代 YakVM 语义
- **值表示**：仍以 `i64` + runtime 对象为主；复杂 map/成员/side-effect 在持续补强中
- **平台**：主路径为 **Linux**；Windows 上部分链接与测试会跳过

## 6. 测试

```bash
mkdir -p .db && export YAKIT_HOME="$PWD/.db"
./common/yak/ssa2llvm/scripts/build_runtime_go.sh

# 插件类型
go test ./common/yak/ssa2llvm/tests/ -run TestYakPluginType -count=1 -v

# yakit.Code / map / side-effect
go test ./common/yak/ssa2llvm/tests/ -run 'TestYaklibSSA_|TestRuntimeOperator_Yakit' -count=1

# coreplugin 探测（需 --target，非未声明的短选项）
go test ./common/yak/ssa2llvm/tests/ -run TestCorePlugin_RunSSADetect -count=1 -v

# 全量（约 10 分钟，建议加长超时）
go test ./common/yak/ssa2llvm/... -count=1 -timeout=30m
```
