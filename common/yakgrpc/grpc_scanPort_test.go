package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServer_PortScan(t *testing.T) {
	client, err := NewLocalClient()
	require.Nil(t, err)

	host, port := utils.DebugMockHTTP([]byte{})

	r, err := client.PortScan(context.Background(), &ypb.PortScanRequest{
		Targets:     host,
		Ports:       strconv.Itoa(port),
		Mode:        "fp",
		Proto:       []string{"tcp"},
		Concurrent:  50,
		Active:      false,
		ScriptNames: []string{},
	})
	_ = r
	require.Nil(t, err)
	for {
		result, err := r.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				require.Nilf(t, err, "stream error: %v", err)
			}
			break
		}
		spew.Dump(result)
	}
}
func TestServer_CustomFingerprint(t *testing.T) {
	client, err := NewLocalClient()
	require.Nil(t, err)

	host, port := utils.DebugMockHTTP([]byte("test CustomFingerprint"))

	f, err := os.CreateTemp(os.TempDir(), "yakit-test-fingerprint-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`- methods:
    - keywords:
        - product: 测试
          regexp: "test CustomFingerprint"`)
	f.Close()

	r, err := client.PortScan(context.Background(), &ypb.PortScanRequest{
		UserFingerprintFiles: []string{f.Name()},
		Targets:              host,
		Ports:                strconv.Itoa(port),
		Mode:                 "fingerprint",
		Proto:                []string{"tcp"},
		Concurrent:           50,
		Active:               false,
		ScriptNames:          []string{},
		SkippedHostAliveScan: true,
	})
	_ = r
	require.Nil(t, err)
	ok := false
	for {
		result, err := r.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				require.Nilf(t, err, "stream error: %v", err)
			}
			break
		}
		if strings.Contains(string(result.Message), "http/测试") {
			ok = true
		}
		spew.Dump(result)
	}
	if !ok {
		t.FailNow()
	}
}

//func TestServer_PortScanUDP(t *testing.T) {
//	client, err := NewLocalClient()
//	if err != nil {
//		panic(err)
//	}
//
//	r, err := client.PortScan(context.Background(), &ypb.PortScanRequest{
//		Targets:    "cybertunnel.run",
//		Ports:      "53",
//		Mode:       "fp",
//		Proto:      []string{"udp"},
//		Concurrent: 50,
//		Active:     true,
//	})
//	_ = r
//	if err != nil {
//		panic(err)
//	}
//	for {
//		result, err := r.Recv()
//		if err != nil {
//			break
//		}
//		spew.Dump(result)
//	}
//}

func TestServer_QueryPorts(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	testScehma := &schema.Port{
		Host:        "127.0.0.1",
		Port:        12312,
		Proto:       "http",
		ServiceType: "test",
		State:       "open",
		Fingerprint: strings.Repeat("\"HTTP/1.1 200 200 OK\\r\\nServer: nginx\\r\\nLast-Modified: Wed, 26 Apr 2017 08:03:47 GMT\\r\\nConnection: keep-alive\\r\\nAccept-Ranges: bytes\\r\\nDate: Mon, 29 Apr 2024 13:09:38 GMT\\r\\nContent-Type: text/html\\r\\nVary: Accept-Encoding\\r\\nEtag: \\\"59005463-52e\\\"\\r\\nContent-Length: 1326\\r\\n\\r\\n<!doctype html>\\n<html>\\n<head>\\n<meta charset=\\\"utf-8\\\">\\n<title>没有找到站点</title>\\n<style>\\n*{margin:0;padding:0;color:#444}\\nbody{font-size:14px;font-family:\\\"宋体\\\"}\\n.main{width:600px;margin:10% auto;}\\n.title{background: #20a53a;color: #fff;font-size: 16px;height: 40px;line-height: 40px;padding-left: 20px;}\\n.content{background-color:#f3f7f9; height:300px;border:1px dashed #c6d9b6;padding:20px}\\n.t1{border-bottom: 1px dashed #c6d9b6;color: #ff4000;font-weight: bold; margin: 0 0 20px; padding-bottom: 18px;}\\n.t2{margin-bottom:8px; font-weight:bold}\\nol{margin:0 0 20px 22px;padding:0;}\\nol li{line-height:30px}\\n</style>\\n</head>\\n\\n<body>\\n\\t<div class=\\\"main\\\">\\n\\t\\t<div class=\\\"title\\\">没有找到站点</div>\\n\\t\\t<div class=\\\"content\\\">\\n\\t\\t\\t<p class=\\\"t1\\\">您的请求在Web服务器中没有找到对应的站点！</p>\\n\\t\\t\\t<p class=\\\"t2\\\">可能原因：</p>\\n\\t\\t\\t<ol>\\n\\t\\t\\t\\t<li>您没有将此域名或IP绑定到对应站点!</li>\\n\\t\\t\\t\\t<li>配置文件未生效!</li>\\n\\t\\t\\t</ol>\\n\\t\\t\\t<p class=\\\"t2\\\">如何解决：</p>\\n\\t\\t\\t<ol>\\n\\t\\t\\t\\t<li>检查是否已经绑定到对应站点，若确认已绑定，请尝试重载Web服务；</li>\\n\\t\\t\\t\\t<li>检查端口是否正确；</li>\\n\\t\\t\\t\\t<li>若您使用了CDN产品，请尝试清除CDN缓存；</li>\\n\\t\\t\\t\\t<li>普通网站访客，请联系网站管理员；</li>\\n\\t\\t\\t</ol>\\n\\t\\t</div>\\n\\t</div>\\n</body>\\n</html>\\n\"", 20000),
		HtmlTitle:   "test-title",
		RuntimeId:   "",
		TaskName:    "test-taskname",
		From:        "111",
	}
	_, err = yak.Execute(`
res = db.SavePortFromResult(struct)~
dump(res)
`, map[string]any{
		"struct": testScehma,
	})
	if err != nil {
		panic(err)
	}
	_ = client
	ports, err := client.QueryPorts(context.Background(), &ypb.QueryPortsRequest{
		Hosts: testScehma.Host,
		Ports: fmt.Sprintf("%v", testScehma.Port),
	})
	if err != nil {
		panic(err)
	}
	for _, port := range ports.Data {
		if port.TaskName == testScehma.TaskName {
			assert.True(t, len([]byte(port.Fingerprint)) <= 30000)
		}
	}
	_, err = client.DeletePorts(context.Background(), &ypb.DeletePortsRequest{
		Hosts: testScehma.Host,
		Ports: fmt.Sprintf("%v", testScehma.Port),
	})
	if err != nil {
		panic(err)
	}
}
