package memfitcli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

type memfitProcessClient struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	protocol *memfitProtocolWriter

	events chan memfitEnvelope
	logs   chan string
	done   chan struct{}

	waitMu  sync.RWMutex
	waitErr error
	closeMu sync.Mutex
	closed  bool

	logMu   sync.Mutex
	logTail []string
	secret  string
}

// memfitClient is the local side of the worker transport. Keeping the TUI on
// this narrow interface makes the complete terminal interaction testable with
// a deterministic worker while production still uses a real child process.
type memfitClient interface {
	send(typ, id string, payload any) error
	Events() <-chan memfitEnvelope
	Logs() <-chan string
	Done() <-chan struct{}
	WaitError() error
	LogTail() []string
	formattedLogTail() string
	PID() int
	Close()
}

func startMemfitProcessClient(ctx context.Context, config memfitStartConfig) (*memfitProcessClient, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, utils.Wrap(err, "resolve yak executable for memfit worker")
	}
	cmd := exec.Command(executable, "memfit-worker")
	cmd.Dir = config.Workdir
	cmd.Env = memfitChildEnvironment(os.Environ())
	configureMemfitChildProcess(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, utils.Wrap(err, "open memfit worker stdin")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, utils.Wrap(err, "open memfit worker stdout")
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, utils.Wrap(err, "open memfit worker stderr")
	}

	client := &memfitProcessClient{
		cmd:      cmd,
		stdin:    stdin,
		protocol: newMemfitProtocolWriter(stdin),
		events:   make(chan memfitEnvelope, 512),
		logs:     make(chan string, 256),
		done:     make(chan struct{}),
		secret:   config.APIKey,
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, utils.Wrap(err, "start isolated memfit worker")
	}

	go client.readProtocol(stdout)
	go client.readLogs(stderr, "")
	go func() {
		err := cmd.Wait()
		client.waitMu.Lock()
		client.waitErr = err
		client.waitMu.Unlock()
		close(client.done)
	}()

	startID := fmt.Sprintf("start-%d", memfitNowMillis())
	if err := client.send("start", startID, config); err != nil {
		client.Close()
		return nil, utils.Wrap(err, "send memfit worker configuration")
	}

	timer := time.NewTimer(60 * time.Second)
	defer timer.Stop()
	for {
		select {
		case envelope := <-client.events:
			switch envelope.Type {
			case "ready":
				return client, nil
			case "error":
				status, _ := decodeMemfitPayload[memfitStatus](envelope)
				client.Close()
				return nil, utils.Errorf("memfit worker initialization failed: %s%s", status.Message, client.formattedLogTail())
			}
		case <-client.done:
			return nil, utils.Errorf("memfit worker exited during initialization: %v%s", client.WaitError(), client.formattedLogTail())
		case <-ctx.Done():
			client.Close()
			return nil, ctx.Err()
		case <-timer.C:
			client.Close()
			return nil, utils.Errorf("timed out waiting for memfit worker%s", client.formattedLogTail())
		}
	}
}

func memfitChildEnvironment(parent []string) []string {
	child := make([]string, 0, len(parent)+1)
	for _, entry := range parent {
		if strings.HasPrefix(entry, "YAK_AI_API_KEY=") || strings.HasPrefix(entry, memfitWorkerEnvironment+"=") {
			continue
		}
		child = append(child, entry)
	}
	return append(child, memfitWorkerEnvironment+"=1")
}

func (c *memfitProcessClient) readProtocol(reader io.Reader) {
	scanner := newMemfitFrameScanner(reader)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		envelope, err := decodeMemfitEnvelope(line)
		if err != nil {
			c.recordLog("stdout: " + string(line))
			continue
		}
		select {
		case c.events <- envelope:
		case <-c.done:
			return
		}
	}
	if err := scanner.Err(); err != nil {
		c.recordLog("stdout read error: " + err.Error())
	}
}

func (c *memfitProcessClient) readLogs(reader io.Reader, prefix string) {
	scanner := newMemfitFrameScanner(reader)
	for scanner.Scan() {
		c.recordLog(prefix + scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		c.recordLog(prefix + "log read error: " + err.Error())
	}
}

func (c *memfitProcessClient) recordLog(line string) {
	line = strings.TrimSpace(redactMemfitSecret(line, c.secret))
	if line == "" {
		return
	}
	c.logMu.Lock()
	c.logTail = append(c.logTail, line)
	if len(c.logTail) > 80 {
		c.logTail = append([]string(nil), c.logTail[len(c.logTail)-80:]...)
	}
	c.logMu.Unlock()
	select {
	case c.logs <- line:
	default:
	}
}

func (c *memfitProcessClient) LogTail() []string {
	c.logMu.Lock()
	defer c.logMu.Unlock()
	return append([]string(nil), c.logTail...)
}

func (c *memfitProcessClient) Events() <-chan memfitEnvelope { return c.events }

func (c *memfitProcessClient) Logs() <-chan string { return c.logs }

func (c *memfitProcessClient) Done() <-chan struct{} { return c.done }

func (c *memfitProcessClient) PID() int {
	if c.cmd == nil || c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
}

func (c *memfitProcessClient) formattedLogTail() string {
	logs := c.LogTail()
	if len(logs) == 0 {
		return ""
	}
	if len(logs) > 8 {
		logs = logs[len(logs)-8:]
	}
	return "\nworker log tail:\n  " + strings.Join(logs, "\n  ")
}

func (c *memfitProcessClient) send(typ, id string, payload any) error {
	select {
	case <-c.done:
		return utils.Errorf("memfit worker is not running: %v", c.WaitError())
	default:
	}
	return c.protocol.send(typ, id, payload)
}

func (c *memfitProcessClient) WaitError() error {
	c.waitMu.RLock()
	defer c.waitMu.RUnlock()
	return c.waitErr
}

func (c *memfitProcessClient) Close() {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return
	}
	c.closed = true
	c.closeMu.Unlock()

	_ = c.protocol.send("shutdown", fmt.Sprintf("shutdown-%d", memfitNowMillis()), nil)
	_ = c.stdin.Close()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-c.done:
		return
	case <-timer.C:
		killMemfitChildProcess(c.cmd)
		select {
		case <-c.done:
		case <-time.After(time.Second):
		}
	}
}
