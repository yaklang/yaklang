package sfweb_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/sfweb"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

var fileContent = `import org.springframework.expression.ExpressionParser;
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

func TestScan(t *testing.T) {
	packet := []byte(fmt.Sprintf(`GET /scan HTTP/1.1
Host: %s
Sec-WebSocket-Version: 13
Sec-WebSocket-Key: wDqumtseNBJdhkihL6PW7w==
Connection: keep-alive, Upgrade
Upgrade: websocket
`, serverAddr))
	writeJSON := func(wc *lowhttp.WebsocketClient, data any) error {
		msg, err := json.Marshal(data)
		if err != nil {
			return err
		}
		return wc.Write(msg)
	}
	progress := 0.0

	wc, err := lowhttp.NewWebsocketClient(
		packet,
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
				t.Logf("Progress: %v", rsp.Progress)
				progress = rsp.Progress
			}
		}),
	)
	require.NoError(t, err)

	err = writeJSON(wc, &sfweb.SyntaxFlowScanRequest{
		Content:        fileContent,
		Lang:           `java`,
		ControlMessage: `start`,
	})
	require.NoError(t, err)

	wc.Start()
	wc.Wait()
	require.Equal(t, 1.0, progress)
}
