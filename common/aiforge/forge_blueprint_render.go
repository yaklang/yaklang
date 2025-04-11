package aiforge

import (
	"bytes"
	"text/template"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

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

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, f.tmpParams(query, params...)); err != nil {
		log.Errorf("execute init prompt failed: %v", err)
		return "", err
	}

	return buf.String(), nil
}

func (f *ForgeBlueprint) renderPersistentPrompt(query string) (string, error) {
	tmpl, err := template.New("persistent").Parse(f.PersistentPrompt)
	if err != nil {
		log.Errorf("parse persistent prompt failed: %v", err)
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, f.tmpParams(query)); err != nil {
		log.Errorf("execute persistent prompt failed: %v", err)
		return "", err
	}

	return buf.String(), nil
}

func (f *ForgeBlueprint) renderResultPrompt(query string) (string, error) {
	tmpl, err := template.New("result").Parse(f.ResultPrompt)
	if err != nil {
		log.Errorf("parse result prompt failed: %v", err)
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, f.tmpParams(query)); err != nil {
		log.Errorf("execute result prompt failed: %v", err)
		return "", err
	}

	return buf.String(), nil
}
