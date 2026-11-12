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
	"github.com/yaklang/yaklang/common/utils/subprocess"
)

type memfitProcessClient struct {
	mp       *subprocess.ManagedProcess
	stdin    io.WriteCloser
	protocol *memfitProtocolWriter
	trace    *memfitTrace

	events chan memfitEnvelope
	logs   chan string

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
	trace, err := newMemfitTrace(config.DumpDir, config.APIKey)
	if err != nil {
		return nil, err
	}
	trace.recordJSONL(memfitTraceLifecycleFile, "parent_started", map[string]any{
		"executable": executable,
		"workdir":    config.Workdir,
		"focus":      config.Focus,
	})

	// Create an io.Pipe for stdin so we can write protocol messages to the child.
	stdinReader, stdinWriter := io.Pipe()

	client := &memfitProcessClient{
		stdin:    stdinWriter,
		protocol: newMemfitProtocolWriter(stdinWriter),
		events:   make(chan memfitEnvelope, 512),
		logs:     make(chan string, 256),
		secret:   config.APIKey,
		trace:    trace,
	}

	mp, err := subprocess.Launch(ctx, &subprocess.LaunchConfig{
		Cmd: &exec.Cmd{
			Path: executable,
			Args: []string{executable, "memfit-worker"},
			Dir:  config.Workdir,
		},
		EnvExclude: []string{"YAK_AI_API_KEY", memfitWorkerEnvironment},
		EnvExtra:   []string{memfitWorkerEnvironment + "=1"},
		Stdio: subprocess.StdioConfig{
			Stdin: stdinReader,
		},
		StartupTimeout:  60 * time.Second,
		ShutdownTimeout: 3 * time.Second,
		Ready: func(ctx context.Context, p *subprocess.ManagedProcess) error {
			// Start reading stdout/stderr from tap readers.
			go client.readProtocol(p.Stdout(), p.Done())
			go client.readLogs(p.Stderr(), "")

			// Send start configuration and wait for "ready".
			startID := fmt.Sprintf("start-%d", memfitNowMillis())
			if err := client.protocol.send("start", startID, config); err != nil {
				return utils.Wrap(err, "send memfit worker configuration")
			}

			timer := time.NewTimer(60 * time.Second)
			defer timer.Stop()
			for {
				select {
				case envelope := <-client.events:
					switch envelope.Type {
					case "ready":
						trace.recordJSONL(memfitTraceLifecycleFile, "worker_ready", nil)
						return nil
					case "error":
						status, _ := decodeMemfitPayload[memfitStatus](envelope)
						return utils.Errorf("memfit worker initialization failed: %s%s", status.Message, client.formattedLogTail())
					}
				case <-p.Done():
					return utils.Errorf("%s worker exited during initialization: %v%s", memfitDisplayName(config), p.WaitError(), client.formattedLogTail())
				case <-ctx.Done():
					return ctx.Err()
				case <-timer.C:
					return utils.Errorf("timed out waiting for memfit worker%s", client.formattedLogTail())
				}
			}
		},
		GracefulShutdown: func(p *subprocess.ManagedProcess) error {
			shutdownID := fmt.Sprintf("shutdown-%d", memfitNowMillis())
			trace.recordProtocol("parent_to_worker", "shutdown", shutdownID, nil)
			_ = client.protocol.send("shutdown", shutdownID, nil)
			_ = stdinWriter.Close()
			return nil
		},
	})
	if err != nil {
		trace.Close()
		return nil, err
	}
	client.mp = mp
	trace.recordJSONL(memfitTraceLifecycleFile, "worker_process_started", map[string]any{"pid": mp.PID()})
	return client, nil
}

func (c *memfitProcessClient) readProtocol(reader io.Reader, done <-chan struct{}) {
	if reader == nil {
		return
	}
	scanner := newMemfitFrameScanner(reader)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		envelope, err := decodeMemfitEnvelope(line)
		if err != nil {
			c.trace.recordLine(memfitTraceWorkerLogFile, "stdout: "+string(line))
			c.recordLog("stdout: " + string(line))
			continue
		}
		c.trace.recordProtocol("worker_to_parent", envelope.Type, envelope.ID, envelope.Data)
		select {
		case c.events <- envelope:
		case <-done:
			return
		}
	}
	if err := scanner.Err(); err != nil {
		c.recordLog("stdout read error: " + err.Error())
	}
}

func (c *memfitProcessClient) readLogs(reader io.Reader, prefix string) {
	if reader == nil {
		return
	}
	scanner := newMemfitFrameScanner(reader)
	for scanner.Scan() {
		line := prefix + scanner.Text()
		c.trace.recordLine(memfitTraceWorkerLogFile, line)
		c.recordLog(line)
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

func (c *memfitProcessClient) Done() <-chan struct{} { return c.mp.Done() }

func (c *memfitProcessClient) PID() int { return c.mp.PID() }

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
	c.trace.recordProtocol("parent_to_worker", typ, id, payload)
	select {
	case <-c.mp.Done():
		return utils.Errorf("memfit worker is not running: %v", c.mp.WaitError())
	default:
	}
	return c.protocol.send(typ, id, payload)
}

func (c *memfitProcessClient) WaitError() error { return c.mp.WaitError() }

func (c *memfitProcessClient) Close() {
	if c.mp != nil {
		c.mp.Close()
	}
	c.trace.Close()
}
