package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestLowRiskToolReviewPolicy(t *testing.T) {
	cfg := NewKeyValueConfig()
	require.True(t, shouldSkipLowRiskToolReview(aitool.NewWithoutCallback("read_file"), nil, cfg))
	require.True(t, shouldSkipLowRiskToolReview(aitool.NewWithoutCallback("batch_do_http_request"), aitool.InvokeParams{"method": "GET"}, cfg))
	require.False(t, shouldSkipLowRiskToolReview(aitool.NewWithoutCallback("batch_do_http_request"), aitool.InvokeParams{"method": "POST", "body": "{}"}, cfg))
	require.False(t, shouldSkipLowRiskToolReview(aitool.NewWithoutCallback("bash"), nil, cfg))

	cfg.SetConfig(ConfigEnableLowRiskToolAutoApprove, false)
	require.False(t, shouldSkipLowRiskToolReview(aitool.NewWithoutCallback("read_file"), nil, cfg))
}
