package aiforge

import (
	"bytes"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"text/template"
)

var queryPrompt = `{{.PROMPT}}

{{ if .EXTRA }}
<|EXTRA_{{ .Nonce }}|>
{{.EXTRA}}
<|EXTRA_END_{{ .Nonce }}|>
{{ end }}

{{ if .OVERLAP }}
<|OVERLAP_{{ .Nonce }}|>
{{.OVERLAP}}
<|OVERLAP_END_{{ .Nonce }}|>
{{ end }}


<|INPUT_{{ .Nonce }}|>
{{.INPUT}}
<|INPUT_END_{{ .Nonce }}|>
`

func LiteForgeQueryFromChunk(prompt string, extraPrompt string, chunk chunkmaker.Chunk, overlapSize int) (string, error) {
	param := map[string]interface{}{
		"PROMPT": prompt,
		"INPUT":  string(chunk.Data()),
		"EXTRA":  extraPrompt,
		"Nonce":  utils.RandStringBytes(4),
	}

	if overlapSize > 0 || chunk.HaveLastChunk() {
		param["OVERLAP"] = string(chunk.PrevNBytes(overlapSize))
	}
	queryTemplate, err := template.New("query").Parse(queryPrompt)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = queryTemplate.ExecuteTemplate(&buf, "query", param)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func quickQueryBuild(prompt string, input ...string) string {
	param := map[string]interface{}{
		"PROMPT": prompt,
		"INPUT":  strings.Join(input, "\n"),
	}
	queryTemplate, err := template.New("query").Parse(queryPrompt)
	if err != nil {
		log.Errorf("parse query template failed: %s", err)
		return ""
	}
	var buf bytes.Buffer
	err = queryTemplate.ExecuteTemplate(&buf, "query", param)
	if err != nil {
		log.Errorf("execute query template failed: %s", err)
		return ""
	}
	return buf.String()
}
