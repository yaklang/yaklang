package interaction

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClarificationFormContractAndAnswers(t *testing.T) {
	var form QuestionForm
	require.NoError(t, json.Unmarshal([]byte(`{"title":"部署参数","fieldOrder":["retries","name","enabled","focus"],"schema":{"type":"object","properties":{"name":{"type":"string","title":"姓名","minLength":1},"retries":{"type":"integer","minimum":1,"maximum":5,"default":3},"enabled":{"type":"boolean"},"focus":{"type":"array","minItems":1,"items":{"type":"string","enum":["数据流","模块"]}}},"required":["name","retries","enabled","focus"]}}`), &form))
	require.NoError(t, form.Validate())
	require.Equal(t, false, form.Schema["additionalProperties"])
	values := map[string]any{"name": "小明", "retries": float64(3), "enabled": true, "focus": []any{"数据流"}}
	text, err := form.AnswerText(values)
	require.NoError(t, err)
	require.Contains(t, text, "姓名：小明")
	require.Contains(t, text, "retries：3")
	for _, bad := range []map[string]any{nil, {}, {"name": "小明", "retries": 6, "enabled": true, "focus": []any{"数据流"}}, {"name": "小明", "retries": 3, "enabled": true, "focus": []any{"数据流", "数据流"}}, {"name": "小明", "retries": 3, "enabled": true, "focus": []any{}}, {"name": "小明", "retries": 3, "enabled": true, "focus": []any{"数据流"}, "admin": true}} {
		_, err := form.AnswerText(bad)
		require.Error(t, err)
	}
	require.Error(t, (&QuestionRequest{Form: &form, Questions: []Question{{ID: "also"}}}).Validate())
}

func TestClarificationFormRejectsInvalidDefinitions(t *testing.T) {
	for _, schema := range []string{
		`{"type":"object","properties":{"name":{"type":"array"}}}`,
		`{"type":"object","properties":{"name":{"type":"string"}},"required":["missing"]}`,
		`{"type":"object","properties":{"name":{"type":"number","minimum":10,"maximum":1}}}`,
		`{"type":"object","properties":{"name":{"type":"integer","default":"wrong"}}}`,
		`{"type":"object","properties":{"name":{"$ref":"https://example.com"}}}`,
	} {
		var fields map[string]any
		require.NoError(t, json.Unmarshal([]byte(schema), &fields))
		require.Error(t, (&QuestionForm{Title: "填写参数", Schema: fields}).Validate())
	}
}
