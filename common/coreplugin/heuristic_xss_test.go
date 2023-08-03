package coreplugin

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"testing"
)

func TestGRPCMUSTPASS_XSS(t *testing.T) {
	client, err := yakgrpc.NewLocalClient()
	if err != nil {
		panic(err)
	}

	vulAddr, err := vulinbox.NewVulinServer(context.Background())
	if err != nil {
		panic(err)
	}

	pluginName := "基础 XSS 检测"
	server := VulServerInfo{
		VulServerAddr: vulAddr,
		IsHttps:       true,
	}

	vul := VulInfo{
		Path: []string{
			"/xss/safe?name=admin",
			"/xss/echo?name=admin",
			"/xss/replace/nocase?name=admin",
			"/xss/js/in-str?name=admin",
			"/xss/js/in-str2?name=admin",
			"/xss/js/in-str-temp?name=admin",
			"/xss/attr/onclick?code=2-1",
			"/xss/attr/onclick2?code=2-1",
			`/xss/attr/alt/json?json={"value":"value=visitor-name"}`,
			`/xss/attr/alt/b64/json?b64json=eyJ2YWx1ZSI6InZhbHVlPXZpc2l0b3ItbmFtZSJ9`,
			`/xss/attr/src?src=/static/logo.png`,
			`/xss/attr/href?href=/static/logo.png`,
			`/xss/cookie/name`,
			`/xss/cookie/b64/json/name`,
		},
		ExpectedResult: map[string]int{
			fmt.Sprintf("XSS for: %s/xss/echo?name=admin", vulAddr):           1,
			fmt.Sprintf("XSS for: %s/xss/replace/nocase?name=admin", vulAddr): 1,
			fmt.Sprintf("XSS for: %s/xss/js/in-str?name=admin", vulAddr):      1,
			fmt.Sprintf("XSS for: %s/xss/js/in-str2?name=admin", vulAddr):     1,
			fmt.Sprintf("XSS for: %s/xss/js/in-str-temp?name=admin", vulAddr): 1,
			fmt.Sprintf("XSS for: %s/xss/attr/onclick?code=2-1", vulAddr):     1,
			fmt.Sprintf(`XSS for: %s/xss/attr/onclick2?code=2-1`, vulAddr):    1,
			fmt.Sprintf(`XSS for: %s/xss/attr/alt/json`, vulAddr):             1,
			fmt.Sprintf(`XSS for: %s/xss/attr/alt/b64/json`, vulAddr):         1,
			fmt.Sprintf(`XSS for: %s/xss/attr/src`, vulAddr):                  1,
			fmt.Sprintf(`XSS for: %s/xss/attr/href`, vulAddr):                 1,
			fmt.Sprintf(`XSS for: %s/xss/cookie/name`, vulAddr):               1,
			fmt.Sprintf(`XSS for: %s/xss/cookie/b64/json/name`, vulAddr):      1,
		},
		StrictMode: false,
	}

	Must(TestCoreMitmPlug(pluginName, server, vul, client, t), " ")

}
