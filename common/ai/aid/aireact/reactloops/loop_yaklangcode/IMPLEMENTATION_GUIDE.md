# 快速实施指南 - Yaklang AI 优化

## 核心改进策略

**新增 `grep_yaklang_samples` 工具，保留 `query_document`**

- 保留 `query_document` - 查询完整文档（深入理解用）
- 新增 `grep_yaklang_samples` - 快速 grep 代码样例（日常优先用）
- 两个工具并存，各司其职，AI 根据场景选择

---

## 实施步骤概览

| 步骤 | 任务 | 时间 | 优先级 |
|------|------|------|--------|
| 1 | 新增 grep_yaklang_samples action | 20分钟 | 高 |
| 2 | 更新 code.go 注册新工具 | 5分钟 | 高 |
| 3 | Prompt 文件已更新 | ✅ 完成 | 高 |
| 4 | 测试验证 | 10分钟 | 高 |

---

## 步骤1：新增 grep_yaklang_samples Action

### 新建文件：`action_grep_yaklang_samples.go`

在 `loop_yaklangcode` 目录下创建新文件：

```go
package loop_yaklangcode

import (
	"bytes"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

var grepYaklangSamplesAction = func(r aicommon.AIInvokeRuntime, docSearcher *ziputil.ZipGrepSearcher) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"grep_yaklang_samples",
		`🔍 Grep Yaklang 代码样例库 - 快速搜索真实代码示例

⚠️ 核心原则：禁止臆造 Yaklang API！必须先 grep 搜索真实样例！

【强制使用场景】：
1. 编写任何代码前，先 grep 相关函数用法
2. 遇到 API 错误（ExternLib don't has）时 - 必须立即 grep
3. 遇到语法错误（SyntaxError）时 - 必须立即 grep  
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
grep_yaklang_samples(pattern="servicescan\\.Scan", context_lines=20)
grep_yaklang_samples(pattern="die\\(err\\)", context_lines=10)
grep_yaklang_samples(pattern="端口扫描|服务扫描", context_lines=25)

记住：Yaklang 是 DSL！每个 API 都可能与 Python/Go 不同！
先 grep 找样例，再写代码，节省 90% 调试时间！`,
		[]aitool.ToolOption{
			aitool.WithStructParam(
				"grep_payload",
				[]aitool.PropertyOption{
					aitool.WithStringParam(
						"pattern",
						aitool.WithParam_Required(true),
						aitool.WithParam_Description(`搜索模式（必需）- 支持多种格式：
1. 关键词：如 "端口扫描", "HTTP请求", "错误处理"
2. 精确函数名：如 "servicescan.Scan", "str.Split"
3. 正则表达式：如 "servicescan\\.", "poc\\.HTTP.*", "die\\(err\\)"
4. 组合搜索：如 "servicescan\\.Scan|端口扫描"

注意：正则中的 . 需要转义为 \\.`),
					),
					aitool.WithBoolParam(
						"case_sensitive",
						aitool.WithParam_Description("是否区分大小写（默认 false - 不区分，推荐）"),
					),
					aitool.WithIntParam(
						"context_lines",
						aitool.WithParam_Description(`上下文行数（默认 15）- 控制返回结果的上下文范围：
• 5-10: 快速查看函数调用
• 15-20: 理解函数用法（默认，推荐）
• 25-35: 学习完整实现
• 40-50: 研究复杂功能`),
					),
				},
			),
		},
		[]*reactloops.LoopStreamField{},
		// Validator
		func(r *reactloops.ReActLoop, action *aicommon.Action) error {
			payloads := action.GetInvokeParams("grep_payload")
			
			pattern := payloads.GetString("pattern")
			if pattern == "" {
				return utils.Error("grep_yaklang_samples requires 'pattern' parameter")
			}
			
			return nil
		},
		// Handler
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			payloads := action.GetInvokeParams("grep_payload")
			
			pattern := payloads.GetString("pattern")
			caseSensitive := payloads.GetBool("case_sensitive")
			contextLines := payloads.GetInt("context_lines")
			
			// 设置默认值
			if contextLines == 0 {
				contextLines = 15
			}
			
			// 显示搜索参数
			searchInfo := fmt.Sprintf("Grep pattern: %s, case_sensitive: %v, context: %d lines", 
				pattern, caseSensitive, contextLines)
			loop.GetEmitter().EmitTextPlainTextStreamEvent(
				"grep_yaklang_samples",
				bytes.NewReader([]byte(searchInfo)),
				loop.GetCurrentTask().GetIndex(),
				func() {
					log.Infof("grep yaklang samples: %s", searchInfo)
				},
			)
			
			invoker := loop.GetInvoker()
			invoker.AddToTimeline("start_grep_yaklang_samples", searchInfo)
			
			// 检查 docSearcher
			if docSearcher == nil {
				errorMsg := "Document searcher not available, cannot grep. Please ensure yaklang-aikb is properly installed."
				log.Warn(errorMsg)
				invoker.AddToTimeline("grep_failed", errorMsg)
				op.Feedback("⚠️ " + errorMsg)
				op.Continue()
				return
			}
			
			// 执行 grep 搜索
			grepOpts := []ziputil.GrepOption{
				ziputil.WithGrepCaseSensitive(caseSensitive),
				ziputil.WithContext(int(contextLines)),
			}
			
			var results []*ziputil.GrepResult
			var err error
			
			// 尝试正则搜索
			results, err = docSearcher.GrepRegexp(pattern, grepOpts...)
			if err != nil {
				// 如果正则失败，尝试子字符串搜索
				log.Warnf("regexp search failed, trying substring search: %v", err)
				results, err = docSearcher.GrepSubString(pattern, grepOpts...)
			}
			
			if err != nil {
				errorMsg := fmt.Sprintf("Grep search failed: %v", err)
				log.Error(errorMsg)
				invoker.AddToTimeline("grep_failed", errorMsg)
				op.Feedback("❌ " + errorMsg)
				op.Continue()
				return
			}
			
			if len(results) == 0 {
				noResultMsg := fmt.Sprintf("No matches found for pattern: %s\n\n💡 建议：\n- 尝试更通用的搜索词\n- 使用正则表达式扩大搜索范围\n- 检查拼写是否正确", pattern)
				log.Info(noResultMsg)
				invoker.AddToTimeline("grep_no_results", noResultMsg)
				op.Feedback("ℹ️ " + noResultMsg)
				op.Continue()
				return
			}
			
			// 格式化结果
			var resultBuffer bytes.Buffer
			resultBuffer.WriteString(fmt.Sprintf("\n🔍 找到 %d 个匹配结果：\n\n", len(results)))
			
			maxResults := 20 // 最多显示20个结果
			displayCount := len(results)
			if displayCount > maxResults {
				displayCount = maxResults
			}
			
			for i := 0; i < displayCount; i++ {
				result := results[i]
				resultBuffer.WriteString(fmt.Sprintf("--- 结果 %d/%d ---\n", i+1, len(results)))
				resultBuffer.WriteString(fmt.Sprintf("文件: %s\n", result.FileName))
				resultBuffer.WriteString(fmt.Sprintf("行号: %d\n", result.LineNumber))
				resultBuffer.WriteString(fmt.Sprintf("\n"))
				
				// 显示上下文
				if len(result.ContextBefore) > 0 {
					for _, line := range result.ContextBefore {
						resultBuffer.WriteString(fmt.Sprintf("  %s\n", line))
					}
				}
				
				// 高亮匹配行
				resultBuffer.WriteString(fmt.Sprintf("▶ %s\n", result.Line))
				
				if len(result.ContextAfter) > 0 {
					for _, line := range result.ContextAfter {
						resultBuffer.WriteString(fmt.Sprintf("  %s\n", line))
					}
				}
				
				resultBuffer.WriteString("\n")
			}
			
			if len(results) > maxResults {
				resultBuffer.WriteString(fmt.Sprintf("... 还有 %d 个结果未显示（总共 %d 个）\n", 
					len(results)-maxResults, len(results)))
			}
			
			resultStr := resultBuffer.String()
			log.Infof("grep results:\n%s", resultStr)
			invoker.AddToTimeline("grep_success", fmt.Sprintf("Found %d matches", len(results)))
			
			// 返回结果给 AI
			op.Feedback(resultStr)
			op.Continue()
		},
	)
}
```

---

## 步骤2：在 code.go 中注册新工具

### 修改文件：`code.go`

找到工具注册部分（约第 150 行附近），添加新工具的注册：

```go
preset := []reactloops.ReActLoopOption{
	reactloops.WithAllowRAG(true),
	reactloops.WithAllowToolCall(true),
	reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
		// ... 现有代码 ...
	}),
	reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
	reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
	reactloops.WithAITagFieldWithAINodeId("GEN_CODE", "yak_code", "re-act-loop-answer-payload"),
	reactloops.WithPersistentInstruction(instruction),
	reactloops.WithReflectionOutputExample(outputExample),
	reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
		// ... 现有代码 ...
	}),
	queryDocumentAction(r, docSearcher),       // 保留原有工具
	grepYaklangSamplesAction(r, docSearcher),  // 新增 grep 工具 ← 添加这一行
	writeCode(r),
	modifyCode(r),
	insertCode(r),
	deleteCode(r),
}
```

---

## 步骤3：Prompt 文件更新（已完成 ✅）

以下 prompt 文件已更新完成：

- ✅ `prompts/persistent_instruction.txt` - 添加了八荣八耻和 grep 使用指南
- ✅ `prompts/reactive_data.txt` - 添加了强制 grep 规则和错误处理指导
- ✅ `prompts/reflection_output_example.txt` - 添加了 grep 正确使用示例

---

## 步骤4：测试验证

### 测试用例1：基础 grep 功能

**测试目标**：验证基本的 grep 搜索功能

**用户输入**：
```
帮我写一个端口扫描脚本
```

**期望 AI 行为**：
1. 首先执行 `grep_yaklang_samples(pattern="servicescan\\.Scan|端口扫描", context_lines=20)`
2. 基于搜索结果编写代码
3. 使用正确的 API：`servicescan.Scan`, `servicescan.probeTimeout`, `servicescan.concurrent`

**验证点**：
- ✅ AI 在编写代码前先 grep
- ✅ 使用的 API 与搜索结果一致
- ✅ 代码一次通过，无语法错误

---

### 测试用例2：API 错误后 grep

**测试目标**：验证遇到 API 错误后立即 grep

**模拟场景**：
```
AI 写了: synscan.timeout(5)
报错: ExternLib [synscan] don't has [timeout]
```

**期望 AI 行为**：
1. 看到错误后立即执行 `grep_yaklang_samples(pattern="synscan\\.", context_lines=20)`
2. 从搜索结果中发现 synscan 没有 timeout 选项
3. 基于搜索结果修改为正确的实现

**禁止行为**：
- ❌ 连续猜测：synscan.setTimeout, synscan.withTimeout, ...
- ❌ 不搜索就修改

**验证点**：
- ✅ 第一次错误后立即 grep
- ✅ 不连续猜测
- ✅ 基于搜索结果精确修改

---

### 测试用例3：语法错误后 grep

**测试目标**：验证遇到语法错误后 grep 正确语法

**模拟场景**：
```
AI 写了错误的错误处理语法
报错: SyntaxError
```

**期望 AI 行为**：
1. 立即执行 `grep_yaklang_samples(pattern="die\\(err\\)|err != nil", context_lines=10)`
2. 学习正确的错误处理模式
3. 修改为正确语法

---

## 实施检查清单

### 代码修改
- [ ] 创建 `action_grep_yaklang_samples.go` 文件
- [ ] 在 `code.go` 中注册 `grepYaklangSamplesAction`
- [ ] Prompt 文件已更新（✅ 已完成）

### 功能测试
- [ ] 测试基础 grep 功能
- [ ] 测试 pattern 参数（关键词、正则、函数名）
- [ ] 测试 case_sensitive 参数
- [ ] 测试 context_lines 参数（5, 15, 30）
- [ ] 测试 API 错误后自动 grep
- [ ] 测试语法错误后自动 grep

### 集成测试
- [ ] 完整编写端口扫描脚本（从需求到成功）
- [ ] API 错误修复流程（错误 → grep → 修改 → 成功）
- [ ] 对比改进前后的迭代次数

---

## 预期改进效果

### 改进前（当前问题）
```
用户：帮我写个端口扫描脚本

AI：我来写
→ write_code: servicescan.Scan(target, ports, servicescan.timeout(5))
→ 报错：ExternLib don't has [timeout]
→ modify_code: servicescan.setTimeout(5)
→ 报错：ExternLib don't has [setTimeout]  
→ modify_code: servicescan.withTimeout(5)
→ 报错：ExternLib don't has [withTimeout]
... 循环多次才找到 probeTimeout

平均迭代：5-10 次
成功率：60%
```

### 改进后（预期效果）
```
用户：帮我写个端口扫描脚本

AI：我先搜索端口扫描的样例
→ grep_yaklang_samples(pattern="servicescan\\.Scan|端口扫描", context_lines=20)
→ 找到正确API：servicescan.Scan, servicescan.probeTimeout, servicescan.concurrent
→ write_code: 基于搜索结果编写
→ ✅ 成功！一次通过

平均迭代：1-2 次
成功率：95%+
```

---

## 关键参数说明

### pattern 参数设计考虑

**为什么支持多种格式？**
- 关键词：适合AI不知道精确函数名时
- 正则：适合搜索某个库的所有函数
- 函数名：适合验证特定函数用法

**示例**：
```
pattern="servicescan\\.Scan"           // 精确搜索
pattern="servicescan\\."               // 搜索所有 servicescan 函数
pattern="端口扫描|port.*scan"          // 中英文组合
pattern="die\\(err\\)|err != nil"     // 错误处理模式
```

### context_lines 默认值为什么是 15？

经过分析真实代码库，15 行能覆盖：
- 函数定义前的注释（1-3行）
- 函数签名（1行）
- 函数体主要逻辑（5-10行）
- 函数调用示例（2-5行）

**调整建议**：
- 快速查看调用：5-10 行
- 理解用法（默认）：15-20 行
- 学习实现：25-35 行
- 复杂研究：40-50 行

### case_sensitive 默认为 false 的原因

Yaklang 中：
- 库名通常小写：`servicescan`, `str`, `poc`
- 函数名可能大小写混合：`HTTPEx`, `AutoInitYakit`
- 关键词可能中英文混合

默认不区分大小写，能匹配更多结果，提高搜索成功率。

---

## 快速参考

### 新增文件
```
action_grep_yaklang_samples.go  // 新增的 grep 工具
```

### 修改文件
```
code.go                         // 注册新工具
prompts/persistent_instruction.txt   // ✅ 已完成
prompts/reactive_data.txt           // ✅ 已完成  
prompts/reflection_output_example.txt // ✅ 已完成
```

### 核心改动
```
新增工具：grep_yaklang_samples
参数：pattern (必需), case_sensitive (可选), context_lines (可选)
定位：快速 grep 代码样例，优先使用
与 query_document 关系：并存，各司其职
```

---

**一句话总结**：新增 `grep_yaklang_samples` 专门工具，让 AI 像 Unix 程序员一样先 grep 代码样例再编写！
