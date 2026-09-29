package aitool

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// RenderUsageForMode uses delimiters reserved for Usage templates. Literal
// payload markers such as {{PATH}} and shell expressions such as [[ -f ... ]]
// remain untouched; FunctionCallMode is the only template data field.
func RenderUsageForMode(usage string, functionCallMode bool) (string, error) {
	tmpl, err := template.New("tool-usage").Delims("[[-", "-]]").Option("missingkey=error").Parse(usage)
	if err != nil {
		return "", fmt.Errorf("parse Usage template: %w", err)
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, struct{ FunctionCallMode bool }{FunctionCallMode: functionCallMode}); err != nil {
		return "", fmt.Errorf("render Usage template: %w", err)
	}
	return strings.TrimSpace(out.String()), nil
}
