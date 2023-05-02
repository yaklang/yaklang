package yaktest

import (
	"os"
	"testing"
	"yaklang.io/yaklang/common/consts"

	"yaklang.io/yaklang/common/yakgrpc/yakit"
)

/*
manager = hook.NewMixPluginCaller()[0]
manager.LoadPlugin("Tomcat 登陆爆破")
manager.SetConcurrent(20)

loglevel("info")
yakit.Info("加载端口")
for result = range servicescan.Scan("47.52.100.104", "443,22")[0] {
    manager.GetNativeCaller().CallByName("handle", result)
}

manager.Wait()
*/

func TestMisc_RuntimeDB(t *testing.T) {
	//os.Setenv("YAKLANGDEBUG", "123")
	consts.GetGormProjectDatabase()
	consts.GetGormProjectDatabase()
	err := yakit.CallPostInitDatabase()
	if err != nil {
		panic(err)
	}
	_ = yakit.ExecResult{}
	os.Setenv(consts.CONST_YAK_SAVE_HTTPFLOW, "true")
	cases := []YakTestCase{
		{
			Name: "函数返回值失败",
			Src: `
yakit.AutoInitYakit()

manager = hook.NewMixPluginCaller()[0]
//err = manager.LoadPlugin("Tomcat 登陆爆破")
//dump(err)

err = manager.LoadPlugin("test111")
dump(err)

manager.SetConcurrent(20)
manager.SetDividedContext(true) // 设置独立上下文

loglevel("info")
for result = range servicescan.Scan("47.52.100.104", "443,22")[0] {
    manager.GetNativeCaller().CallByName("handle", result)
}

manager.Wait()
`,
		},
	}

	Run("yaklang syntax fix", t, cases...)
}
