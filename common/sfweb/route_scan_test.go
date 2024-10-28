package sfweb_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/sfweb"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func GetScanRequest() []byte {
	return []byte(fmt.Sprintf(`GET /scan HTTP/1.1
Host: %s
Sec-WebSocket-Version: 13
Sec-WebSocket-Key: wDqumtseNBJdhkihL6PW7w==
Connection: keep-alive, Upgrade
Upgrade: websocket
`, serverAddr))
}

var scanFileContent = `import org.springframework.expression.ExpressionParser;
import org.springframework.expression.spel.standard.SpelExpressionParser;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RestController;

@RestController
public class SpelInjectionController {

	private static final ExpressionParser parser = new SpelExpressionParser();

	@PostMapping("/evaluate")
	public String evaluate(@RequestBody String expression) {
		// 直接使用用户输入的表达式
		return parser.parseExpression(expression).getValue().toString();
	}
}`

func writeJSON(wc *lowhttp.WebsocketClient, data any) error {
	msg, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return wc.Write(msg)
}

func TestScan(t *testing.T) {
	progress := 0.0
	var risks []*sfweb.SyntaxFlowScanRisk

	wc, err := lowhttp.NewWebsocketClient(
		GetScanRequest(),
		lowhttp.WithWebsocketFromServerHandlerEx(func(wc *lowhttp.WebsocketClient, b []byte, f []*lowhttp.Frame) {
			var rsp sfweb.SyntaxFlowScanResponse
			err := json.Unmarshal(b, &rsp)
			require.NoError(t, err)
			if rsp.Error != "" {
				t.Logf("Error: %v", rsp.Error)
			} else if rsp.Message != "" {
				t.Logf("Info: %v", rsp.Message)
			}
			if rsp.Progress > 0 {
				progress = rsp.Progress
			}
			if len(rsp.Risk) > 0 {
				risks = append(risks, rsp.Risk...)
			}
		}),
	)
	require.NoError(t, err)

	err = writeJSON(wc, &sfweb.SyntaxFlowScanRequest{
		Content:        scanFileContent,
		Lang:           `java`,
		ControlMessage: `start`,
	})
	require.NoError(t, err)

	wc.Start()
	wc.Wait()
	require.GreaterOrEqual(t, len(risks), 1)
	require.Equal(t, 1.0, progress)
}
