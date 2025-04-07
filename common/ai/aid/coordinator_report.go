package aid

import (
	"bytes"
	"github.com/yaklang/yaklang/common/utils"
	"text/template"
)

func (r *Coordinator) generateReport() (string, error) {
	params := map[string]any{
		"Memory": r.config.memory,
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
