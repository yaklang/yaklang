package reactloops

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestObserveToolInformationGainNormalizesDynamicCaptcha(t *testing.T) {
	invoker := mock.NewMockInvoker(context.Background())
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	blobA := strings.Repeat("A", 120)
	blobB := strings.Repeat("B", 120)
	results := []*aitool.ToolResult{
		{Success: true, Data: `{"captcha_id":"123e4567-e89b-12d3-a456-426614174000","image":"` + blobA + `","code":10001}`},
		{Success: true, Data: `{"captcha_id":"223e4567-e89b-12d3-a456-426614174111","image":"` + blobB + `","code":10002}`},
		{Success: true, Data: `{"captcha_id":"323e4567-e89b-12d3-a456-426614174222","image":"` + blobA + `","code":10003}`},
	}
	for i, result := range results {
		observation := loop.ObserveToolInformationGain("do_http_request", result)
		if i < 2 {
			require.False(t, observation.ShouldSwitch)
		} else {
			require.True(t, observation.ShouldSwitch)
			require.True(t, observation.NewlyDetected)
		}
	}
}

func TestObserveToolInformationGainResetsOnDifferentResult(t *testing.T) {
	invoker := mock.NewMockInvoker(context.Background())
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	loop.ObserveToolInformationGain("do_http_request", &aitool.ToolResult{Success: true, Data: "401 login required"})
	observation := loop.ObserveToolInformationGain("do_http_request", &aitool.ToolResult{Success: true, Data: "200 swagger document"})
	require.Equal(t, 1, observation.Consecutive)
}
