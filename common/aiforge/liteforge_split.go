package aiforge

import (
	"bytes"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"text/template"
)

var splitPrompt = promptloader.MustLoad("inline/aiforge/liteforge_split/splitPrompt.txt")

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
