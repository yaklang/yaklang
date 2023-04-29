package yaktest

import "testing"

func TestHookCaller(t *testing.T) {
	Run("测试加载插件", t, YakTestCase{
		Name: "测试加载插件崩溃",
		Src: `
m = hook.NewMixPluginCaller()[0]
m.SetDividedContext(true)

err =  m.LoadPlugin("sleep3")
die(err)

start = time.Now().Unix()
m.SetConcurrent(2)
m.HandleServiceScanResult(result)
m.HandleServiceScanResult(result)
m.HandleServiceScanResult(result)
m.HandleServiceScanResult(result)
m.Wait()
du = time.Now().Unix() - start
if du >= 7 {
    panic("concurrent panic")
}




`,
	})
}
