---
name: yaklang-codegen
metadata:
  display_name_zh-CN: Yaklang 代码编写
description: >
  端到端编写 Yaklang 脚本：DSL 硬规则、脚本骨架、错误处理与 HTTP 模式，写码后用
  check_yaklang_syntax 语法检查，并用 bash 执行脚本完成 YAK_MAIN 自测闭环。当用户要求
  编写 yak 脚本、PoC、插件、扫描器或把需求实现为 Yaklang 代码时加载。
---

# Yaklang 代码编写 (yaklang-codegen)

在通用对话模式下复刻「Yaklang 代码生成」专注模式的写码闭环：写文件 → 语法检查 →
运行自测 → 失败修复 → 交付。Yaklang 是面向网络安全的 DSL，**语法借鉴 Go，但不是
Python / Go / JS**：无 import、无类，标准库全局可用。

## 0. 验收标准

| 脚本类型 | 完成条件 |
|---|---|
| 普通脚本 | `check_yaklang_syntax` 通过，且 `bash` 执行退出码为 0 |
| PoC / 插件 / MITM 等 | 语法通过，且 `YAK_MAIN` 自测运行通过（退出码 0、断言无失败） |

未运行验证的代码，**禁止声称“已完成/可用”**。

## 1. 工作流

```
1) 明确目标与 CLI 参数
2) 按骨架写脚本到 .yak 文件（write_file）
3) check_yaklang_syntax 语法检查（path）
4) bash 执行：普通脚本直接跑；PoC 类跑 YAK_MAIN 自测
5) 失败 → 读输出修复 → 重跑 3、4
6) 通过 → 交付
```

### 1.1 语法检查

`check_yaklang_syntax` 是 scenario 工具（默认清单不展示），但**可以直接调用**，参数本文已给出，
不需要 `require_tool` 预加载。**不要使用 `require_tool`**（弱模型容易传错 payload 导致任务中断）：

```json
{"@action": "directly_call_tool", "directly_call_tool_name": "check_yaklang_syntax", "directly_call_tool_params": {"path": "<脚本绝对路径>"}}
```

**禁止**用 SyntaxFlow 的检查工具校验 Yaklang 代码。

> `grep` / `read_file` / `write_file` / `bash` 都在默认工具清单中可见，直接调用即可，
> 不要用 `require_tool` 加载它们。

### 1.2 运行自测

用 `bash` 工具执行脚本（Windows 会按环境选择 bash/PowerShell）：

```json
{
  "@action": "directly_call_tool",
  "directly_call_tool_name": "bash",
  "directly_call_tool_params": {
    "command": "yak <脚本绝对路径>",
    "timeout": 120,
    "accepted-exit-codes": "0"
  }
}
```

若 `yak` 不在 PATH：先用 `bash` 执行 `where yak` / `which yak` 定位，或使用当前引擎二进制的
绝对路径替换命令中的 `yak`。

- PoC/插件类必须让脚本内 `runSelfTest()` 通过 `if YAK_MAIN` 触发；直接运行即可。
- 依赖外网/目标时，自测应使用 mock 数据，不要在自测里打真实目标。
- 超时按任务设置，避免无限等待。

### 1.3 修复原则

- 看退出码与 stderr，按错误类型修：语法错误改代码；运行时错误查 API 用法。
- **禁止盲目重写**：先定位到出错行/函数，做最小修改后重跑。
- 连续两轮修不好，回到 2 节骨架与 4 节陷阱逐条核对。

## 2. 脚本骨架

```
__DESC__ = "脚本描述"
__VERBOSE_NAME__ = "脚本名"
__KEYWORDS__ = ["keyword1", "keyword2"]

yakit.AutoInitYakit()

target = cli.String("target", cli.setHelp("扫描目标"), cli.setRequired(true))
mode = cli.String("mode", cli.setDefault("fast"), cli.setHelp("模式"))
cli.check()

// 业务逻辑...
```

- 元数据写在最前；`yakit.AutoInitYakit()` 先于一切。
- 用户可配置项一律暴露为 `cli` 参数（`setHelp`/`setRequired`/`setDefault`），最后 `cli.check()`。
- 不要就实现细节反问用户，选合理默认值并以 `cli` 参数暴露。
- 变量声明用 `x = 10` 或 `x := 10`。

## 3. 错误处理三板斧

1. 常规：`f, err := file.OpenFile(...); if err != nil { yakit.Error(...); return }`
2. 确定可忽略：`~` 抑制（如 `tls.GenerateRSA2048KeyPair()~`）。
3. 致命：`die(err)` 或 `try { ... } catch e { ... }`。

## 4. 硬规则与陷阱（写码前必读）

### DSL 硬规则

- **字符串**：禁止跨行 `"..." + "..."` 拼接；用单行、`\n` 或 `sprintf`。
- **byte 数组**：`[]byte{0xAC, 0xED, 0x00, 0x05}`；元素必须 byte 字面量（`0x..`），禁裸整数。
- **[]byte 拼接**：`append(a, b...)`；禁止 `append(a, b)`。追加单字节：`append(a, 0x70)`。
- **poc.Get / poc.Post**：必须三变量接收 `rsp, req, err := poc.Post(...)`。
- **PoC/插件脚本**：末尾加 `func runSelfTest(){...}` 与 `if YAK_MAIN { runSelfTest() }`。

### 三个"能编译但会崩"的陷阱

1. **map 取值没有 comma-ok**：`v, ok := m[key]` 不成立。用 `v = m[key]; if v == nil {...}`
   或 `if "key" in m {...}`。
2. **HTTP mock 报文的 `\r\n` 必须放在双引号字符串里**：反引号原始字符串不转义，
   `\r\n` 会按字面传给 poc，导致找不到 body。
3. **`if YAK_MAIN { runSelfTest() }` 要求 `func runSelfTest(){...}` 定义在同一文件**；
   不要用裸的 `defer recover()` 兜错。

## 5. HTTP 常用模式

- `poc.HTTP(packet)` / `poc.Get` / `poc.Post`：入参是 `[]byte` 原始报文；`poc.HTTP` 的
  返回值 `rsp` **本身就是 `[]byte`**（没有 `rsp.RawPacket`）。
- `poc.HTTPEx` 返回对象：`rspInst.RawPacket`、`rspInst.GetStatusCode()`。
- 典型链路：`poc.ParseUrlToHTTPRequestRaw(...)` → `poc.HTTPEx(packet, poc.https(isHttps))`
  → `poc.GetHTTPPacketBody(rsp.RawPacket)`。
- 其他常用：`fuzz.HTTPRequest`、`servicescan.Scan`、`synscan.Scan`。

## 6. API 纪律

- **不许编造 API**：不确定签名时，优先用 `grep` 在引擎安装目录/工作区搜索真实样例，
  或按第 5 节的已知模式使用；宁可少写也不要臆造。
- 自测素材运行时生成，不硬编码：`tls.GenerateRSA2048KeyPair()~`、
  `codec.GenerateSM2PrivateKeyPEM()~`、`codec.RandBytes(n)`。

## 7. 与专注模式的差异

| 能力 | 本技能（通用模式） | 专注模式 |
|---|---|---|
| 语法检查 | 你显式调用 `check_yaklang_syntax` | 写码后自动 lint |
| 运行自测 | 你用 `bash` 执行脚本 | 内置 `run_code` 执行并回灌失败原因 |
| API 文档检索 | grep 工作区/安装目录样例 | `yakdoc_*` 全套文档工具 |
| 样例搜索 | grep | 语义搜索 |
| 交付 | 写入普通文件 | 交付到前端代码编辑器 |

完整写码-运行-修错闭环在通用模式靠你按第 1 节流程严格执行；无法复刻的是系统的自动拦截。
