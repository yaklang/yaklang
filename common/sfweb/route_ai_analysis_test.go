package sfweb_test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/sfweb"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func GetAIAnalysisHTTPRequest() []byte {
	return []byte(fmt.Sprintf(`GET /ai_analysis HTTP/1.1
Host: %s
Sec-WebSocket-Version: 13
Sec-WebSocket-Key: wDqumtseNBJdhkihL6PW7w==
Connection: keep-alive, Upgrade
Upgrade: websocket
`, serverAddr))
}

func TestAIAnalysis(t *testing.T) {
	t.Parallel()

	key := os.Getenv(sfweb.CHAT_GLM_API_KEY)
	if key == "" {
		t.Skip("missing CHAT_GLM_API_KEY from env")
		return
	}

	// scan
	var risks []*sfweb.SyntaxFlowScanRisk
	wc, err := lowhttp.NewWebsocketClient(
		GetScanHTTPRequest(),
		lowhttp.WithWebsocketFromServerHandlerEx(func(wc *lowhttp.WebsocketClient, b []byte, f []*lowhttp.Frame) {
			var rsp sfweb.SyntaxFlowScanResponse
			err := json.Unmarshal(b, &rsp)
			require.NoError(t, err)
			if rsp.Error != "" {
				t.Logf("Error: %v", rsp.Error)
			} else if rsp.Message != "" {
				t.Logf("Info: %v", rsp.Message)
			}
			if len(rsp.Risk) > 0 {
				risks = append(risks, rsp.Risk...)
			}
		}),
	)
	err = writeJSON(wc, &sfweb.SyntaxFlowScanRequest{
		Content:        scanFileContent,
		Lang:           `java`,
		ControlMessage: `start`,
		TimeoutSecond:  15, // 将超时从默认的180秒减少到15秒
	})
	require.NoError(t, err)

	wc.Start()
	wc.Wait()
	require.GreaterOrEqual(t, len(risks), 1, "no risks found")

	// ai analysis
	wc, err = lowhttp.NewWebsocketClient(
		GetAIAnalysisHTTPRequest(),
		lowhttp.WithWebsocketFromServerHandlerEx(func(wc *lowhttp.WebsocketClient, b []byte, f []*lowhttp.Frame) {
			var rsp sfweb.SyntaxFlowAIAnalysisResponse
			err := json.Unmarshal(b, &rsp)
			require.NoError(t, err)
			if rsp.Error != "" {
				t.Logf("Error: %v", rsp.Error)
			} else if rsp.Message != "" {
				fmt.Fprint(os.Stdout, rsp.Message)
			}
		}),
	)
	require.NoError(t, err)

	risk := risks[0]
	err = writeJSON(wc, &sfweb.SyntaxFlowAIAnalysisRequest{
		Lang:     `java`,
		ResultID: int64(risk.ResultID),
		VarName:  risk.VarName,
	})
	require.NoError(t, err)

	wc.Start()
	wc.Wait()
}
