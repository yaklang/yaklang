package test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestDoHTTPRequestCapturesSessionContextWithoutToolIdentity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "session evidence")
	}))
	defer server.Close()
	tool := getDoHTTPRequestTool(t)
	attempts := make([]chan lowhttp.HTTPAttempt, 2)
	results := make(chan *aitool.ToolResult, 2)
	errors := make(chan error, 2)
	for index := range attempts {
		observed := make(chan lowhttp.HTTPAttempt, 2)
		attempts[index] = observed
		callCtx := lowhttp.WithHTTPAttemptObserver(ctx, func(lowhttp.HTTPAttempt) func(lowhttp.HTTPAttempt) {
			return func(end lowhttp.HTTPAttempt) { observed <- end }
		})
		go func() {
			result, err := tool.InvokeWithParams(aitool.InvokeParams{"url": fmt.Sprintf("%s/session-%d", server.URL, index)}, aitool.WithContext(callCtx))
			results <- result
			errors <- err
		}()
	}
	for range attempts {
		require.NoError(t, <-errors)
		result := <-results
		require.True(t, result.Success)
		execution := result.Data.(*aitool.ToolExecutionResult)
		cleanupHTTPArtifacts(t, utils.InterfaceToGeneralMap(execution.Result))
	}
	for index := range attempts {
		require.Len(t, attempts[index], 1)
		attempt := <-attempts[index]
		require.Contains(t, string(attempt.Request), fmt.Sprintf("GET /session-%d HTTP/1.1", index))
		require.Empty(t, attempt.RuntimeID)
		require.Contains(t, string(attempt.Response), "session evidence")
	}
}
