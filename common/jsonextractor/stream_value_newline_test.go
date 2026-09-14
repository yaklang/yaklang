package jsonextractor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestObjectValueLeadingNewline(t *testing.T) {
	for _, value := range []string{`["ERROR","WARN"]`, `{"nested":[1,2]}`, `"text"`, `42`, `true`, `null`} {
		for _, whitespace := range []string{"\n", " \r\n\t\n"} {
			t.Run(value+whitespace, func(t *testing.T) {
				raw := "{\"@action\":\"search_logs\",\"value\":" + whitespace + value + ",\"after\":\"intact\"}"
				var got map[string]any
				err := ExtractStructuredJSONFromStream(strings.NewReader(raw), WithObjectCallback(func(v map[string]any) {
					if v["@action"] == "search_logs" {
						got = v
					}
				}))
				require.NoError(t, err)
				require.JSONEq(t, raw, mustJSON(t, got))
			})
		}
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}
