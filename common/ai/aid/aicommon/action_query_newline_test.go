package aicommon

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearchActionMultilineQueries(t *testing.T) {
	action, err := ExtractValidActionFromStream(context.Background(), strings.NewReader(`{
  "@action": "search_logs",
  "queries":
  ["ERROR", "WARN"],
  "offset": 0
}`), "search_logs")
	require.NoError(t, err)
	require.Equal(t, []string{"ERROR", "WARN"}, action.GetStringSlice("queries"))
}
