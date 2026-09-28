package sgnds

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
)

const (
	defaultBridgeAddr = "127.0.0.1:19090"
	bridgeJarName     = "sgnds-bridge.jar"
	driverJarName     = "sg-jdbc-driver.jar"
)

var (
	bridgeOnce     sync.Once
	bridgeLauncher *BridgeLauncher
)

// BridgeLauncher 负责把内置的 jar 释放到临时目录并启动本地代理。
type BridgeLauncher struct {
	workDir string
	addr    string
	cmd     *exec.Cmd
	mu      sync.Mutex
}

// DefaultBridgeAddr 返回默认桥接地址。
func DefaultBridgeAddr() string { return defaultBridgeAddr }

// EnsureBridgeRunning 确保本地 Java 代理已启动并返回监听地址。
// 首次调用会释放 jar、启动进程；后续调用直接返回地址。
func EnsureBridgeRunning(ctx context.Context) (string, error) {
	bridgeOnce.Do(func() {
		bridgeLauncher = &BridgeLauncher{addr: defaultBridgeAddr}
	})
	return bridgeLauncher.Ensure(ctx)
}

// Ensure 检查/启动代理。
func (l *BridgeLauncher) Ensure(ctx context.Context) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.cmd != nil && l.cmd.Process != nil && l.isListening(ctx) {
		return l.addr, nil
	}

	workDir, err := os.MkdirTemp("", "yakit-sgnds-bridge-*")
	if err != nil {
		return "", fmt.Errorf("create bridge workdir: %w", err)
	}
	l.workDir = workDir

	if err := os.WriteFile(filepath.Join(workDir, bridgeJarName), bridgeJarBytes, 0644); err != nil {
		return "", fmt.Errorf("write bridge jar: %w", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, driverJarName), driverJarBytes, 0644); err != nil {
		return "", fmt.Errorf("write driver jar: %w", err)
	}

	// 找一个可用端口
	host, port, err := splitHostPort(l.addr)
	if err != nil {
		return "", err
	}
	freePort, err := findFreePort(host, port)
	if err != nil {
		return "", err
	}
	l.addr = net.JoinHostPort(host, strconv.Itoa(freePort))

	cpSep := ";"
	if runtime.GOOS != "windows" {
		cpSep = ":"
	}
	classpath := filepath.Join(workDir, bridgeJarName) + cpSep + filepath.Join(workDir, driverJarName)

	cmd := exec.CommandContext(ctx, "java",
		"-cp", classpath,
		"com.yaklang.sgnds.bridge.BridgeServer",
		host, strconv.Itoa(freePort), "32",
	)
	cmd.Dir = workDir
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start bridge: %w", err)
	}
	l.cmd = cmd

	// 等待启动就绪
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if l.isListening(ctx) {
			return l.addr, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return "", fmt.Errorf("bridge did not start in time")
}

// Stop 停止代理进程。
func (l *BridgeLauncher) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cmd != nil && l.cmd.Process != nil {
		_ = l.cmd.Process.Kill()
		_, _ = l.cmd.Process.Wait()
	}
	if l.workDir != "" {
		_ = os.RemoveAll(l.workDir)
	}
	return nil
}

func (l *BridgeLauncher) isListening(ctx context.Context) bool {
	d := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", l.addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func splitHostPort(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

func findFreePort(host string, preferred int) (int, error) {
	if preferred > 0 {
		l, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(preferred)))
		if err == nil {
			_ = l.Close()
			return preferred, nil
		}
	}
	l, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return 0, err
	}
	defer l.Close()
	_, portStr, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(portStr)
}
