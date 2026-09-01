package poc

import (
	"bytes"
	"testing"

	"github.com/yaklang/yaklang/common/utils/cli"
)

func TestRequestConfigProxyLookupDoesNotRegisterCLIParams(t *testing.T) {
	previousArgs := cli.DefaultCliApp.GetArgs()
	cli.DefaultCliApp.SetArgs([]string{"--proxy", "http://127.0.0.1:18080"})
	t.Cleanup(func() { cli.DefaultCliApp.SetArgs(previousArgs) })

	var helpBefore bytes.Buffer
	cli.DefaultCliApp.Help(&helpBefore)

	const rawRequest = "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"
	for i := 0; i < 1000; i++ {
		urlConfig, err := handleUrlAndConfig("http://example.com")
		if err != nil {
			t.Fatal(err)
		}
		if len(urlConfig.Proxy) != 1 || urlConfig.Proxy[0] != "http://127.0.0.1:18080" {
			t.Fatalf("unexpected URL proxy config: %#v", urlConfig.Proxy)
		}

		_, rawConfig, err := handleRawPacketAndConfig(rawRequest)
		if err != nil {
			t.Fatal(err)
		}
		if len(rawConfig.Proxy) != 1 || rawConfig.Proxy[0] != "http://127.0.0.1:18080" {
			t.Fatalf("unexpected raw-packet proxy config: %#v", rawConfig.Proxy)
		}
	}

	var helpAfter bytes.Buffer
	cli.DefaultCliApp.Help(&helpAfter)
	if helpAfter.String() != helpBefore.String() {
		t.Fatal("request config lookup registered process proxy as script CLI metadata")
	}
}
