package aiforge

import (
	"bytes"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/log"
	"strings"
	"text/template"
)

var queryPrompt = `{{.PROMPT}}

{{ if .OVERLAP }}
<|OVERLAP|>
{{.OVERLAP}}
<|OVERLAP END|>
{{ end }}


<|INPUT|>
{{.INPUT}}
<|INPUT END|>
`

func LiteForgeQueryFromChunk(prompt string, chunk chunkmaker.Chunk, overlapSize int) (string, error) {
	param := map[string]interface{}{
		"PROMPT": prompt,
		"INPUT":  string(chunk.Data()),
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
