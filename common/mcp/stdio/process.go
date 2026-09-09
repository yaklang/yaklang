// Package stdio isolates MCP execution from the process owning the client's
// stdin/stdout. Worker standard streams carry diagnostics, never JSON-RPC.
package stdio

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	workerEnv       = "YAK_MCP_WORKER"
	addressEnv      = "YAK_MCP_WORKER_ADDRESS"
	tokenEnv        = "YAK_MCP_WORKER_TOKEN"
	startupTimeout  = 60 * time.Second
	shutdownTimeout = 5 * time.Second
)

// IsWorker reports the internal re-execution mode, before CLI initialization.
func IsWorker() bool { return os.Getenv(workerEnv) == "1" }

// DialWorker connects the worker's protocol transport. stdout and stderr remain
// untouched, including handles cached by package initializers and subprocesses.
func DialWorker() (*net.TCPConn, error) {
	address, token := os.Getenv(addressEnv), os.Getenv(tokenEnv)
	// Do not pass the internal worker identity to tools which launch Yak again.
	for _, key := range []string{workerEnv, addressEnv, tokenEnv} {
		_ = os.Unsetenv(key)
	}
	if len(token) != 64 {
		return nil, fmt.Errorf("invalid MCP worker credentials")
	}
	addr, err := net.ResolveTCPAddr("tcp4", address)
	if err != nil || addr == nil || !addr.IP.IsLoopback() {
		return nil, fmt.Errorf("invalid MCP worker address")
	}
	conn, err := net.DialTimeout("tcp4", address, startupTimeout)
	if err != nil {
		return nil, fmt.Errorf("connect MCP supervisor: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(startupTimeout))
	if _, err = io.WriteString(conn, token); err == nil {
		var ack [1]byte
		_, err = io.ReadFull(conn, ack[:])
		if err == nil && ack[0] != 1 {
			err = fmt.Errorf("invalid MCP supervisor handshake")
		}
	}
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("authenticate MCP worker: %w", err)
	}
	_ = conn.SetDeadline(time.Time{})
	return conn.(*net.TCPConn), nil
}

type workerProcess struct {
	cmd     *exec.Cmd
	conn    *net.TCPConn
	done    chan struct{}
	waitErr error // published by closing done
}

func childEnvironment(parent []string, address, token string) []string {
	result := make([]string, 0, len(parent)+4)
	for _, entry := range parent {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case workerEnv, addressEnv, tokenEnv, "YAK_MCP_STDIO":
			continue
		}
		result = append(result, entry)
	}
	return append(result, workerEnv+"=1", addressEnv+"="+address, tokenEnv+"="+token, "YAK_MCP_STDIO=1")
}

func startWorker(ctx context.Context, cmd *exec.Cmd, logs *os.File) (_ *workerProcess, retErr error) {
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return nil, fmt.Errorf("listen for MCP worker: %w", err)
	}
	defer listener.Close()
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, fmt.Errorf("create MCP worker token: %w", err)
	}
	token := hex.EncodeToString(secret[:])
	environment := cmd.Env
	if environment == nil {
		environment = os.Environ()
	}
	cmd.Env = childEnvironment(environment, listener.Addr().String(), token)
	cmd.Stdin = nil
	// Pass the diagnostic descriptor directly to the child. Wrapping it in an
	// io.Writer makes exec.Cmd start copying goroutines; a full client stderr
	// pipe can then block Wait even after the child has been killed.
	cmd.Stdout, cmd.Stderr = nil, nil
	if logs != nil {
		cmd.Stdout, cmd.Stderr = logs, logs
	}
	configureProcess(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start MCP worker: %w", err)
	}
	p := &workerProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		p.waitErr = cmd.Wait()
		close(p.done)
	}()
	defer func() {
		if retErr != nil {
			p.close()
		}
	}()

	startupCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	accepted := make(chan *net.TCPConn)
	go func() {
		for {
			conn, err := listener.AcceptTCP()
			if err != nil {
				return
			}
			_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
			var supplied [64]byte
			_, err = io.ReadFull(conn, supplied[:])
			if err != nil || subtle.ConstantTimeCompare(supplied[:], []byte(token)) != 1 {
				_ = conn.Close()
				continue
			}
			if _, err = conn.Write([]byte{1}); err != nil {
				_ = conn.Close()
				continue
			}
			_ = conn.SetDeadline(time.Time{})
			select {
			case accepted <- conn:
			case <-startupCtx.Done():
				_ = conn.Close()
			}
			return
		}
	}()
	select {
	case p.conn = <-accepted:
		return p, nil
	case <-p.done:
		if p.waitErr != nil {
			return nil, fmt.Errorf("MCP worker exited during initialization: %w", p.waitErr)
		}
		return nil, fmt.Errorf("MCP worker exited during initialization")
	case <-startupCtx.Done():
		return nil, fmt.Errorf("wait for MCP worker: %w", startupCtx.Err())
	}
}

func (p *workerProcess) close() {
	if p.conn != nil {
		_ = p.conn.Close()
	}
	killProcess(p.cmd)
	<-p.done
}
