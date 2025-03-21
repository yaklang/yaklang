package aid

import (
	"bytes"
	"github.com/yaklang/yaklang/common/utils"
	"text/template"
)

func (r *Coordinator) generateReport(runtime *runtime) (string, error) {
	params := map[string]any{
		"Runtime": runtime,
	}

	tmp, err := template.New(`report-finished`).Parse(__prompt_ReportFinished)
	if err != nil {
		return "", err
	}
	var prompt bytes.Buffer
	err = tmp.Execute(&prompt, params)
	if err != nil {
		return "", utils.Errorf("execute report-finished prompt build failed: %v", err)
	}
	return prompt.String(), nil
}
