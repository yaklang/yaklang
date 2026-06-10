package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsHTTPFlowBuiltinTag(t *testing.T) {
	require.True(t, IsHTTPFlowBuiltinTag(HTTPFlowTagDiscarded))
	require.True(t, IsHTTPFlowBuiltinTag(HTTPFlowTagAutoFixResponse))
	require.True(t, IsHTTPFlowBuiltinTag(HTTPFlowTagResend))
	require.True(t, IsHTTPFlowBuiltinTag(HTTPFlowTagResend+"tag1"))

	require.False(t, IsHTTPFlowBuiltinTag("webfuzzer"))
	require.False(t, IsHTTPFlowBuiltinTag("custom-tag"))
}
