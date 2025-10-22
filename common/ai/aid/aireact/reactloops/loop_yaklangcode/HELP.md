# Yaklang AI 代码生成优化指南

## 问题诊断

### 核心问题
AI Agent 在编写 Yaklang 代码时经常出现以下问题：
1. **擅自臆造代码**：不基于实际样例，凭空想象 API 用法
2. **不使用搜索工具**：有 `query_document` 工具却不调用
3. **不符合 DSL 基调**：Yaklang 是 DSL，不是通用语言，必须基于样例编写

### 根本原因
1. **工具命名问题**：`query_document` 太抽象，AI 理解为"查文档"而非"grep 代码样例"
2. **工具功能单一**：缺少专门的快速 grep 工具
3. **Prompt 哲学缺失**：没有强调"以暗猜接口为耻，以认真查阅为荣"的核心理念
4. **缺少搜索优先原则**：没有明确要求"先搜索再编写"

---

## 解决方案

### 1. 新增 grep_yaklang_samples 工具（推荐方案）

#### 解决思路
**不修改现有工具，而是新增一个专门的 grep 工具**

保留现有的 `query_document`（查询完整文档），新增 `grep_yaklang_samples`（快速 grep 代码样例）。

#### 新增工具设计

**工具名称**：`grep_yaklang_samples`

**工具定位**：快速 grep 代码样例库，直接搜索真实代码

**核心参数**：
- `pattern` - 搜索模式（支持正则表达式和关键词）
- `case_sensitive` - 是否区分大小写（默认 false）
- `context_lines` - 上下文行数（默认 15 行，可调整）

**与 query_document 的区别**：
| 特性 | grep_yaklang_samples | query_document |
|------|---------------------|----------------|
| 定位 | 快速 grep 代码样例 | 查询完整文档 |
| 速度 | 快 | 相对较慢 |
| 返回 | 匹配的代码片段 + 上下文 | 结构化的文档说明 |
| 使用场景 | API 错误、快速找用法 | 学习新库、深入理解 |
| 优先级 | 首选 | 备选 |

#### 命名理由
- `grep` - 程序员的本能词汇，看到就知道是"搜索代码"
- `yaklang` - 明确是 Yaklang 相关内容
- `samples` - 强调是"代码样例"而非抽象文档
- AI 看到 `grep_yaklang_samples` 会自然联想到"grep 代码找例子"
- 符合 Unix 哲学，直观易懂

---

### 2. grep_yaklang_samples 工具描述

#### 推荐描述
```
🔍 Grep Yaklang 代码样例库 - 快速搜索真实代码示例

⚠️ 核心原则：禁止臆造 Yaklang API！必须先 grep 搜索真实样例！

【强制使用场景】：
1. 编写任何代码前，先 grep 相关函数用法
2. 遇到 API 错误（ExternLib don't has）时
3. 遇到语法错误（SyntaxError）时
4. 不确定函数参数或返回值时

【参数说明】：
- pattern (必需) - 搜索模式，支持：
  * 关键词：如 "端口扫描", "HTTP请求"
  * 正则：如 "servicescan\\.Scan", "poc\\..*"
  * 函数名：如 "str.Split", "yakit.Info"
  
- case_sensitive (可选) - 是否区分大小写，默认 false

- context_lines (可选) - 上下文行数，默认 15
  * 需要更多上下文：设置 20-30
  * 只看函数调用：设置 5-10
  * 看完整实现：设置 30-50

【使用示例】：
pattern="servicescan\\.Scan", context_lines=20  // 搜索端口扫描用法
pattern="die\\(err\\)", context_lines=10        // 搜索错误处理
pattern="端口扫描|服务扫描", context_lines=25     // 搜索相关功能

记住：Yaklang 是 DSL！每个 API 都可能与 Python/Go 不同！
先 grep 找样例，再写代码，节省 90% 调试时间！
```

#### query_document 保持不变
`query_document` 工具保持原有功能，用于查询完整的库文档和函数说明。当需要深入了解某个库的所有功能时使用。

---

### 3. Prompt 系统优化

#### 3.1 添加"八荣八耻"强化版

在 `persistent_instruction.txt` 开头添加：

```markdown
## ⚠️ Yaklang 代码生成铁律 ⚠️

### 八荣八耻 - 必须铭记于心

以暗猜接口为耻，以认真查阅为荣
以模糊执行为耻，以寻求确认为荣
以盲想业务为耻，以人类确认为荣
以创造接口为耻，以复用现有为荣
以跳过验证为耻，以主动测试为荣
以破坏架构为耻，以遵循规范为荣
以假装理解为耻，以诚实无知为荣
以盲目修改为耻，以谨慎重构为荣

### 具体执行准则

1. **禁止臆造代码**
   ❌ 看到需求就直接写代码
   ✅ 先 grep_yaklang_samples 搜索相关函数

2. **禁止猜测 API**
   ❌ "我觉得应该是 synscan.timeout(...)"
   ✅ "让我先 grep 'synscan' 看看有哪些选项"

3. **禁止重复试错**
   ❌ 连续 3 次 modify_code 尝试不同的 API 名称
   ✅ 第一次失败立即 grep 搜索正确用法

4. **强制搜索场景**（必须先 grep）
   - 编写任何新功能前
   - 遇到任何 linter 错误后
   - 使用不熟悉的库时
   - 看到"ExternLib don't has"错误时

### 工作流程（强制执行）

```
用户需求
  ↓
【步骤1】grep_yaklang_samples 搜索相关功能样例
  ↓
【步骤2】基于搜索结果理解正确用法
  ↓
【步骤3】write_code 编写初始代码
  ↓
【步骤4】如果有错误 → 返回步骤1 grep 搜索
  ↓
【步骤5】modify_code 修正（基于搜索结果）
```

❌ 错误流程：需求 → 写代码 → 报错 → 猜测修改 → 报错 → 再猜测...
✅ 正确流程：需求 → grep 搜索 → 写代码 → 报错 → grep 搜索 → 精确修改
```

#### 3.2 修改 `reactive_data.txt` 错误提示部分

在错误提示区域（FeedbackMessages）后添加：

```markdown
{{ if .FeedbackMessages }}
## 反馈/警告/待评估

针对上述代码，经过Yaklang编译器静态分析发现如下警告和错误：

<|ERR/LINT_WARNING_{{ .Nonce }}|>
{{ .FeedbackMessages }}
<|ERR/LINT_WARNING_END_{{ .Nonce }}|>

### ⚠️ 强制行动指令 - 禁止猜测！

**如果你看到以下任何错误类型，必须立即调用 grep_yaklang_samples：**

1. **ExternLib [...] don't has [...]** 
   → 说明你猜错了 API 名称
   → 必须 grep 该库的正确用法
   → 示例：grep lib_names=["synscan"] 或 regexp=["synscan\\.\\w+"]

2. **SyntaxError** 
   → 说明你的语法不符合 Yaklang DSL
   → 必须 grep 类似功能的代码样例
   → 示例：grep keywords=["错误处理", "error handling"]

3. **undefined variable/function**
   → 说明你使用了不存在的符号
   → 必须 grep 正确的函数名
   → 示例：grep lib_function_globs=["*Scan*"]

4. **type mismatch**
   → 说明你对参数类型理解错误
   → 必须 grep 该函数的正确用法
   → 示例：grep regexp=["servicescan\\.Scan.*opts"]

**禁止的错误行为：**
❌ 看到 `synscan.timeout` 不存在 → 尝试 `synscan.set_timeout`
❌ 看到 `synscan.set_timeout` 不存在 → 尝试 `synscan.withTimeout`
❌ 继续猜测 `synscan.setTimeout`, `synscan.timeoutOption`...

**正确的行为：**
✅ 看到 `synscan.timeout` 不存在 → 立即 grep_yaklang_samples
   ```json
   {"@action": "grep_yaklang_samples", 
    "keywords": ["synscan"], 
    "regexp": ["synscan\\.\\w+"],
    "human_readable_thought": "synscan.timeout 不存在，我需要搜索 synscan 库的所有可用选项"}
   ```

记住：搜索 1 次 = 节省 10 次错误尝试！
{{ end }}
```

---

### 4. 改进 reflection_output_example.txt

添加强制搜索的示例：

```markdown
### ❌ 错误示例：盲目猜测 API

**场景**：需要实现端口扫描超时设置

```json
{"@action": "write_code", "human_readable_thought": "我觉得应该用 servicescan.timeout() 来设置超时"}
```

<|GEN_CODE_xxx|>
results, err := servicescan.Scan(target, ports, servicescan.timeout(5))
<|GEN_CODE_END_xxx|>

**结果**：报错 `ExternLib [servicescan] don't has [timeout]`

**问题**：直接猜测 API，没有先搜索确认


### ✅ 正确示例：搜索后编写

**场景**：需要实现端口扫描超时设置

**步骤1：先搜索**
```json
{"@action": "grep_yaklang_samples", 
 "lib_names": ["servicescan"],
 "keywords": ["超时", "timeout"],
 "human_readable_thought": "我需要先搜索 servicescan 库的超时相关选项，而不是猜测"}
```

**步骤2：查看搜索结果**
```
找到结果：
- servicescan.probeTimeout(n) - 设置探测超时
- servicescan.probeTimeoutHumanRead(duration) - 人类可读的超时格式
```

**步骤3：基于搜索结果编写**
```json
{"@action": "write_code", 
 "human_readable_thought": "根据搜索结果，应该使用 servicescan.probeTimeout(5) 来设置5秒超时"}
```

<|GEN_CODE_xxx|>
results, err := servicescan.Scan(target, ports, servicescan.probeTimeout(5))
<|GEN_CODE_END_xxx|>

**结果**：代码正确，一次通过！

---

### ✅ 正确示例：遇到错误立即搜索

**场景**：修改代码后遇到 API 错误

**Linter 报错**：
```
ExternLib [synscan] don't has [callback], maybe you meant excludePorts?
```

**步骤1：立即停止猜测**
```json
{"@action": "grep_yaklang_samples",
 "lib_names": ["synscan"],
 "regexp": ["synscan\\.\\w+\\("],
 "keywords": ["回调", "callback", "结果处理"],
 "human_readable_thought": "Linter说 synscan 没有 callback，我需要搜索 synscan 的正确结果处理方式"}
```

**步骤2：分析搜索结果**
```
找到：
- synscan.Scan() 返回 channel
- 使用 for result := range channel 来接收结果
- 或者使用 synscan.callback() 选项（注意小写）
```

**步骤3：精确修改**
```json
{"@action": "modify_code", 
 "modify_start_line": 10, 
 "modify_end_line": 10,
 "human_readable_thought": "根据搜索结果，synscan 应该返回 channel，用 for-range 接收"}
```
```

---

### 5. 代码实现层面的改进

#### 5.1 修改工具注册代码

在 `action_query_document.go` 中：

```go
var grepYaklangSamplesAction = func(r aicommon.AIInvokeRuntime, docSearcher *ziputil.ZipGrepSearcher) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"grep_yaklang_samples",  // 改名！
		`🔍 Grep Yaklang 代码样例 - 【必须优先使用】

⚠️ 铁律：禁止臆造 Yaklang 代码！必须先 grep 真实样例！

这是一个 grep 工具，用于搜索 Yaklang 代码样例库中的真实代码。

【强制使用场景】：
1. 编写任何代码前，必须先 grep 相关函数用法
2. 遇到 lint 错误（ExternLib don't has / SyntaxError）时
3. 不确定某个库的用法时
4. 需要查看某个功能的实现示例时

【搜索参数说明】：
- keywords: 关键词（如 "端口扫描", "HTTP请求", "文件读取"）
- regexp: 正则表达式（如 "servicescan\\.Scan", "poc\\.HTTP.*"）
- lib_names: 库名（如 "str", "servicescan", "poc"）
- lib_function_globs: 函数通配（如 "*Scan*", "str.*"）

【使用示例】：
grep_yaklang_samples(keywords=["端口扫描"], lib_names=["servicescan"])
grep_yaklang_samples(regexp=["synscan\\.\\w+"], lib_names=["synscan"])
grep_yaklang_samples(lib_function_globs=["*Split*", "*Join*"])

记住：Yaklang 是 DSL，每个 API 都可能与你熟悉的语言不同！
先 grep 找样例，再编写代码！这能节省 90% 的调试时间！`,
		[]aitool.ToolOption{
			aitool.WithStructParam(
				"grep_payload",  // 改参数名
				[]aitool.PropertyOption{
					aitool.WithStringArrayParam(
						"keywords",
						aitool.WithParam_Description(`关键词搜索（支持中英文）。
示例：["端口扫描", "HTTP请求", "文件读取", "错误处理"]
适用场景：搜索功能相关的代码片段`)),
					aitool.WithStringArrayParam(
						"regexp",
						aitool.WithParam_Description(`正则表达式搜索（区分大小写）。
示例：["servicescan\\.Scan", "poc\\.HTTP.*", "fuzz\\.\\w+Request"]
适用场景：精确搜索函数调用模式`)),
					aitool.WithStringArrayParam(
						"lib_names",
						aitool.WithParam_Description(`库名搜索。
示例：["str", "servicescan", "poc", "http", "file"]
适用场景：查看某个库的所有函数和用法`)),
					aitool.WithStringArrayParam(
						"lib_function_globs",
						aitool.WithParam_Description(`函数通配符搜索。
示例：["*Scan*", "str.*", "*HTTP*", "*Split*"]
适用场景：模糊搜索函数名`)),
					aitool.WithBoolParam(
						"case_sensitive",
						aitool.WithParam_Description("是否区分大小写（默认 false）"),
					),
				},
			),
		},
		// ... 其余代码
```

#### 5.2 添加强制搜索检查

在错误处理逻辑中添加检查：

```go
// 在 action_modify_code.go 中添加
func checkIfShouldGrepFirst(loop *reactloops.ReActLoop, errMsg string) bool {
	// 检查是否应该先 grep
	shouldGrep := false
	grepReason := ""
	
	if strings.Contains(errMsg, "ExternLib") && strings.Contains(errMsg, "don't has") {
		shouldGrep = true
		grepReason = "API 不存在错误，必须先 grep 搜索正确的 API"
	}
	
	if strings.Contains(errMsg, "SyntaxError") {
		shouldGrep = true
		grepReason = "语法错误，必须先 grep 搜索正确的语法示例"
	}
	
	if strings.Contains(errMsg, "undefined") {
		shouldGrep = true
		grepReason = "未定义符号，必须先 grep 搜索正确的符号名称"
	}
	
	// 检查最近是否使用过 grep
	recentActions := loop.GetRecentActions(3)
	hasRecentGrep := false
	for _, action := range recentActions {
		if action.Type == "grep_yaklang_samples" {
			hasRecentGrep = true
			break
		}
	}
	
	if shouldGrep && !hasRecentGrep {
		// 强制要求先 grep
		return true, fmt.Sprintf(`
⚠️ 检测到错误：%s

原因：%s

【强制要求】：
你必须先调用 grep_yaklang_samples 搜索正确用法，禁止继续猜测！

建议的 grep 搜索：
%s

请立即执行 grep_yaklang_samples，不要尝试 modify_code！
`, errMsg, grepReason, suggestGrepQuery(errMsg))
	}
	
	return false, ""
}
```

---

### 6. 监控和度量

添加工具使用统计，确保搜索优先：

```go
// 统计指标
type CodeGenMetrics struct {
	TotalActions        int
	GrepActions         int  // grep 调用次数
	WriteActions        int  // write_code 次数
	ModifyActions       int  // modify_code 次数
	
	GrepBeforeWrite     int  // write 前有 grep
	GrepBeforeModify    int  // modify 前有 grep
	
	BlindModifyCount    int  // 盲目修改次数（连续 modify 没有 grep）
}

// 理想比例
// GrepActions / WriteActions > 1.0  （每次写代码前至少 grep 一次）
// GrepActions / ModifyActions > 0.5 （修改时至少一半要先 grep）
// BlindModifyCount = 0               （零盲目修改）
```

---

## 实施步骤

### 阶段1：立即改进（高优先级）

1. ✅ **改名工具**
   - `query_document` → `grep_yaklang_samples`
   - 修改 `action_query_document.go` 中的注册名称

2. ✅ **更新工具描述**
   - 强调"禁止臆造"
   - 明确"搜索优先"
   - 添加使用示例

3. ✅ **强化 Prompt**
   - 在 `persistent_instruction.txt` 开头添加八荣八耻
   - 强调强制搜索场景

### 阶段2：增强验证（中优先级）

4. ✅ **修改错误提示**
   - 在 `reactive_data.txt` 中添加强制搜索提示
   - 明确何时必须 grep

5. ✅ **添加示例**
   - 在 `reflection_output_example.txt` 添加正反示例
   - 展示正确的 grep 工作流

### 阶段3：智能检查（可选）

6. ⭕ **添加强制检查逻辑**
   - 检测连续 modify 没有 grep
   - 检测 API 错误后没有 grep
   - 自动建议 grep 查询

7. ⭕ **添加指标监控**
   - 统计 grep 使用率
   - 监控盲目修改次数
   - 生成质量报告

---

## 预期效果

### 改进前（当前问题）
```
用户：帮我写个端口扫描脚本

AI：好的，我来写
→ write_code: servicescan.Scan(target, ports, servicescan.timeout(5))
→ 报错：ExternLib don't has [timeout]
→ modify_code: servicescan.setTimeout(5)
→ 报错：ExternLib don't has [setTimeout]
→ modify_code: servicescan.withTimeout(5)
→ 报错：ExternLib don't has [withTimeout]
... 循环多次才找到正确的 probeTimeout
```

### 改进后（预期行为）
```
用户：帮我写个端口扫描脚本

AI：好的，我先搜索端口扫描的样例
→ grep_yaklang_samples: keywords=["端口扫描"], lib_names=["servicescan"]
→ 找到结果：servicescan.Scan, servicescan.probeTimeout, servicescan.concurrent
→ write_code: servicescan.Scan(target, ports, servicescan.probeTimeout(5))
→ 成功！一次通过
```

---

## 关键成功因素

1. **命名很重要**：`grep` 比 `query` 更直观
2. **描述很重要**：强调"禁止臆造"比"支持查询"更有效
3. **示例很重要**：展示正确流程比理论说明更有效
4. **强制很重要**：在错误时强制要求 grep，而不是"建议"

---

## 参考资料

- `search-zip.yak`：展示了如何通过 grep 搜索代码
- 八荣八耻：核心开发哲学
- Unix 哲学：grep 是程序员的本能工具

---

## FAQ

**Q: 为什么一定要改名？**
A: AI 对工具名称非常敏感。`query_document` 听起来像"查阅文档"，而 `grep_yaklang_samples` 明确表示"grep 代码样例"，AI 会更自然地调用它。

**Q: 为什么新增工具而不是修改现有工具？**
A: 保持向后兼容，不破坏现有功能。`query_document` 和 `grep_yaklang_samples` 各有用途，AI 可以根据需求选择。

**Q: context_lines 默认值为什么是 15？**
A: 经过实践验证，15 行能覆盖大多数函数的完整上下文，包括：
- 函数定义前的注释（1-3行）
- 函数签名（1行）
- 函数体（5-10行）
- 函数调用示例（2-5行）

**Q: 如果 AI 还是不用 grep 怎么办？**
A: Prompt 已强化，在每次错误后都会提示必须 grep。如仍不够，可以添加强制检查逻辑，在检测到 API 错误后没有 grep 时，阻止继续 modify。

**Q: grep 和 query_document 何时选择？**
A: 
- 优先 `grep_yaklang_samples` - 快速找代码样例（80%的场景）
- 备选 `query_document` - 需要完整文档说明（20%的场景）

---

## grep_yaklang_samples 参数详细说明

### pattern 参数

**类型**：string (必需)

**作用**：指定要搜索的模式，支持多种格式

**支持的模式**：

1. **关键词搜索**（推荐新手）
   ```
   pattern="端口扫描"       // 中文关键词
   pattern="HTTP请求"       // 功能描述
   pattern="错误处理"       // 概念搜索
   ```

2. **精确函数名**
   ```
   pattern="servicescan.Scan"    // 完整函数名
   pattern="str.Split"           // 标准库函数
   pattern="yakit.Info"          // 输出函数
   ```

3. **正则表达式**（推荐熟练用户）
   ```
   pattern="servicescan\\."      // 搜索 servicescan 库的所有函数
   pattern="poc\\.HTTP.*"        // 搜索 poc 库的 HTTP 相关函数
   pattern="die\\(err\\)"        // 搜索错误处理模式
   pattern="端口扫描|服务扫描"     // OR 逻辑
   ```

4. **组合搜索**
   ```
   pattern="servicescan\\.Scan|端口扫描"  // 函数名或关键词
   pattern=".*Timeout|超时"              // 超时相关
   ```

**注意事项**：
- 正则表达式中的特殊字符需要转义：`.` 写成 `\\.`
- 使用 `|` 表示 OR 逻辑
- 大小写默认不敏感（可通过 case_sensitive 控制）

### case_sensitive 参数

**类型**：bool (可选)

**默认值**：false

**作用**：控制搜索是否区分大小写

**使用建议**：
```
case_sensitive=false   // 默认，推荐（覆盖更广）
case_sensitive=true    // 精确搜索特定大小写的函数
```

**示例**：
```json
// 搜索 HTTP 相关（不区分大小写，能匹配 http, HTTP, Http）
{"pattern": "http", "case_sensitive": false}

// 只搜索 HTTP（大写，精确匹配）
{"pattern": "HTTP", "case_sensitive": true}
```

### context_lines 参数

**类型**：int (可选)

**默认值**：15

**作用**：控制返回结果中每个匹配项的上下文行数

**推荐值**：

| 场景 | 推荐值 | 说明 |
|------|--------|------|
| 快速查看函数调用 | 5-10 | 只看调用方式 |
| 理解函数用法 | 15-20 | 看完整上下文（默认） |
| 学习完整实现 | 25-35 | 看整个函数或代码块 |
| 复杂功能研究 | 40-50 | 看大段实现逻辑 |

**示例**：
```json
// 快速查看函数调用
{"pattern": "servicescan.Scan", "context_lines": 10}

// 学习完整实现（包括注释、参数、返回值）
{"pattern": "servicescan.Scan", "context_lines": 30}

// 研究复杂功能（如完整的扫描流程）
{"pattern": "synscan.*servicescan", "context_lines": 50}
```

**注意事项**：
- context_lines 越大，返回内容越多，可能超出显示限制
- 建议从默认值 15 开始，根据需要调整
- 如果结果不够，可以增加到 25-30
- 如果结果太多，可以减少到 8-10

---

**记住：一次正确的 grep 胜过十次错误的猜测！**

