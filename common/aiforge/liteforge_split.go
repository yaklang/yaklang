package aiforge

import (
	"bytes"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"text/template"
)

var splitPrompt = `# 角色
你是一位资深的文本编辑和内容策略专家。你的专长是将长篇内容分解成多个逻辑独立、易于理解、并且语义流畅的模块化文本块。

# 任务
你的核心任务是**优化并分割**给定的文本。目标是创建一系列文本块，每个块都像是为独立阅读而设计的，同时严格忠于原文的核心信息。你不仅仅是在寻找切割点，更是在**打磨每个文本块的边缘，使其无缝且自包含**。

# 核心原则与授权

1.  **语义完整性与流畅性优先**：这是最高原则。每个文本块都必须能够独立存在，读者无需阅读上下文就能理解其主要内容。
2.  **授权微调（重点）**：为了实现上述目标，你被授权进行以下类型的**微小编辑**：
    *   **添加过渡词或短语**：在文本块的开头或结尾加上如“此外”、“总而言之”、“关于这一点”等词语，使其与上下文的逻辑关系更清晰（即使上下文不在当前块中）。
    *   **补充上下文指代**：将模糊的代词（如“它”、“这种方法”、“他”）替换为更具体的名词（如“这种RAG技术”、“这位研究员”），消除歧义。
    *   **轻微重述或总结**：可以对一个块的开头进行轻微的重述，以引入主题；或在结尾进行简短总结，使其结论更明确。
3.  **严守原意**：所有编辑都必须严格服务于“提高独立可读性”这一目标，**绝对不能扭曲、增删或猜测原文的核心事实、观点和意图**。编辑是为了“澄清”，而不是“创作”。
4.  **长度限制**：每个文本块的长度应尽量控制在 **{{ .Limit }}** 字符以内。但语义完整性优先于严格的长度限制。如果为了保持一个关键句子的完整而轻微超出，是可以接受的。


### 你的任务

现在，请根据上述角色、原则，处理以下文本。

**输入文本:**
{{ .Input }}

`

var splitSchema = aitool.NewObjectSchemaWithAction(
	aitool.WithStringArrayParam("text_list", aitool.WithParam_Description("The list of text blocks after splitting")),
)

func SplitText(text string, maxLength int, opts ...any) ([]string, error) {
	var promptParam = map[string]interface{}{
		"Limit": maxLength,
		"Input": text,
	}
	config := NewAnalysisConfig(opts...)
	tmp, err := template.New("splite").Parse(splitPrompt)
	if err != nil {
		return nil, utils.Errorf("template parse failed: %v", err)
	}
	var buf bytes.Buffer
	err = tmp.Execute(&buf, promptParam)
	if err != nil {
		return nil, utils.Errorf("template execute failed: %v", err)
	}

	result, err := _executeLiteForgeTemp(buf.String(), config.ForgeExecOption(splitSchema)...)
	if err != nil {
		return nil, utils.Errorf("execute liteforge failed: %v", err)
	}

	return result.GetStringSlice("text_list"), nil
}

func SplitTextSafe(text string, maxLength int, opts ...any) ([]string, error) {
	if len(text) < maxLength {
		return []string{text}, nil
	}
	result, err := SplitText(text, maxLength, opts...)
	if err != nil {
		return nil, utils.Errorf("split text failed: %v", err)
	}
	safeResult := make([]string, 0)
	for _, s := range result {
		itemRes, err := SplitTextSafe(s, maxLength, opts)
		if err != nil {
			return nil, utils.Errorf("split text failed: %v", err)
		}
		safeResult = append(safeResult, itemRes...)
	}
	return safeResult, nil
}
