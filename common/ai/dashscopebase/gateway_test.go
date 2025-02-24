package dashscopebase

import (
	"fmt"
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
)

func TestDashScopeGateway(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
		return
	}
	apiKey, err := os.ReadFile("/tmp/bailian-apikey.txt")
	if err != nil {
		t.Fatal(err)
	}
	gateway := CreateDashScopeGateway("a51e9af5a60f40c983dac6ed50dba15b")
	gateway.LoadOption(aispec.WithAPIKey(string(apiKey)))
	c, err := gateway.StructuredStream("yaklang 输出一个 gcm aes 加密程序")
	if err != nil {
		t.Fatal(err)
	}
	for i := range c {
		if i.OutputReason != "" {
			fmt.Println(i.OutputReason)
		}
		if i.OutputText != "" {
			fmt.Println(i.OutputText)
		}
	}
}
