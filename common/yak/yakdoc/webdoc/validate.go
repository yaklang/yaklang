package webdoc

import "strings"

// 关键词: 示例抽取, ExtractYakExamples, 示例语法校验, CheckExampleSyntax
//
// 这里复刻"最终消费方"从生成的 Markdown 里抽取示例代码的逻辑：示例统一用 14 反引号 yak 围栏
// 包裹(见 fenceExampleYak)。抽取后由调用方(generate_web_doc，持有引擎)做 antlr 语法/编译检查。
// 本包不依赖引擎，故 CheckExampleSyntax 以注入 checker 的方式工作，保持 webdoc 纯净可测。

// ExtractYakExamples 从生成的 Markdown 中抽取所有 14 反引号 yak 围栏里的示例代码。
func ExtractYakExamples(md string) []string {
	lines := strings.Split(md, "\n")
	var examples []string
	var cur []string
	in := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !in {
			// 开围栏：14 反引号 + yak
			if trimmed == exampleFence+"yak" {
				in = true
				cur = nil
			}
			continue
		}
		// 闭围栏：恰好 14 反引号
		if trimmed == exampleFence {
			examples = append(examples, strings.Join(cur, "\n"))
			in = false
			cur = nil
			continue
		}
		cur = append(cur, line)
	}
	return examples
}

// CheckExampleSyntax 用注入的 checker 校验一段示例代码的语法/可编译性。checker 为空则跳过。
func CheckExampleSyntax(code string, checker func(string) error) error {
	if checker == nil {
		return nil
	}
	return checker(code)
}
