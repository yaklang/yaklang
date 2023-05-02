package yaktest

import (
	"fmt"
	"os"
	"testing"
	"yaklang.io/yaklang/common/utils"
)

func TestMisc_YAKIT(t *testing.T) {
	randomPort := utils.GetRandomAvailableTCPPort()
	utils.NewWebHookServer(randomPort, func(data interface{}) {

	})

	cases := []YakTestCase{
		{
			Name: "测试 yakit.File",
			Src:  fmt.Sprintf(`yakit.File("/etc/hosts", "HOSTS", "this is hosts")`),
		},
	}

	Run("yakit.File 可用性测试", t, cases...)
}

func TestMisc_YAKIT2(t *testing.T) {
	os.Setenv("YAKMODE", "vm")
	randomPort := utils.GetRandomAvailableTCPPort()
	utils.NewWebHookServer(randomPort, func(data interface{}) {

	})

	cases := []YakTestCase{
		{
			Name: "测试 ",
			Src: `println()))
a = 123;
a()
`,
		},
	}

	Run("))))))测试", t, cases...)
}
