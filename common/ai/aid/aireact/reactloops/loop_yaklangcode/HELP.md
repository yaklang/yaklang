# Yaklang AI 代码生成优化指南

## 问题诊断

### 核心问题
AI Agent 在编写 Yaklang 代码时经常出现以下问题：
1. **擅自臆造代码**：不基于实际样例，凭空想象 API 用法
2. **不使用搜索工具**：有 `query_document` 工具却不调用
3. **不符合 DSL 基调**：Yaklang 是 DSL，不是通用语言，必须基于样例编写

### 根本原因
1. **工具命名问题**：`query_document` 太抽象，AI 理解为"查文档"而非"grep 代码样例"
2. **工具描述问题**：没有强调"搜索代码样例"的核心作用
3. **Prompt 哲学缺失**：没有强调"以暗猜接口为耻，以认真查阅为荣"的核心理念
4. **缺少搜索优先原则**：没有明确要求"先搜索再编写"

---

## 解决方案

### 1. 工具改名建议

#### 当前命名
```
query_document - 查询Yaklang代码文档和库函数
```

#### 建议改名（推荐顺序）

**最推荐**：
```
grep_yaklang_samples - 搜索 Yaklang 代码样例和库函数用法
```

**备选方案**：
```
search_code_examples - 搜索代码示例和函数用法
grep_code_samples - Grep 代码样例库
find_yaklang_usage - 查找 Yaklang 函数用法示例
```

#### 改名理由
- `grep` 是程序员的本能词汇，看到就知道是"搜索"
- `samples/examples` 明确表示"代码样例"而非"文档"
- AI 看到 `grep_yaklang_samples` 会自然联想到"grep代码找例子"
- 符合 Unix 哲学，直观易懂

---

### 2. 工具描述优化

#### 当前描述
```
查询Yaklang代码文档和库函数。支持关键字搜索（使用动宾结构，如'端口扫描'、'文件读取'）、
正则表达式匹配、库名查询（如'str'、'http'）和函数模糊搜索（如'*Split*'、'str.Join'）。
当你需要了解某个功能如何实现、查找特定函数或学习库的用法时使用此工具。
```

#### 建议描述（强调搜索优先）
```
🔍 Grep Yaklang 代码样例库 - 你的首要工具！

⚠️ 核心原则：禁止臆造 Yaklang 代码！必须先 grep 搜索真实样例！

使用场景（按优先级排序）：
1. 【最高优先级】编写任何代码前，先 grep 相关函数用法
2. 【必须】遇到 API 错误（ExternLib [...] don't has [...]）时
3. 【必须】遇到语法错误（SyntaxError）时
4. 【强烈推荐】不确定某个库的用法时
5. 【推荐】需要学习如何实现某个功能时

支持的搜索方式：
- keywords: 关键词搜索（如 "端口扫描", "文件读取", "HTTP请求"）
- regexp: 正则表达式（如 "servicescan\\.Scan", "poc\\.HTTPEx"）
- lib_names: 库名查询（如 "str", "http", "servicescan"）
- lib_function_globs: 函数模糊搜索（如 "*Split*", "str.Join*"）

记住：Yaklang 是 DSL，不是你熟悉的 Python/Go，每个 API 都可能不同！
先 grep，后编写！
```

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

**Q: 改名后会不会影响现有代码？**
A: 只需修改工具注册的名称和参数名，不影响底层实现。已有的调用需要更新工具名。

**Q: 如果 AI 还是不用 grep 怎么办？**
A: 添加强制检查逻辑（阶段3），在检测到应该 grep 但没 grep 时，直接返回错误并要求 grep。

**Q: 搜索结果太多怎么办？**
A: 已有 limit 和 size 限制，可以通过 RRF 排序返回最相关的结果。关键是让 AI 意识到搜索的重要性。

---

**记住：一次正确的 grep 胜过十次错误的猜测！**

