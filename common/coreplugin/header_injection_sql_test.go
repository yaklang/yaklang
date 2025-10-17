package coreplugin

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/yakgrpc"
)

func TestGRPCMUSTPASS_SQLHeaderInjection(t *testing.T) {
	client, err := yakgrpc.NewLocalClient()
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	pluginName := "SQL注入-高危Header注入"
	tests := []struct {
		name           string
		path           string
		header         []*ypb.KVPair
		expectedResult map[string]int
	}{
		{
			name: "Safe ID",
			path: "/user/by-id-safe?id=1",
			header: []*ypb.KVPair{
				{
					Key:   "Referer",
					Value: fmt.Sprintf("%s/visitor/reference", server.VulServerAddr),
				},
			},
			expectedResult: map[string]int{
				"": 0,
			},
		},

		{
			name: "Referer Header Injection",
			path: "/visitor/reference",
			header: []*ypb.KVPair{
				{
					Key:   "Referer",
					Value: fmt.Sprintf("%s/visitor/reference", server.VulServerAddr),
				},
			},
			expectedResult: map[string]int{
				"SQL Injection-Dangerous HTTP Header:": 1,
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
				Method:         "POST",
			}
			Must(CoreMitmPlugTest(pluginName, server, vul, client, t))
		})
	}
}
