package aiforge

import (
	"bytes"
	"github.com/yaklang/yaklang/common/ai/aid"
	"text/template"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type ForgePromptParams struct {
	UserParams       string
	InitPrompt       string
	PersistentPrompt string
	UserQuery        string // raw user query
}

// Params if set userInput will check require cli parameter
func (f *ForgeBlueprint) Params(query string, userInput ...*ypb.ExecParamItem) (*ForgePromptParams, error) {
	if len(userInput) == 0 {
		return &ForgePromptParams{
			UserQuery:        query,
			InitPrompt:       f.InitializePrompt,
			PersistentPrompt: f.PersistentPrompt,
		}, nil
	}

	arguments, err := f.AnalyzeCliParameter(userInput)
	if err != nil {
		return nil, utils.Errorf("AnalyzeCliParameter failed: %v", err)
	}
	return &ForgePromptParams{
		UserParams:       arguments.String(),
		UserQuery:        query,
		InitPrompt:       f.InitializePrompt,
		PersistentPrompt: f.PersistentPrompt,
	}, nil
}

func (f *ForgeBlueprint) ToolPrompt() string {
	if len(f.Tools) <= 0 {
		return ""
	}
	tmp, err := template.New("tool").Parse(`# 工具提示
在设计任务中，只考虑工具名称即可，具体参数在后面的对话会按需确认:
{{range .Tools}}
- "{{.Name}}": "{{.Description}}"
{{end}}`)
	if err != nil {
		log.Errorf("[ForgeBlueprint.ToolPrompt] %v", err)
		return ""
	}
	var buf bytes.Buffer
	err = tmp.Execute(&buf, map[string]any{
		"Tools": f.Tools,
	})
	if err != nil {
		log.Errorf("[ForgeBlueprint.ToolPrompt] %v", err)
		return ""
	}
	return buf.String()
}

func (f *ForgeBlueprint) tmpParams(query string, params ...*ypb.ExecParamItem) map[string]any {
	var paramBuf bytes.Buffer
	if !utils.IsNil(params) {
		for _, p := range params {
			paramBuf.WriteString(codec.StrConvQuote(p.Key))
			paramBuf.WriteString(": ")
			paramBuf.WriteString(codec.StrConvQuote(p.Value))
			paramBuf.WriteByte('\n')
		}
	}

	return map[string]any{
		"Forge": map[string]any{
			"Tool":             f.Tools,
			"UserParams":       paramBuf.String(),
			"Init":             f.InitializePrompt,
			"PersistentPrompt": f.PersistentPrompt,
			"Result":           "",
		},
	}
}

func (f *ForgeBlueprint) renderInitPrompt(query string, params ...*ypb.ExecParamItem) (string, error) {
	tmpl, err := template.New("init").Parse(f.InitializePrompt)
	if err != nil {
		log.Errorf("parse init prompt failed: %v", err)
		return "", err
	}

	forgePromptParams, err := f.Params(query, params...)
	if err != nil {
		log.Errorf("get init params failed: %v", err)
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{
		"Forge": forgePromptParams,
	}); err != nil {
		log.Errorf("execute init prompt failed: %v", err)
		return "", err
	}

	if ret := f.ToolPrompt(); ret != "" {
		buf.WriteString("\n")
		buf.WriteString(ret)
	}
	return buf.String(), nil
}

func (f *ForgeBlueprint) renderPersistentPrompt(query string) (string, error) {
	tmpl, err := template.New("persistent").Parse(f.PersistentPrompt)
	if err != nil {
		log.Errorf("parse persistent prompt failed: %v", err)
		return "", err
	}
	forgePromptParams, err := f.Params(query)
	if err != nil {
		log.Errorf("get init params failed: %v", err)
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{
		"Forge": forgePromptParams,
	}); err != nil {
		log.Errorf("execute persistent prompt failed: %v", err)
		return "", err
	}

	return buf.String(), nil
}

func (f *ForgeBlueprint) renderResultPrompt(memory *aid.Memory) (string, error) {
	tmpl, err := template.New("result").Parse(f.ResultPrompt)
	if err != nil {
		log.Errorf("parse result prompt failed: %v", err)
		return "", err
	}

	var params = make(map[string]any)
	forgePromptParams, _ := f.Params(memory.Query)
	params["Forge"] = forgePromptParams
	params["Memory"] = memory
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		log.Errorf("execute result prompt failed: %v", err)
		return "", err
	}

	return buf.String(), nil
}
