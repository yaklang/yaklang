package aitool

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestDeprecatedAIToolCachedInvocation(t *testing.T) {
	for _, name := range schema.DeprecatedAIToolNames() {
		t.Run(name, func(t *testing.T) {
			called := false
			tool, err := New(name,
				WithStringParam("file", WithParam_Required(true)),
				WithSimpleCallback(func(InvokeParams, io.Writer, io.Writer) (any, error) {
					called = true
					return nil, nil
				}))
			require.NoError(t, err)
			// Even invalid legacy arguments should receive migration guidance,
			// rather than a schema error encouraging a retry of the old tool.
			out, err := tool.InvokeWithParams(map[string]any{})
			require.ErrorContains(t, err, "load_pcapfile")
			require.False(t, out.Success)
			out, err = tool.InvokeWithOrderedParams(nil)
			require.ErrorContains(t, err, "query_pcapdb")
			require.False(t, out.Success)
			result, err := tool.ExecuteToolWithCapture(context.Background(), map[string]any{"file": "old.pcap"}, NewToolInvokeConfig())
			require.Nil(t, result)
			require.ErrorContains(t, err, "manage_pcapdb")
			require.False(t, called)
		})
	}
}
