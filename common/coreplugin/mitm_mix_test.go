package coreplugin

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGRPCMUSTPASS_MITM(t *testing.T) {
	var client, err = NewLocalClient()
	if err != nil {
		t.Fatalf("start mitm local client failed: %s", err)
	}
	OverWriteCorePluginToLocal()

	var vulinboxPort = utils.GetRandomAvailableTCPPort()
	var vulinboxAddr = utils.HostPort("127.0.0.1", vulinboxPort)
	go func() {
		v, err := vulinbox.NewVulinServerEx(context.Background(), false, false, "127.0.0.1", vulinboxPort)
		if err != nil {
			t.Fatalf("start vulinbox server failed: %s", err)
		}
		vulinboxAddr = v
	}()
	err = utils.WaitConnect(vulinboxAddr, 5)
	if err != nil {
		panic(err)
	}
	t.Logf("vulinbox server started: %s", vulinboxAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatalf("start mitm stream failed: %s", err)
	}
	var port = utils.GetRandomAvailableTCPPort()
	var mitmProxy = fmt.Sprintf("http://127.0.0.1:" + fmt.Sprint(port))
	err = stream.Send(&ypb.MITMRequest{
		Host:             "127.0.0.1",
		Port:             uint32(port),
		Recover:          true,
		EnableHttp2:      false,
		SetAutoForward:   true,
		AutoForwardValue: true,
		InitPluginNames: []string{
			"基础 XSS 检测",
		},
	})
	if err != nil {
		t.Fatalf("send mitm request failed: %s", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)

	var (
		started            bool
		pluginStartLoading bool
		pluginLoaded       bool
		vulnFound          bool
	)

	for {
		var rsp, err = stream.Recv()
		if err != nil {
			break
		}

		if strings.Contains(spew.Sdump(rsp.Message), "starting mitm server") && !started {
			fmt.Println("---------------------------------------------")
			fmt.Println("---------------------------------------------")
			fmt.Println("---------------------------------------------")
			fmt.Println("---------------------------------------------")
			fmt.Println("---------------------------------------------")
			started = true
			err = stream.Send(&ypb.MITMRequest{
				SetPluginMode: true,
				InitPluginNames: []string{
					"基础 XSS 检测",
				},
			})
			if err != nil {
				t.Fatalf("send mitm request failed: %s", err)
			}
		}

		if !pluginStartLoading && started && strings.Contains(spew.Sdump(rsp), "Initializing MITM Plugin: 基础 XSS 检测") {
			pluginStartLoading = true
		}
		if pluginStartLoading && strings.Contains(spew.Sdump(rsp), "初始化加载插件完成，加载成功【1】个") {
			fmt.Println("==============================================")
			fmt.Println("==============================================")
			fmt.Println("==============================================")
			fmt.Println("==============================================")
			fmt.Println("==============================================")
			go func() {
				defer func() {
					cancel()
					wg.Done()
				}()

				packet := lowhttp.ReplaceHTTPPacketHeader([]byte(`GET /xss/attr/src?src=/static/logo.png HTTP/1.1
Host: 111

`), "Host", vulinboxAddr)
				params := map[string]any{
					"packet":        packet,
					"proxy":         mitmProxy,
					"vulinbox_addr": vulinboxAddr,
				}
				_, err := yak.NewScriptEngine(10).ExecuteEx(`
packet = getParam("packet")
proxy = getParam("proxy")

dump(packet)
dump(proxy)

rsp, req = poc.HTTP(packet, poc.proxy(proxy), poc.https(true))~
println(string(rsp))
sleep(1)

vulinboxAddr  = getParam("vulinbox_addr")
count = 0
risks = []
dump(vulinboxAddr)
haveVuls = false
for {
	count++
	for result in risk.YieldRiskByTarget(vulinboxAddr) {
		haveVuls = true
		risks.Push(result)
		println(result.Url)
	}
	if risks.Length() > 0 {
		break
	}
	if count > 10 {
		break
	}
	sleep(1)
}
if risks == nil { die("no vulns found") }
if !haveVuls {
	die("no vulns found (!haveVuls)")
}
risk.DeleteRiskByTarget(vulinboxAddr)


`, params)
				if err != nil {
					panic(err)
				}
				vulnFound = true
			}()
		}
		spew.Dump(rsp)
	}
	wg.Wait()
	if !started {
		panic("MITM CANNOT START UP!!!")
	}

	if !pluginLoaded {
		panic("XSS PLUGIN LOADED")
	}

	if !vulnFound {
		panic("XSS VULN CANNOT FOUND VIA MITM PLUGIN")
	}
}
