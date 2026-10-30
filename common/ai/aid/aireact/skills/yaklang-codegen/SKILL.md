---
name: yaklang-codegen
metadata:
  display_name_zh-CN: Yaklang 代码编写
description: >
  Yaklang 脚本编写技能：DSL 硬规则（字符串、byte 数组、append、poc 三返回值、YAK_MAIN
  自测）、脚本骨架、错误处理三板斧与 HTTP 常用模式。用于在通用对话模式下编写正确的
  Yaklang 脚本；需要写码-运行-修错完整闭环时请使用「Yaklang 代码生成」专注模式。
---

# Yaklang 代码编写 (yaklang-codegen)

编写 Yaklang（Yak）脚本时加载本技能。Yaklang 是一门面向网络安全的 DSL，**语法借鉴 Go，
但不是 Python / Go / JS**：没有 import、没有类，标准库全局可用。以下规则来自大量真实
踩坑，违反通常**能编译但运行时崩溃**。

## 1. DSL 硬规则（写码前必读）

- **字符串**：禁止跨行 `"..." + "..."` 拼接；用单行、`\n` 或 `sprintf`。
- **byte 数组**：`[]byte{0xAC, 0xED, 0x00, 0x05}`；元素必须是 byte 字面量（`0x..`），
  禁止裸整数。
- **[]byte 拼接**：`append(a, b...)`；**禁止** `append(a, b)`（整段 bytes 不能当单个
  元素）。追加单字节：`append(a, 0x70)`。
- **poc.Get / poc.Post**：必须三变量接收 `rsp, req, err := poc.Post(...)`。
- **PoC/插件脚本**：末尾加 `func runSelfTest(){...}` 与 `if YAK_MAIN { runSelfTest() }`。

## 2. 三个"能编译但会崩"的陷阱

1. **map 取值没有 comma-ok**：`v, ok := m[key]` 在 Yaklang 中不成立。用
   `v = m[key]; if v == nil {...}` 或 `if "key" in m {...}`。
2. **HTTP mock 报文的 `\r\n` 必须放在双引号字符串里**：反引号原始字符串不转义，
   `\r\n` 会按字面传给 poc，导致找不到 body。
3. **`if YAK_MAIN { runSelfTest() }` 要求 `func runSelfTest(){...}` 定义在同一文件**；
   不要用裸的 `defer recover()` 兜错。

## 3. 脚本骨架

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

- 元数据（`__DESC__` / `__VERBOSE_NAME__` / `__KEYWORDS__`）写在最前。
- `yakit.AutoInitYakit()` 先于一切。
- 用户可配置项一律暴露为 `cli` 参数（`setHelp` / `setRequired` / `setDefault`），最后
  `cli.check()`；不要就实现细节反问用户，选合理默认值。
- 变量声明用 `x = 10` 或 `x := 10`；无 import、无类。

## 4. 错误处理三板斧

1. 常规：`f, err := file.OpenFile(...); if err != nil { yakit.Error(...); return }`
2. 确定可忽略时：`~` 抑制（如 `tls.GenerateRSA2048KeyPair()~`）。
3. 致命：`die(err)` 或 `try { ... } catch e { ... }`。

## 5. HTTP 常用模式

- `poc.HTTP(packet)` / `poc.Get` / `poc.Post`：入参是 `[]byte` 原始报文；`poc.HTTP` 的
  返回值 `rsp` **本身就是 `[]byte`**（没有 `rsp.RawPacket`）。
- `poc.HTTPEx` 返回对象：`rspInst.RawPacket`、`rspInst.GetStatusCode()`。
- 典型链路：`poc.ParseUrlToHTTPRequestRaw(...)` 构造报文 → `poc.HTTPEx(packet, poc.https(isHttps))`
  发送 → `poc.GetHTTPPacketBody(rsp.RawPacket)` 取 body。
- 其他常用：`fuzz.HTTPRequest`、`servicescan.Scan`、`synscan.Scan`。

## 6. 自测约定（PoC / MITM / 插件类脚本）

末尾必须附自测并通过 `YAK_MAIN` 触发；自测用的 mock 数据（req/rsp）与断言写在
`runSelfTest` 内；密钥等随机素材**运行时生成**，不要硬编码：
`tls.GenerateRSA2048KeyPair()~`、`codec.GenerateSM2PrivateKeyPEM()~`、`codec.RandBytes(n)`。

## 7. API 纪律与校验

- **不许编造 API**：不确定函数签名时，宁可少写也不要臆造；按第 5 节的已知模式使用。
- 写完后用 `check_yaklang_syntax` 做语法校验（该工具默认不在工具清单中展示，通过
  `require_tool` 加载后调用）；**禁止**用 SyntaxFlow 规则检查工具校验 Yaklang 代码。

## 8. 边界说明

本技能覆盖通用对话模式下的脚本编写与语法自查。「Yaklang 代码生成」专注模式提供完整
闭环：yakdoc API 文档检索、样例代码搜索、运行自测（YAK_MAIN）与编译错误分类修复。
长脚本、复杂工具开发建议切换到专注模式。
