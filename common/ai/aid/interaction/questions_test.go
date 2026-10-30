package interaction

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUIAnswerValidation(t *testing.T) {
	q := QuestionRequest{Questions: []Question{
		{ID: "one", Title: "单选", Prompt: "选一个", Options: []Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}},
		{ID: "two", Title: "多选", Prompt: "可自述", Multiple: true, AllowFreeText: true, Options: []Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}},
	}}
	require.NoError(t, q.Validate())
	text, err := q.AnswerText([]Answer{{QuestionID: "two", Text: "我的回答"}})
	require.NoError(t, err)
	require.Contains(t, text, "未选择")
	require.Contains(t, text, "我的回答")
	text, err = q.AnswerText(nil)
	require.NoError(t, err)
	require.Empty(t, text)
	for _, answer := range []Answer{
		{QuestionID: "one", Text: "forbidden"},
		{QuestionID: "one", Selected: []string{"a", "b"}},
		{QuestionID: "two", Selected: []string{"a", "a"}},
		{QuestionID: "two", Selected: []string{"missing"}},
		{QuestionID: "missing", Text: "unknown"},
		{QuestionID: "two", Text: "cannot skip answered", Skipped: true},
	} {
		_, err := q.AnswerText([]Answer{answer})
		require.Error(t, err)
	}
}
