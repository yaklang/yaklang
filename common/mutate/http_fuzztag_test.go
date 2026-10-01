package mutate

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRenderHTTPTemplate(t *testing.T) {
	for _, tc := range []struct {
		name, template string
		variables      map[string]interface{}
		want           []string
	}{
		{"nested encoding", "{{base64({{hex({{list(a|b)}})}})}}", nil, []string{"NjE=", "NjI="}},
		{"synchronized", "{{list::row(a|b)}}={{int::row(1-2)}}", nil, []string{"a=1", "b=2"}},
		{"repeat", "{{repeat(3)}}ok", nil, []string{"ok", "ok", "ok"}},
		{"raw JSON", `{{base64({{={"a":{"b":1}}=}})}}`, nil, []string{"eyJhIjp7ImIiOjF9fQ=="}},
		{"literal variables", "{{PATH}}:{{params(value)}}", map[string]interface{}{"PATH": "/{{int(1-3)}}", "value": "{{unknown}} {{unterminated"}, []string{"/{{int(1-3)}}:{{unknown}} {{unterminated"}},
		{"array variables", "{{params(name)}}", map[string]interface{}{"name": []string{"a", "a", "b"}}, []string{"a", "a", "b"}},
		{"variable name collides with tag", "{{uuid}}/{{uuid()}}", map[string]interface{}{"uuid": "literal"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RenderHTTPTemplate(tc.template, tc.variables, 10, context.Background())
			require.NoError(t, err)
			if tc.want != nil {
				require.Equal(t, tc.want, got)
			} else {
				require.Len(t, got, 1)
				require.Contains(t, got[0], "literal/")
			}
		})
	}
}

func TestRenderHTTPTemplateRejectsBeforeReturningRows(t *testing.T) {
	for _, template := range []string{
		"{{missing}}", "{{missing(a)}}", "{{params(missing)}}", "{{param(missing)}}", "{{p(missing)}}",
		"{{base64({{unknown()}})}}", "{{int(1-4)}}", "{{repeat(4)}}", "{{list(a|b)}}{{int(1-2)}}",
		"valid{{unclosed", "{{outer{{list(a|b)}}", "{{int(1-2)}}{{broken(}}", "{{file(/tmp/wordlist)}}",
	} {
		t.Run(template, func(t *testing.T) {
			got, err := RenderHTTPTemplate(template, nil, 3, context.Background())
			require.Error(t, err)
			require.Nil(t, got)
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := RenderHTTPTemplate("{{int(1-500)}}", nil, 500, ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, got)
}

func TestRenderHTTPFieldsPairsAndPreservesBoundaries(t *testing.T) {
	rows, err := RenderHTTPFields([]string{"/{{int::row(1-2)}}", "{{list::row(a & b|c+d)}}", `{"x":{"y":1}}`, "{{params(payload)}}"}, map[string]interface{}{"payload": "\x00{{int(1-3)}}\r\n"}, 2, context.Background())
	require.NoError(t, err)
	require.Equal(t, [][]string{
		{"/1", "a & b", `{"x":{"y":1}}`, "\x00{{int(1-3)}}\r\n"},
		{"/2", "c+d", `{"x":{"y":1}}`, "\x00{{int(1-3)}}\r\n"},
	}, rows)
}
