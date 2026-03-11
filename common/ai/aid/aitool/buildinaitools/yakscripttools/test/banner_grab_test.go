package test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

const bannerGrabToolName = "banner_grab"

func getBannerGrabTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/pentest/banner_grab.yak")
	if err != nil {
		t.Fatalf("failed to read banner_grab.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(bannerGrabToolName, string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse banner_grab.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty")
	}
	return tools[0]
}

func execBannerGrabTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String()
}

func startMockTCPServer(t *testing.T, banner string) (string, int, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock TCP server: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				c.Write([]byte(banner))
				time.Sleep(100 * time.Millisecond)
			}(conn)
		}
	}()

	return addr.IP.String(), addr.Port, func() { ln.Close() }
}

func startMockEchoServer(t *testing.T) (string, int, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock echo server: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				n, err := c.Read(buf)
				if err != nil {
					return
				}
				c.Write([]byte("ECHO: "))
				c.Write(buf[:n])
				time.Sleep(100 * time.Millisecond)
			}(conn)
		}
	}()

	return addr.IP.String(), addr.Port, func() { ln.Close() }
}

func TestBannerGrab_SSHBanner(t *testing.T) {
	host, port, cleanup := startMockTCPServer(t, "SSH-2.0-OpenSSH_8.9p1 Ubuntu-3ubuntu0.1\r\n")
	defer cleanup()

	tool := getBannerGrabTool(t)
	stdout, _ := execBannerGrabTool(t, tool, aitool.InvokeParams{
		"target":  fmt.Sprintf("%s:%d", host, port),
		"timeout": 5,
	})

	assert.Assert(t, strings.Contains(stdout, "SSH"), "should identify SSH service, got:\n%s", stdout)
	assert.Assert(t, strings.Contains(stdout, "OpenSSH"), "should show OpenSSH banner")
	assert.Assert(t, strings.Contains(stdout, "[OK]"), "should report success")
	t.Logf("stdout:\n%s", stdout)
}

func TestBannerGrab_SMTPBanner(t *testing.T) {
	host, port, cleanup := startMockTCPServer(t, "220 mail.example.com ESMTP Postfix\r\n")
	defer cleanup()

	tool := getBannerGrabTool(t)
	stdout, _ := execBannerGrabTool(t, tool, aitool.InvokeParams{
		"target":  fmt.Sprintf("%s:%d", host, port),
		"timeout": 5,
	})

	assert.Assert(t, strings.Contains(stdout, "FTP/SMTP") || strings.Contains(stdout, "SMTP"),
		"should identify SMTP service, got:\n%s", stdout)
	assert.Assert(t, strings.Contains(stdout, "Postfix"), "should show Postfix in banner")
	t.Logf("stdout:\n%s", stdout)
}

func TestBannerGrab_WithSendData(t *testing.T) {
	host, port, cleanup := startMockEchoServer(t)
	defer cleanup()

	tool := getBannerGrabTool(t)
	stdout, _ := execBannerGrabTool(t, tool, aitool.InvokeParams{
		"target":    fmt.Sprintf("%s:%d", host, port),
		"send-data": "HELLO\r\n",
		"timeout":   5,
	})

	assert.Assert(t, strings.Contains(stdout, "ECHO"), "should contain echoed data, got:\n%s", stdout)
	assert.Assert(t, strings.Contains(stdout, "HELLO"), "should echo back the sent data")
	t.Logf("stdout:\n%s", stdout)
}

func TestBannerGrab_BatchTargets(t *testing.T) {
	host1, port1, cleanup1 := startMockTCPServer(t, "SSH-2.0-TestSSH\r\n")
	defer cleanup1()
	host2, port2, cleanup2 := startMockTCPServer(t, "220 test.smtp.server\r\n")
	defer cleanup2()

	tool := getBannerGrabTool(t)
	target := fmt.Sprintf("%s:%d,%s:%d", host1, port1, host2, port2)
	stdout, _ := execBannerGrabTool(t, tool, aitool.InvokeParams{
		"target":  target,
		"timeout": 5,
	})

	assert.Assert(t, strings.Contains(stdout, "SSH"), "should find SSH banner")
	assert.Assert(t, strings.Contains(stdout, "smtp"), "should find SMTP banner")
	assert.Assert(t, strings.Contains(stdout, "2 succeeded"), "should report 2 successes, got:\n%s", stdout)
	t.Logf("stdout:\n%s", stdout)
}

func TestBannerGrab_ConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	tool := getBannerGrabTool(t)
	stdout, _ := execBannerGrabTool(t, tool, aitool.InvokeParams{
		"target":  "127.0.0.1:" + strconv.Itoa(port),
		"timeout": 2,
	})

	assert.Assert(t, strings.Contains(stdout, "FAIL") || strings.Contains(stdout, "failed"),
		"should report connection failure, got:\n%s", stdout)
	t.Logf("stdout:\n%s", stdout)
}
