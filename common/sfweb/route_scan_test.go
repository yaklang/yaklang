package sfweb_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/sfweb"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func GetScanHTTPRequest() []byte {
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
	t.Parallel()
	scanContent(t, "java", scanFileContent)

}

func scanContent(t *testing.T, lang, content string) {
	progress := 0.0
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
			if rsp.Progress > 0 {
				progress = rsp.Progress
			}
			if len(rsp.Risk) > 0 {
				risks = append(risks, rsp.Risk...)
			}
		}),
	)
	require.NoError(t, err)
	now := time.Now()
	err = writeJSON(wc, &sfweb.SyntaxFlowScanRequest{
		Content:        content,
		Lang:           lang,
		ControlMessage: `start`,
		TimeoutSecond:  40,
	})

	/*
		--- PASS: TestTemplate/check_template_all (89.50s)
		--- PASS: TestTemplate/check_template_all/templates/golang/cwe-79-xss-unsafe (0.92s)
		--- PASS: TestTemplate/check_template_all/templates/golang/cwe-89-sql-injection-gin-unsafe (1.38s)
		--- PASS: TestTemplate/check_template_all/templates/golang/cwe-89-sql-injection-net-unsafe (1.66s)
		--- PASS: TestTemplate/check_template_all/templates/golang/cwe-90-ldqp-injection-unsafe (1.52s)
		--- PASS: TestTemplate/check_template_all/templates/golang/cwe-918-ssrf-unsafe (1.57s)
		--- PASS: TestTemplate/check_template_all/templates/java/cwe-1336-ssti.java (6.24s)
		--- PASS: TestTemplate/check_template_all/templates/java/cwe-22-path-travel.java (25.55s)
		--- PASS: TestTemplate/check_template_all/templates/java/cwe-434-unrestricted-upload-file-1.java (2.95s)
		--- PASS: TestTemplate/check_template_all/templates/java/cwe-434-unrestricted-upload-file-2.java (8.85s)
		--- PASS: TestTemplate/check_template_all/templates/java/cwe-502-untrusted-unserialization.java (2.13s)
		--- PASS: TestTemplate/check_template_all/templates/java/cwe-611-xxe.java (2.28s)
		--- PASS: TestTemplate/check_template_all/templates/java/cwe-77-command-injection-1.java (4.12s)
		--- PASS: TestTemplate/check_template_all/templates/java/cwe-77-command-injection-2.java (3.22s)
		--- PASS: TestTemplate/check_template_all/templates/java/cwe-79-xss.java (2.84s)
		--- PASS: TestTemplate/check_template_all/templates/java/cwe-89-sql-injection-unsafe-query-statement-concat.java (6.40s)
		--- PASS: TestTemplate/check_template_all/templates/java/cwe-918-ssrf.java (2.47s)
		--- PASS: TestTemplate/check_template_all/templates/php/cwe-502-unserialize.php (3.80s)
		--- PASS: TestTemplate/check_template_all/templates/php/cwe-611-xxe.php (1.02s)
		--- PASS: TestTemplate/check_template_all/templates/php/cwe-73-unfiltered-file-or-path.php (2.22s)
		--- PASS: TestTemplate/check_template_all/templates/php/cwe-77-common-injection.php (1.55s)
		--- PASS: TestTemplate/check_template_all/templates/php/cwe-78-os-command-injection.php (1.72s)
		--- PASS: TestTemplate/check_template_all/templates/php/cwe-79-xss.php (2.06s)
		--- PASS: TestTemplate/check_template_all/templates/php/cwe-89-sql-injection.php (1.87s)
		--- PASS: TestTemplate/check_template_all/templates/php/cwe-90-ldap-injection.php (1.13s)
	*/
	require.NoError(t, err)

	wc.Start()
	wc.Wait()

	if len(risks) > 0 {
		t.Cleanup(func() {
			ssadb.DeleteProgram(ssadb.GetDB(), risks[0].ProgramName)
		})
	}

	require.Equal(t, 1.0, progress)
	require.GreaterOrEqual(t, len(risks), 1)
	require.GreaterOrEqual(t, risks[0].Timestamp, now.Unix(), "timestamp should be >= time that scan started")
}
