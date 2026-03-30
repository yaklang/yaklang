package test

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

const scanPortToolName = "scan_port"

func getScanPortTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/pentest/scan_port.yak")
	if err != nil {
		t.Fatalf("failed to read scan_port.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(scanPortToolName, string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse scan_port.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty, toolCovertHandle may not be registered")
	}
	return tools[0]
}

func isReachableTCP(target string, ports []string) bool {
	for _, port := range ports {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(target, port), 3*time.Second)
		if err == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}

func TestScanPortTool_SynMode_UsesSynscanxForPointToPointRoute(t *testing.T) {
	if testing.Short() {
		t.Skip("skip external scan_port integration test in short mode")
	}
	if os.Getenv("CI") != "" {
		t.Skip("skip external scan_port integration test in CI")
	}

	const target = "10.129.220.92"
	if !isReachableTCP(target, []string{"21", "22", "80"}) {
		t.Skip("target host is not reachable from current environment")
	}

	tool := getScanPortTool(t)
	w1, w2 := strings.Builder{}, strings.Builder{}
	_, err := tool.Callback(context.Background(), aitool.InvokeParams{
		"hosts":         target,
		"ports":         "21,22,80",
		"mode":          "syn",
		"concurrent":    200,
		"fp-concurrent": 20,
		"active":        true,
		"web":           true,
	}, nil, &w1, &w2)
	if err != nil {
		t.Fatalf("scan_port tool returned error: %v", err)
	}

	stdout := w1.String()
	stderr := w2.String()
	combined := stdout + "\n" + stderr

	assert.Assert(t, !strings.Contains(combined, "assemble arp packet failed"), "unexpected arp assembly failure:\n%s", combined)
	assert.Assert(t, !strings.Contains(combined, "invalid MAC address"), "unexpected invalid MAC failure:\n%s", combined)
	assert.Assert(t, !strings.Contains(combined, "SetBPFFilter failed"), "unexpected BPF setup failure:\n%s", combined)
	assert.Assert(t, strings.Contains(combined, target), "expected target host in output:\n%s", combined)
	assert.Assert(t, strings.Contains(combined, "open:"), "expected open port results in output:\n%s", combined)
	assert.Assert(t, strings.Contains(combined, "ftp") || strings.Contains(combined, target+":21"), "expected ftp/21 evidence in output:\n%s", combined)
	assert.Assert(t, strings.Contains(combined, "ssh") || strings.Contains(combined, target+":22"), "expected ssh/22 evidence in output:\n%s", combined)
	assert.Assert(t, strings.Contains(combined, "http") || strings.Contains(combined, target+":80"), "expected http/80 evidence in output:\n%s", combined)
	assert.Assert(t, strings.Contains(combined, "scan completed"), "expected completion marker in output:\n%s", combined)
}
