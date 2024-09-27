package coreplugin

import (
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/yakgrpc"
)

func TestGRPCMUSTPASS_SQLTimeBlind(t *testing.T) {
	client, err := yakgrpc.NewLocalClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	pluginName := "SQL注入-时间盲注-Sleep"
	tests := []struct {
		name           string
		path           string
		header         []*ypb.KVPair
		expectedResult map[string]int
	}{
		{
			name: "Safe ID",
			path: "/user/by-id-safe?id=1",
			expectedResult: map[string]int{
				"": 0,
			},
		},
		{
			name: "Numeric ID TimeBlind",
			path: "/user/id-time-blind?id=1",
			expectedResult: map[string]int{
				"SQL Time-Blind-Based Injection": 1,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vul := VulInfo{
				Path:           []string{tc.path},
				ExpectedResult: tc.expectedResult,
				StrictMode:     false,
				Headers:        tc.header,
			}
			Must(CoreMitmPlugTest(pluginName, server, vul, client, t))
		})
	}
}
