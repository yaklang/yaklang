package yaktest

import (
	"fmt"
	"yaklang/common/consts"
	"yaklang/common/utils"
	"yaklang/common/yakgrpc/yakit"
	"testing"
)

func TestMisc_Hook(t *testing.T) {
	randomPort := utils.GetRandomAvailableTCPPort()
	utils.NewWebHookServer(randomPort, func(data interface{}) {

	})

	s := &yakit.YakScript{
		ScriptName: "yakit-plugin-test-abcccc",
		Type:       "testtype",
		Content:    `clear = func() {println("Hello World")}`,
	}
	yakit.CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), s.ScriptName, s)
	defer yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), s.ScriptName)

	cases := []YakTestCase{
		{
			Name: "测试 hook",
			Src: fmt.Sprintf(`
a = hook.NewManager()
err = hook.LoadYakitPlugin(a, "asdfhuiasdhfhasdf", "clear")
if err == nil {
	die("load failed")
}
`),
		},
		{Name: "测试 hooks，已知插件", Src: `
a = hook.NewManager()
err = hook.LoadYakitPlugin(a, "testtype", "clear")
if err != nil {
    die(err)
}

a.CallByName("clear")
`},
	}

	Run("hooks 测试", t, cases...)
}
