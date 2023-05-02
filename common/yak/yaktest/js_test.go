package yaktest

import (
	"testing"
	"yaklang/common/utils"
)

func TestMisc_JS(t *testing.T) {
	randomPort := utils.GetRandomAvailableTCPPort()
	utils.NewWebHookServer(randomPort, func(data interface{}) {

	})

	jsCode1 := `// Sample xyzzy example
    (function(){
        if (3.14159 > 0) {
            console.log("Hello, World.");
            return;
        }

        var xyzzy = NaN;
        console.log("Nothing happens.");
        return xyzzy;
    })();`

	jsCode2 := `// Sample xyzzy example
    function test(){
        if (3.14159 > 0) {
            console.log("Call By Function!!!!!!!!!!!!.");
            return "CallByFunc";
        }

        var xyzzy = NaN;
        console.log("Nothing happens.");
        return xyzzy;
    }`
	cases := []YakTestCase{
		{
			Name: "测试 js",
			Src:  `die(js.Run("1+1")[2])`,
		},
		{
			Name: "测试闭包函数 js",
			Src:  "vm, value, err = js.Run(`" + jsCode1 + "`); die(err); dump(value)",
		},
		{
			Name: "测试函数定义执行 js",
			Src:  "value, err = js.CallFunctionFromCode(`" + jsCode2 + "`, `test`); die(err); dump(value)",
		},
	}

	Run("JS OTTO 可用性测试", t, cases...)
}
