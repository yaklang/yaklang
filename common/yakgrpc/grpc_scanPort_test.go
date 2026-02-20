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

	const maxRetries = 3

	createFingerprintFiles := func(t *testing.T) []string {
		var fpFiles []string
		f1, err := os.CreateTemp(os.TempDir(), "yakit-test-fingerprint-*.yaml")
		require.NoError(t, err)
		t.Cleanup(func() { os.Remove(f1.Name()) })
		f1.WriteString(`- methods:
    - keywords:
        - product: 测试1
          regexp: "test CustomFingerprint1"`)
		f1.Close()
		fpFiles = append(fpFiles, f1.Name())

		f2, err := os.CreateTemp(os.TempDir(), "yakit-test-fingerprint-*.yaml")
		require.NoError(t, err)
		t.Cleanup(func() { os.Remove(f2.Name()) })
		f2.WriteString(`- methods:
    - keywords:
        - product: 测试2
          regexp: "test CustomFingerprint2"`)
		f2.Close()
		fpFiles = append(fpFiles, f2.Name())
		return fpFiles
	}

	fpFiles := createFingerprintFiles(t)

	var lastErr error
	for retry := 0; retry < maxRetries; retry++ {
		host, port := utils.DebugMockHTTP([]byte("test CustomFingerprint1,test CustomFingerprint2"))
		err := utils.WaitConnect(utils.HostPort(host, port), 5)
		if err != nil {
			lastErr = fmt.Errorf("retry %d: mock server not ready: %v", retry, err)
			continue
		}

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
		if err != nil {
			lastErr = fmt.Errorf("retry %d: PortScan RPC failed: %v", retry, err)
			continue
		}

		found1 := false
		found2 := false
		scanErr := false
		var allMessages []string
		for {
			result, err := r.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					lastErr = fmt.Errorf("retry %d: stream error: %v", retry, err)
					scanErr = true
				}
				break
			}
			msgStr := string(result.Message)
			allMessages = append(allMessages, msgStr)
			if strings.Contains(msgStr, "测试1") {
				found1 = true
			}
			if strings.Contains(msgStr, "测试2") {
				found2 = true
			}
		}

		if scanErr {
			continue
		}

		if found1 && found2 {
			return
		}
		lastErr = fmt.Errorf("retry %d: fingerprint match incomplete (found1=%v, found2=%v), last 5 messages: %v",
			retry, found1, found2, tailMessages(allMessages, 5))
	}
	require.NoError(t, lastErr, "after %d retries still failed", maxRetries)
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
		yakit.DeleteGeneralRuleByName(consts.GetGormProfileDatabase(), ruleName2)
		yakit.DeleteGeneralRuleGroupByName(consts.GetGormProfileDatabase(), []string{groupName2})
	})

	// helper function to create mock http server for each subtest
	createMockServer := func() (string, int) {
		host, port := utils.DebugMockHTTP([]byte(fmt.Sprintf(
			"HTTP 1.1 200 OK\r\nServer: nginx\r\nContent-Length: 0\r\n\r\n%s\r\n%s",
			token1,
			token2,
		)))
		// wait for server to be ready
		utils.WaitConnect(utils.HostPort(host, port), 3)
		return host, port
	}

	// max retry times for CI environment stability
	const maxRetries = 3

	t.Run("test port scan with one fingerprint group", func(t *testing.T) {
		var lastErr error
		for retry := 0; retry < maxRetries; retry++ {
			// create independent mock server for this attempt
			host, port := createMockServer()

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
			if err != nil {
				lastErr = err
				continue
			}

			checkGroup1 := false
			scanErr := false
			for {
				result, err := r.Recv()
				if err != nil {
					if !errors.Is(err, io.EOF) {
						lastErr = err
						scanErr = true
					}
					break
				}
				spew.Dump(result)
				var msg msg
				if result.IsMessage && result.GetMessage() != nil {
					err := json.Unmarshal(result.GetMessage(), &msg)
					if err != nil {
						continue
					}
					if msg.Content.Level == "json" {
						data := msg.Content.Data
						if strings.Contains(data, "fingerprint") && strings.Contains(data, ruleName1) {
							checkGroup1 = true
						}
					}
				}
			}

			if scanErr {
				continue
			}

			if checkGroup1 {
				return // test passed
			}
			lastErr = fmt.Errorf("没有发现使用group1的指纹扫描结果")
		}
		require.NoError(t, lastErr, "after %d retries still failed", maxRetries)
	})

	t.Run("test port scan with all fingerprint group", func(t *testing.T) {
		var lastErr error
		for retry := 0; retry < maxRetries; retry++ {
			// create independent mock server for this attempt
			host, port := createMockServer()

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
			if err != nil {
				lastErr = err
				continue
			}

			checkAllGroup1 := false
			checkAllGroup2 := false
			scanErr := false
			for {
				result, err := r2.Recv()
				if err != nil {
					if !errors.Is(err, io.EOF) {
						lastErr = err
						scanErr = true
					}
					break
				}
				spew.Dump(result)
				var msg msg
				if result.IsMessage && result.GetMessage() != nil {
					err := json.Unmarshal(result.GetMessage(), &msg)
					if err != nil {
						continue
					}
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

			if scanErr {
				continue
			}

			if checkAllGroup1 && checkAllGroup2 {
				return // test passed
			}
			if !checkAllGroup1 {
				lastErr = fmt.Errorf("没有发现使用group1的指纹扫描结果")
			} else {
				lastErr = fmt.Errorf("没有发现使用group2的指纹扫描结果")
			}
		}
		require.NoError(t, lastErr, "after %d retries still failed", maxRetries)
	})
}

func tailMessages(msgs []string, n int) []string {
	if len(msgs) <= n {
		return msgs
	}
	return msgs[len(msgs)-n:]
}
