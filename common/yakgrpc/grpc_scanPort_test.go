package yakgrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

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

	host, port := utils.DebugMockHTTP([]byte("test CustomFingerprint1,test CustomFingerprint2"))

	utils.WaitConnect(utils.HostPort(host, port), 3)

	fpFiles := []string{}
	f, err := os.CreateTemp(os.TempDir(), "yakit-test-fingerprint-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`- methods:
    - keywords:
        - product: 测试1
          regexp: "test CustomFingerprint1"`)
	f.Close()
	fpFiles = append(fpFiles, f.Name())
	f, err = os.CreateTemp(os.TempDir(), "yakit-test-fingerprint-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`- methods:
    - keywords:
        - product: 测试2
          regexp: "test CustomFingerprint2"`)
	f.Close()
	fpFiles = append(fpFiles, f.Name())

	r, err := client.PortScan(context.Background(), &ypb.PortScanRequest{
		UserFingerprintFiles: fpFiles,
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
		if strings.Contains(string(result.Message), "http/测试1/测试2") || strings.Contains(string(result.Message), "http/测试2/测试1") {
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

func TestServer_ScanWithFingerprintGroup(t *testing.T) {
	// 创建指纹扫描规则并加入组
	client, err := NewLocalClient()
	require.NoError(t, err)

	token1 := uuid.NewString()
	token2 := uuid.NewString()
	ruleExpr1 := fmt.Sprintf("body=\"%s\"", token1)
	ruleExpr2 := fmt.Sprintf("body=\"%s\"", token2)

	ruleName1 := "rule1" + uuid.NewString()
	ruleName2 := "rule2" + uuid.NewString()
	groupName1 := "group1" + uuid.NewString()
	groupName2 := "group2" + uuid.NewString()

	_, err = client.CreateFingerprint(context.Background(), &ypb.CreateFingerprintRequest{
		Rule: &ypb.FingerprintRule{
			RuleName:        ruleName1,
			CPE:             nil,
			WebPath:         "",
			ExtInfo:         "",
			MatchExpression: ruleExpr1,
			GroupName:       []string{groupName1},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		yakit.DeleteGeneralRuleByName(consts.GetGormProfileDatabase(), ruleName1)
		yakit.DeleteGeneralRuleGroupByName(consts.GetGormProfileDatabase(), []string{groupName1})
	})

	_, err = client.CreateFingerprint(context.Background(), &ypb.CreateFingerprintRequest{
		Rule: &ypb.FingerprintRule{
			RuleName:        ruleName2,
			CPE:             nil,
			WebPath:         "",
			ExtInfo:         "",
			MatchExpression: ruleExpr2,
			GroupName:       []string{groupName2},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		yakit.DeleteGeneralRuleByName(consts.GetGormProfileDatabase(), ruleName1)
		yakit.DeleteGeneralRuleGroupByName(consts.GetGormProfileDatabase(), []string{groupName2})
	})

	// mock http server
	host, port := utils.DebugMockHTTP([]byte(fmt.Sprintf(
		"HTTP 1.1 200 OK\r\nServer: nginx\r\nContent-Length: 0\r\n\r\n%s\r\n%s",
		token1,
		token2,
	)))

	t.Run("test port scan with one fingerprint group", func(t *testing.T) {
		// port scan with one group
		r, err := client.PortScan(context.Background(), &ypb.PortScanRequest{
			Targets:                host,
			Ports:                  strconv.Itoa(port),
			Mode:                   "fingerprint",
			Proto:                  []string{"tcp"},
			Concurrent:             50,
			Active:                 false,
			ScriptNames:            []string{},
			SkippedHostAliveScan:   true,
			FingerprintGroup:       []string{groupName1},
			EnableFingerprintGroup: true,
		})
		_ = r
		require.Nil(t, err)
		checkGroup1 := false
		for {
			result, err := r.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					require.Nilf(t, err, "stream error: %v", err)
				}
				break
			}
			spew.Dump(result)
			var msg msg
			if result.IsMessage && result.GetMessage() != nil {
				err := json.Unmarshal(result.GetMessage(), &msg)
				require.NoError(t, err)
				if msg.Content.Level == "json" {
					data := msg.Content.Data
					if strings.Contains(data, "fingerprint") && strings.Contains(data, ruleName1) {
						checkGroup1 = true
					}
				}
			}
		}
		require.True(t, checkGroup1, "没有发现使用group1的指纹扫描结果")
	})

	t.Run("test port scan with all fingerprint group", func(t *testing.T) {

		// port scan with all group
		r2, err := client.PortScan(context.Background(), &ypb.PortScanRequest{
			Targets:                host,
			Ports:                  strconv.Itoa(port),
			Mode:                   "fingerprint",
			Proto:                  []string{"tcp"},
			Concurrent:             50,
			Active:                 false,
			ScriptNames:            []string{},
			SkippedHostAliveScan:   true,
			FingerprintGroup:       []string{}, //传空使用所有组
			EnableFingerprintGroup: true,
		})
		require.Nil(t, err)
		checkAllGroup1 := false
		checkAllGroup2 := false
		for {
			result, err := r2.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					require.Nilf(t, err, "stream error: %v", err)
				}
				break
			}
			spew.Dump(result)
			var msg msg
			if result.IsMessage && result.GetMessage() != nil {
				err := json.Unmarshal(result.GetMessage(), &msg)
				require.NoError(t, err)
				if msg.Content.Level == "json" {
					data := msg.Content.Data
					if strings.Contains(data, "fingerprint") && strings.Contains(data, ruleName1) {
						checkAllGroup1 = true
					}
					if strings.Contains(data, "fingerprint") && strings.Contains(data, ruleName2) {
						checkAllGroup2 = true
					}
				}
			}
		}
		require.True(t, checkAllGroup1, "没有发现使用group1的指纹扫描结果")
		require.True(t, checkAllGroup2, "没有发现使用group2的指纹扫描结果")
	})
}
