# 快速实施指南 - Yaklang AI 优化

## 立即可执行的改进（10分钟内完成）

### 改进1：修改工具名称（最关键！）

#### 文件：`action_query_document.go`

**改动位置1：工具名称（第16行）**
```go
// 原来
"query_document",

// 改为
"grep_yaklang_samples",
```

**改动位置2：工具描述（第17行）**
```go
// 原来
"查询Yaklang代码文档和库函数。支持关键字搜索...",

// 改为（完整版本）
`🔍 Grep Yaklang 代码样例库 - 你编写代码前的首要工具

⚠️ 核心原则：禁止臆造 Yaklang API！必须先 grep 搜索真实样例！

【强制使用场景】- 必须先 grep：
1. 编写任何新功能代码前
2. 遇到 lint 错误（ExternLib don't has / SyntaxError）后
3. 对某个库/函数不确定时
4. 需要查看功能实现示例时

【搜索参数】：
• keywords - 关键词（如 "端口扫描", "HTTP请求"）
• regexp - 正则表达式（如 "servicescan\\.Scan"）
• lib_names - 库名（如 "servicescan", "poc", "str"）
• lib_function_globs - 函数通配（如 "*Scan*", "*Split*"）

记住：Yaklang 是 DSL！每个 API 都可能不同！先 grep 再写！`,
```

**改动位置3：参数名称（第20行）**
```go
// 原来
"query_document_payload",

// 改为
"grep_payload",
```

**改动位置4：参数描述优化**
```go
// 优化 keywords 描述（第29行附近）
aitool.WithStringArrayParam(
	"keywords",
	aitool.WithParam_Description(`关键词/短语搜索（中英文）。
示例：["端口扫描", "HTTP请求", "文件读取", "错误处理"]
适用：搜索功能相关代码片段`)),

// 优化 regexp 描述
aitool.WithStringArrayParam(
	"regexp",
	aitool.WithParam_Description(`正则表达式匹配（区分大小写）。
示例：["servicescan\\.Scan", "poc\\.HTTP.*", "str\\.\\w+"]
适用：精确搜索函数调用模式
注意：用 \\ 转义特殊字符`)),

// 优化 lib_names 描述
aitool.WithStringArrayParam(
	"lib_names",
	aitool.WithParam_Description(`库名查询 - 查看整个库的函数。
示例：["servicescan", "str", "poc", "http", "file"]
适用：了解某个库有哪些功能`)),

// 优化 lib_function_globs 描述
aitool.WithStringArrayParam(
	"lib_function_globs",
	aitool.WithParam_Description(`函数通配符搜索 - 模糊匹配函数名。
示例：["*Scan*", "str.Split*", "*HTTP*"]
适用：不确定完整函数名时`)),
```

**改动位置5：事件名称（第88行）**
```go
// 原来
"query_yaklang_document",

// 改为
"grep_yaklang_samples",
```

**改动位置6：Timeline 消息（第97行、第100行）**
```go
// 原来
invoker.AddToTimeline("start_query_yaklang_docs", "AI decided to query document...")
invoker.AddToTimeline("query_yaklang_docs_result", "No document searcher available...")

// 改为
invoker.AddToTimeline("start_grep_yaklang_samples", "AI decided to grep yaklang samples: "+utils.InterfaceToString(payloads))
invoker.AddToTimeline("grep_yaklang_samples_no_result", "No document searcher available, cannot grep: "+utils.InterfaceToString(payloads))
```

---

### 改进2：强化 Prompt - 添加强制搜索原则

#### 文件：`prompts/persistent_instruction.txt`

**在文件开头（第1行之前）添加：**

```markdown
## ⚠️⚠️⚠️ Yaklang 代码生成核心原则 - 搜索优先！⚠️⚠️⚠️

### 八荣八耻 - Yaklang 开发者的行为准则

以暗猜接口为耻，以认真查阅为荣
以模糊执行为耻，以寻求确认为荣
以盲想业务为耻，以人类确认为荣
以创造接口为耻，以复用现有为荣
以跳过验证为耻，以主动测试为荣
以破坏架构为耻，以遵循规范为荣
以假装理解为耻，以诚实无知为荣
以盲目修改为耻，以谨慎重构为荣

### 核心工作流程（强制执行）

```
【正确流程】
需求理解 → grep_yaklang_samples 搜索 → 基于样例编写 → 测试 → (如有错误) → grep 搜索 → 精确修改

【错误流程 - 禁止！】
需求理解 → 直接写代码 → 报错 → 猜测修改 → 报错 → 再猜测 → ...
```

### 强制 grep 场景（必须执行）

1. **编写任何代码前** - 先 grep 相关功能的样例
2. **遇到 lint 错误后** - 立即 grep，禁止猜测
3. **使用新库/函数时** - 先 grep 用法
4. **不确定参数时** - grep 搜索示例

### 禁止行为清单

❌ 看到需求就直接写代码（没有先 grep）
❌ 遇到 API 错误后继续猜测其他 API 名称
❌ 连续 2 次以上 modify_code 而没有 grep
❌ 假装知道某个函数的用法（实际没 grep 确认）
❌ 使用 "我觉得"、"应该是"、"可能是" 这类猜测性语言

### grep_yaklang_samples 工具是你的第一选择

**重要性排序：**
1. grep_yaklang_samples - 【最重要】搜索代码样例
2. write_code - 基于 grep 结果编写代码
3. modify_code - 基于 grep 结果修改代码
4. bash - 测试代码

**使用频率期望：**
- 理想：每次 write_code 前至少 1 次 grep
- 底线：每次遇到错误后必须 grep

---

```

**在原有的"代码生成与修改的铁律"之后（约第35行）添加：**

```markdown
## grep_yaklang_samples - 你最重要的工具

### 为什么必须使用 grep？

Yaklang 是一门 **DSL（领域特定语言）**，不是 Python、Go、JavaScript！
- API 命名可能完全不同
- 语法可能有特殊规则
- 参数顺序可能不符合直觉

**猜测 = 浪费时间 = 连续报错**
**grep = 准确快速 = 一次成功**

### 何时必须 grep（强制）

1. **API 不存在错误**
   ```
   错误：ExternLib [servicescan] don't has [timeout]
   行动：立即 grep lib_names=["servicescan"] 查看所有可用选项
   ```

2. **语法错误**
   ```
   错误：SyntaxError near 'if err != nil'
   行动：立即 grep keywords=["错误处理", "error handling"]
   ```

3. **不确定的函数**
   ```
   想用：不确定 str 库有没有 Split 函数
   行动：立即 grep lib_function_globs=["str.Split*"]
   ```

### grep 搜索示例

**场景1：想实现端口扫描**
```json
{"@action": "grep_yaklang_samples", 
 "keywords": ["端口扫描", "服务扫描"],
 "lib_names": ["servicescan"],
 "human_readable_thought": "我需要先查看端口扫描的样例代码"}
```

**场景2：遇到 API 错误**
```json
{"@action": "grep_yaklang_samples",
 "lib_names": ["synscan"],
 "regexp": ["synscan\\.\\w+"],
 "human_readable_thought": "synscan.timeout 不存在，我需要搜索 synscan 的所有可用选项"}
```

**场景3：模糊搜索函数**
```json
{"@action": "grep_yaklang_samples",
 "lib_function_globs": ["*Split*", "*Join*"],
 "human_readable_thought": "我需要查找字符串分割和拼接的函数"}
```

```

---

### 改进3：优化错误提示 - 强制 grep

#### 文件：`prompts/reactive_data.txt`

**找到 FeedbackMessages 部分（约第464行），在 `<|ERR/LINT_WARNING_END|>` 之后添加：**

```markdown
### ⚠️ 错误处理强制规则 ⚠️

**如果上述错误包含以下任何一种，你必须立即使用 grep_yaklang_samples：**

#### 错误类型1：API 不存在
```
ExternLib [xxx] don't has [yyy]
```
**含义**：你猜错了 API 名称，该库没有这个函数/选项
**行动**：必须 grep_yaklang_samples，参数设置：
- lib_names: ["xxx"]  （搜索该库）
- regexp: ["xxx\\.\\w+"]  （搜索该库的所有函数）

**禁止**：❌ 继续猜测其他 API 名称
**正确**：✅ 立即 grep 搜索真实可用的 API

#### 错误类型2：语法错误
```
SyntaxError: ...
```
**含义**：你的语法不符合 Yaklang DSL 规范
**行动**：必须 grep_yaklang_samples，参数设置：
- keywords: ["相关功能的中文描述"]
- regexp: ["相关的代码模式"]

**禁止**：❌ 继续尝试不同的语法写法
**正确**：✅ grep 搜索正确的语法示例

#### 错误类型3：未定义符号
```
undefined: xxx
```
**含义**：变量/函数不存在
**行动**：必须 grep_yaklang_samples，参数设置：
- lib_function_globs: ["*xxx*"]
- keywords: ["功能描述"]

### 反面教材 - 禁止的行为模式

❌ **错误模式1：连续猜测**
```
尝试1: servicescan.timeout(5)     → 报错
尝试2: servicescan.setTimeout(5)  → 报错
尝试3: servicescan.withTimeout(5) → 报错
... 继续猜测
```

✅ **正确模式：立即搜索**
```
尝试1: servicescan.timeout(5) → 报错
行动: grep_yaklang_samples(lib_names=["servicescan"]) → 找到 probeTimeout
成功: servicescan.probeTimeout(5) → 通过！
```

### 自查清单

在执行 modify_code 之前，问自己：
1. ✅ 我是否刚刚 grep 过相关 API？
2. ✅ 我的修改是基于 grep 结果还是猜测？
3. ✅ 如果是猜测，为什么不先 grep？

如果答案是"我在猜测"，**立即停止**，先执行 grep_yaklang_samples！

```

---

### 改进4：添加正确示例

#### 文件：`prompts/reflection_output_example.txt`

**在文件末尾（第97行后）添加：**

```markdown

---

## ✅ grep_yaklang_samples 正确使用示例

### 示例1：编写新功能前先 grep

**场景**：用户要求实现一个端口扫描脚本

**步骤1：理解需求后立即 grep**
```json
{"@action": "grep_yaklang_samples",
 "keywords": ["端口扫描", "服务扫描", "servicescan"],
 "lib_names": ["servicescan"],
 "human_readable_thought": "用户需要端口扫描功能，我先搜索 servicescan 库的使用示例，了解正确的 API 用法"}
```

**步骤2：查看 grep 结果**
```
找到 15 个相关样例：
- servicescan.Scan(target, ports, ...opts)
- servicescan.concurrent(n) - 设置并发数
- servicescan.probeTimeout(n) - 设置超时
- servicescan.onOpen(callback) - 开放端口回调
```

**步骤3：基于 grep 结果编写代码**
```json
{"@action": "write_code",
 "human_readable_thought": "根据 grep 结果，我知道了正确的用法：servicescan.Scan + probeTimeout + concurrent + onOpen"}
```

<|GEN_CODE_xxx|>
yakit.AutoInitYakit()

target := cli.String("target", cli.setRequired(true))
ports := cli.String("ports", cli.setDefault("80,443,8080"))
cli.check()

results, err := servicescan.Scan(
    target, 
    ports,
    servicescan.concurrent(50),
    servicescan.probeTimeout(5),
    servicescan.onOpen(result => {
        yakit.Info("发现开放端口: %v", result.String())
    }),
)
die(err)

for result in results {
    yakit.Info("扫描结果: %v", result.String())
}
<|GEN_CODE_END_xxx|>

**结果**：✅ 代码一次通过，没有错误！

---

### 示例2：遇到 API 错误后立即 grep

**场景**：修改代码后遇到错误

**Linter 报错**：
```
Line 10: ExternLib [synscan] don't has [callback], maybe you meant excludePorts?
```

**❌ 错误做法：继续猜测**
```json
{"@action": "modify_code",
 "modify_start_line": 10,
 "modify_end_line": 10,
 "human_readable_thought": "callback 不行，我试试 onCallback"}
```
*这会导致继续报错，浪费时间！*

**✅ 正确做法：立即 grep**
```json
{"@action": "grep_yaklang_samples",
 "lib_names": ["synscan"],
 "regexp": ["synscan\\.\\w+"],
 "keywords": ["回调", "结果处理"],
 "human_readable_thought": "synscan.callback 不存在，Linter 提示可能是 excludePorts，但我不确定如何处理扫描结果。我需要 grep 搜索 synscan 的正确用法"}
```

**查看 grep 结果**：
```
找到结果：
- synscan.Scan() 返回 channel 和 error
- 使用 for result := range channel 接收结果
- 没有 callback 选项，应该直接遍历 channel
```

**基于 grep 结果修改**：
```json
{"@action": "modify_code",
 "modify_start_line": 8,
 "modify_end_line": 12,
 "human_readable_thought": "根据 grep 结果，synscan.Scan 返回 channel，应该用 for-range 接收，而不是 callback"}
```

<|GEN_CODE_xxx|>
synChan, err := synscan.Scan(target, ports)
die(err)

for result := range synChan {
    yakit.Info("发现开放端口: %v", result.String())
}
<|GEN_CODE_END_xxx|>

**结果**：✅ 修改成功，一次通过！

---

### 示例3：不确定函数名时 grep

**场景**：需要字符串分割功能，但不确定函数名

**❌ 错误做法：猜测**
```json
{"@action": "write_code",
 "human_readable_thought": "应该是 str.split 吧"}
```
*可能函数名不对*

**✅ 正确做法：先 grep**
```json
{"@action": "grep_yaklang_samples",
 "lib_function_globs": ["*Split*", "str.*"],
 "keywords": ["字符串分割", "split"],
 "human_readable_thought": "我不确定 Yaklang 中字符串分割函数的准确名称，先 grep 搜索"}
```

**查看 grep 结果**：
```
找到：
- str.Split(s, sep) - 分割字符串
- str.SplitN(s, sep, n) - 分割 N 次
- str.ParseStringToLines(s) - 按行分割
```

**基于结果编写**：
```json
{"@action": "write_code",
 "human_readable_thought": "根据 grep 结果，应该使用 str.Split(s, sep)"}
```

---

## ❌ grep 反面教材 - 禁止的错误模式

### 反面教材1：不 grep 直接写

```json
{"@action": "write_code",
 "human_readable_thought": "用户要端口扫描，我直接写"}
```
**问题**：没有先 grep 确认 API，可能写错

### 反面教材2：报错后继续猜测

**报错**: `ExternLib [poc] don't has [Get]`

```json
{"@action": "modify_code",
 "human_readable_thought": "Get 不行，试试 HTTPGet"}
```
**问题**：继续猜测而不是 grep 搜索

### 反面教材3：连续多次 modify 没有 grep

```
第1次: modify_code → 报错
第2次: modify_code → 报错
第3次: modify_code → 报错
```
**问题**：陷入猜测循环，应该在第一次报错后立即 grep

---

**记住：grep 一次，胜过猜测十次！**
```

---

## 实施检查清单

完成以上修改后，请检查：

### ✅ 文件修改清单

- [ ] `action_query_document.go` - 工具名改为 `grep_yaklang_samples`
- [ ] `action_query_document.go` - 工具描述强调"搜索优先"
- [ ] `action_query_document.go` - 参数名改为 `grep_payload`
- [ ] `action_query_document.go` - 所有参数描述优化
- [ ] `prompts/persistent_instruction.txt` - 开头添加八荣八耻
- [ ] `prompts/persistent_instruction.txt` - 添加 grep 使用指南
- [ ] `prompts/reactive_data.txt` - 错误提示部分添加强制 grep 规则
- [ ] `prompts/reflection_output_example.txt` - 添加 grep 正确示例

### ✅ 代码修改清单

- [ ] 所有 `query_document` 引用改为 `grep_yaklang_samples`
- [ ] 所有 `query_document_payload` 改为 `grep_payload`
- [ ] 所有相关的 timeline 事件名称更新

### ✅ 测试验证

测试用例：要求 AI 写一个端口扫描脚本

**期望行为**：
1. AI 首先执行 `grep_yaklang_samples`
2. 搜索 `servicescan` 相关样例
3. 基于搜索结果编写代码
4. 一次通过，无错误

**如果出现问题**：
- AI 直接写代码没有 grep → Prompt 需要更强调
- AI 遇到错误继续猜测 → 错误提示需要更明确

---

## 预期改进效果

### 改进前
```
用户请求 → AI 猜测写代码 → 报错 → 猜测修改 → 报错 → 再猜测 → ...
平均迭代: 5-10 次
成功率: 60%
```

### 改进后
```
用户请求 → AI grep 搜索 → 基于样例写代码 → 成功
平均迭代: 1-2 次  
成功率: 95%+
```

---

## 快速参考

### 核心改动
1. 工具名：`query_document` → `grep_yaklang_samples`
2. 核心理念：查询文档 → grep 代码样例
3. 行为准则：八荣八耻 + 搜索优先

### 关键文件
- `action_query_document.go` - 工具定义
- `prompts/persistent_instruction.txt` - 持久指令
- `prompts/reactive_data.txt` - 响应式数据（错误处理）
- `prompts/reflection_output_example.txt` - 示例

---

**一句话总结**：把"查询文档"改成"grep 代码样例"，让 AI 像 Unix 程序员一样先 grep 再写代码！

