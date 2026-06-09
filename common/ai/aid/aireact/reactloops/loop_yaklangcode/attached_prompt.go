package loop_yaklangcode

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type yaklangAnalyzeRequirementOptions struct {
	userInput       string
	hasAttachedPath bool
	attachedPath    string
	workspacePath   string
	hasGrepSearcher bool
	hasRAGSearcher  bool
}

func buildYaklangAnalyzeRequirementPrompt(opts yaklangAnalyzeRequirementOptions) string {
	nonce := utils.RandStringBytes(4)
	if opts.hasAttachedPath {
		return utils.MustRenderTemplate(`
你的目标是分析用户需求，完成以下任务：

【已知编辑器上下文】
用户当前打开文件：{{ .attachedPath }}
{{ if .workspacePath }}工作区目录：{{ .workspacePath }}
{{ end }}文件路径已由前端附件提供，无需再判断 create_new_file / existed_filepath；请专注生成代码搜索关键字。

{{ if .hasGrepSearcher }}【任务1：生成精确代码搜索关键字(Grep模式)】
根据用户需求，生成 2-4 个搜索模式（search_patterns），用于在 Yaklang 代码样例库中进行精确文本搜索：

搜索模式类型：
1. 函数名搜索：如 "servicescan\\.Scan", "poc\\.Get", "str\\.Split"
2. 关键词搜索：如 "端口扫描", "HTTP请求", "JSON解析"
3. 混合搜索：如 "mitm.*证书", "fuzz.*参数"

注意事项：
- 优先使用函数名搜索（使用 \\.  转义点号）
- 每个pattern要具体且相关，避免过于宽泛
- 如果涉及多个功能点，可以为每个功能点生成一个pattern
- 搜索模式需要是正则表达式或关键词
{{ end }}{{ if .hasRAGSearcher }}
【任务{{ if .hasGrepSearcher }}2{{ else }}1{{ end }}：生成语义搜索问题(RAG向量搜索)】
根据用户需求，生成 2-4 个完整的问题（semantic_questions），用于语义向量搜索相关代码样例：

问题格式要求：
1. 必须是完整的主谓宾句式
2. 禁止使用代词（它、这个、那个等）
3. 明确指明 Yaklang 语言
4. 每个问题要从不同角度描述需求

问题示例：
Good: "Yaklang中如何发送HTTP请求？"
Good: "Yaklang中如何进行端口扫描？"
Bad: "如何发送请求？" - 缺少主语
Bad: "它如何使用？" - 使用代词
Bad: "端口扫描" - 不完整句式
{{ end }}
<|USER_INPUT_{{ .nonce }}|>
{{ .data }}
<|USER_INPUT_END_{{ .nonce }}|>
`, map[string]any{
			"nonce":           nonce,
			"data":            opts.userInput,
			"attachedPath":    opts.attachedPath,
			"workspacePath":   opts.workspacePath,
			"hasGrepSearcher": opts.hasGrepSearcher,
			"hasRAGSearcher":  opts.hasRAGSearcher,
		})
	}

	return utils.MustRenderTemplate(`
你的目标是分析用户需求，完成以下任务：

【任务1：判断文件操作类型】
判断这是创建新文件还是修改已有文件：
- 如果用户明确提到文件路径（如"修改 /tmp/test.yak"），则是修改已有文件
- 如果用户只描述功能需求，没有提到具体文件，则是创建新文件
{{ if .hasGrepSearcher }}
【任务2：生成精确代码搜索关键字(Grep模式)】
根据用户需求，生成 2-4 个搜索模式（search_patterns），用于在 Yaklang 代码样例库中进行精确文本搜索：

搜索模式类型：
1. 函数名搜索：如 "servicescan\\.Scan", "poc\\.Get", "str\\.Split"
2. 关键词搜索：如 "端口扫描", "HTTP请求", "JSON解析"
3. 混合搜索：如 "mitm.*证书", "fuzz.*参数"

注意事项：
- 优先使用函数名搜索（使用 \\.  转义点号）
- 每个pattern要具体且相关，避免过于宽泛
- 如果涉及多个功能点，可以为每个功能点生成一个pattern
- 搜索模式需要是正则表达式或关键词
{{ end }}{{ if .hasRAGSearcher }}
【任务{{ if .hasGrepSearcher }}3{{ else }}2{{ end }}：生成语义搜索问题(RAG向量搜索)】
根据用户需求，生成 2-4 个完整的问题（semantic_questions），用于语义向量搜索相关代码样例：

问题格式要求：
1. 必须是完整的主谓宾句式
2. 禁止使用代词（它、这个、那个等）
3. 明确指明 Yaklang 语言
4. 每个问题要从不同角度描述需求

问题示例：
Good: "Yaklang中如何发送HTTP请求？"
Good: "Yaklang中如何进行端口扫描？"
Good: "Yaklang中如何处理JSON数据？"
Good: "Yaklang中如何调用爬虫功能？"
Bad: "如何发送请求？" - 缺少主语
Bad: "它如何使用？" - 使用代词
Bad: "端口扫描" - 不完整句式
{{ end }}
<|USER_INPUT_{{ .nonce }}|>
{{ .data }}
<|USER_INPUT_END_{{ .nonce }}|>
`, map[string]any{
		"nonce":           nonce,
		"data":            opts.userInput,
		"hasGrepSearcher": opts.hasGrepSearcher,
		"hasRAGSearcher":  opts.hasRAGSearcher,
	})
}

func buildYaklangAnalyzeRequirementToolOptions(opts yaklangAnalyzeRequirementOptions, hasSearcher bool) []aitool.ToolOption {
	var toolOptions []aitool.ToolOption
	if !opts.hasAttachedPath {
		toolOptions = append(toolOptions,
			aitool.WithBoolParam("create_new_file", aitool.WithParam_Description("Is this task to create a new file or modify an existing file? If user mentions specific file path, set to false."), aitool.WithParam_Required(true)),
			aitool.WithStringParam("existed_filepath", aitool.WithParam_Description("Only when create_new_file is false. The file path to modify.")),
		)
	}
	if opts.hasGrepSearcher {
		toolOptions = append(toolOptions,
			aitool.WithStringArrayParam("search_patterns", aitool.WithParam_Description("2-4 search patterns for finding relevant Yaklang code examples. Each pattern should be a regex or keyword."), aitool.WithParam_Required(true)),
		)
	}
	if opts.hasRAGSearcher {
		toolOptions = append(toolOptions,
			aitool.WithStringArrayParam("semantic_questions", aitool.WithParam_Description("2-4 complete questions for semantic search of Yaklang code examples. Each question must be a complete sentence with subject-predicate-object structure and explicitly mention 'Yaklang'."), aitool.WithParam_Required(true)),
		)
	}
	if hasSearcher {
		toolOptions = append(toolOptions,
			aitool.WithStringParam("reason", aitool.WithParam_Description("Explain your decision and why these search patterns/questions are chosen."), aitool.WithParam_Required(true)),
		)
	}
	return toolOptions
}
