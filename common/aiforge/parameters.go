package aiforge

import (
	"bytes"

	"github.com/yaklang/yaklang/common/utils"
)

// Parameter is an invocation value independent of transport protocols.
// Values retain their normalized textual representation; callers own validation.
type Parameter struct {
	Key   string
	Value string
}

func parametersToPromptString(params []Parameter) string {
	var buf = new(bytes.Buffer)

	var queryParams = make([]string, 0, len(params))
	var extraParams = make([]Parameter, 0, len(params))
	for _, i := range params {
		if i.Key == "query" {
			queryParams = append(queryParams, i.Value)
			continue
		}
		extraParams = append(extraParams, i)
	}

	for _, i := range queryParams {
		buf.WriteString(utils.InterfaceToString(i))
		buf.WriteString("\n")
	}

	for _, i := range extraParams {
		if i.Key == "" && i.Value == "" {
			continue
		}
		result := utils.MustRenderTemplate(`<|PARAM_{{ .Key }}_|>
{{.Value}}
<|PARAM_END_{{ .Key }}_|>
`, map[string]interface{}{"Key": i.Key, "Value": utils.InterfaceToString(i.Value)})
		if result != "" {
			buf.WriteString(result)
		}
	}
	return buf.String()
}
