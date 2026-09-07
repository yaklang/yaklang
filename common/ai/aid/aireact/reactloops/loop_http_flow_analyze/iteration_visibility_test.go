package loop_http_flow_analyze

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPFlowPromptDoesNotExposeIterationDeadline(t *testing.T) {
	text := strings.ToLower(reactiveData)
	for _, forbidden := range []string{"islastiteration", "last iteration", "final iteration", "最后一次迭代", "最后一轮"} {
		require.NotContains(t, text, strings.ToLower(forbidden))
	}
}
