# Link prep (`link_prep`) 与运行时符号

`link_prep` 是 **独立于 LLVM obfuscator 管线** 的链接前阶段：当前用于在静态库（`libyak.a` 以及 obfuscation 产出的 `.a`）上 **重命名 C 链接可见的运行时符号**，使 LLVM IR 与归档里的实现使用同一套 `manifest`，从而在不改 Go 源码的前提下改变导出符号指纹。

它不是混淆器：不做 IR 变换，也不属于 `KindLLVM` obfuscator 体系。与 **ABI 常量**、**invokeconst** 等相关工作若落地，也应归在 link-prep / 链接策略里，而不是 obfuscation 包。

## Profile 字段

在 JSON profile 中：

```json
{
  "link_prep": {
    "randomize_runtime_symbols": true,
    "obfuscate_abi_constants": false
  }
}
```

- `randomize_runtime_symbols`：是否按构建种子生成 `rt_<hex>` 风格别名并改写归档。
- `obfuscate_abi_constants`：预留，当前未实现。

## 默认语义（与 `--profile` 的关系）

- **未加载 profile**（无 `--profile`）：保持历史行为，**不重命名**运行时符号（便于测试与快速编译）。
- **已加载 profile** 且 **省略 `link_prep` 整节**：视为需要与 profile 一致的加固策略，**默认开启**运行时符号随机化。
- **显式** `"link_prep": { "randomize_runtime_symbols": false }`：**关闭**随机化（稳定 `nm` / 调试）。
- 内置 **`resilience-lite` / `resilience-hybrid` / `resilience-max`** 以及 **`debug-stable-runtime`** 均带有 `link_prep.randomize_runtime_symbols: false`，保证稳定导出名。

编译器在应用 profile 后会构建 `RuntimeSymManifest`；LLVM 侧通过 `runtimeSymName` 使用映射后的名字；**链接前**只通过 **`linkprep.PrepareForLink`**（单一入口）处理归档；其内部再调用 `RewriteArchives`。编译缓存键会纳入 manifest（或 `rtSym=off`），避免错误复用。

## 工具链要求

`PrepareForLink` / `RewriteArchives` 依赖 PATH 中的：

- `llvm-ar` 或 `ar`
- `llvm-nm` 或 `nm`
- `llvm-objcopy` 或 `objcopy`

流程为：解包归档 → 对每个 `.o` 用 `nm -g` 列出符号（含未定义引用）→ 对 manifest 中在该对象内出现的符号拼 `--redefine-sym=old=new` → 调用 `objcopy` 写临时目标再覆盖 → 重新打包。

## Backlog

- CLI `profile list` / 内置 preset 表（当前以文档与 `profile.Names()` 为准）。

## 限制与注意事项

- 主要针对 **ELF** 类目标（Linux 等）。其他对象格式需自行验证 `objcopy` 行为。
- 重命名集合由 `linkprep.CanonicalRuntimeSymbols()` 固定枚举（ABI 符号 + `//export` 名）；用户 `.o` 里的 `yak_internal_atmain` 等不在该列表中（按设计仅处理 libyak / 相关归档导出）。
- 若工具缺失，预链接阶段会报错，而不是静默跳过。
- 每个 `.o` 都会对 manifest 中**在该对象里出现**的符号（含 **未定义的 `U`** 引用，如 C stub 调用 `yak_internal_release_shadow`）做 `--redefine-sym`，否则会出现定义已改名、调用方仍引用旧名而链接失败。
- 重写后的 `libyak.a` 往往位于临时目录、旁无 `runtime_go/`。`resolveRuntimeLinkArgs` 在无法从归档路径推断源码目录时，会回退到 `go env GOMOD` 定位模块根，再收集 `runtime_go` 的 CGO LDFLAGS；**在模块外调用 CLI** 时需保证 `go env GOMOD` 仍指向有效 `go.mod`，否则可能缺少 `-lpcap` 等依赖。
- 未使用 `--profile` 时，若嵌入 runtime 未命中，编译器会解析仓库内的 `libyak.a` 路径，以便 linkprep 与链接阶段使用**同一**归档路径（此前仅链接阶段隐式 `findRuntimeArchive`，会导致 IR 已随机化符号而归档未重写）。

## 与「三条轨道」的关系

见根目录 `readme.md` 中 **Profile、混淆与链接预处理**：构建标签 `ssa2llvm_runtime_debug`、profile 内 **obfuscators**、以及 **`link_prep`** 彼此正交，可单独开关。
