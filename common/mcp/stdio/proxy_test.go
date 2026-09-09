package stdio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

var cachedStdout = os.Stdout

func init() {
	if os.Getenv("TEST_MCP_WORKER") == "1" {
		// Deliberately no newline: prefix filtering on stdout cannot isolate it.
		fmt.Fprint(cachedStdout, "INITIALIZER_STDOUT")
	}
}

func TestWorkerProcess(t *testing.T) {
	if os.Getenv("TEST_MCP_WORKER") != "1" {
		return
	}
	mode := os.Getenv("TEST_MCP_MODE")
	if mode == "startup-exit" {
		fmt.Fprintln(os.Stderr, "initialization failed")
		os.Exit(23)
	}
	if mode == "startup-hang" {
		time.Sleep(time.Minute)
		os.Exit(1)
	}
	if mode == "unauthenticated" {
		conn, err := net.DialTimeout("tcp4", os.Getenv(addressEnv), time.Second)
		if err != nil {
			os.Exit(6)
		}
		conn.SetDeadline(time.Now().Add(time.Second))
		io.WriteString(conn, strings.Repeat("x", 64))
		var ack [1]byte
		if _, err := conn.Read(ack[:]); err == nil {
			os.Exit(7)
		}
		conn.Close()
	}
	conn, err := DialWorker()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if IsWorker() || os.Getenv(addressEnv) != "" || os.Getenv(tokenEnv) != "" {
		os.Exit(3)
	}
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64<<10), maxFrameSize)
	for scanner.Scan() {
		var request envelope
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			os.Exit(4)
		}
		fmt.Fprint(cachedStdout, "TOOL_STDOUT")
		fmt.Fprintln(os.Stdout, `{"jsonrpc":"2.0","id":"fake","result":"LOG_JSON"}`)
		fmt.Fprintln(os.Stderr, "TOOL_STDERR")
		if request.Method == "crash" {
			switch mode {
			case "panic":
				go func() { panic("MCP_WORKER_PANIC") }()
				time.Sleep(time.Minute)
			case "partial":
				fmt.Fprintf(conn, `{"jsonrpc":"2.0","id":%s,"result":`, request.ID)
				os.Exit(24)
			case "unterminated":
				fmt.Fprintf(conn, `{"jsonrpc":"2.0","id":%s,"result":{}}`, request.ID)
				os.Exit(24)
			case "invalid":
				fmt.Fprintln(conn, "not JSON-RPC")
				time.Sleep(time.Minute)
			case "hang":
				fmt.Fprintln(conn, `{"jsonrpc":"2.0","method":"notifications/progress","params":{"progress":1}}`)
				time.Sleep(time.Minute)
			case "flood-logs":
				fmt.Fprintln(conn, `{"jsonrpc":"2.0","method":"notifications/progress","params":{"progress":1}}`)
				fmt.Fprint(os.Stderr, strings.Repeat("diagnostic output\n", 1<<20))
				time.Sleep(time.Minute)
			case "drain-exit":
				// Read two requests, then publish the first response immediately
				// before dying with the second request still pending.
				firstID := bytes.Clone(request.ID)
				if !scanner.Scan() {
					os.Exit(5)
				}
				fmt.Fprintf(conn, "{\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{}}\n", firstID)
				os.Exit(23)
			case "clean-exit":
				os.Exit(0)
			default:
				os.Exit(23)
			}
		}
		if request.Method != "" && len(request.ID) > 0 {
			response, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"pid": os.Getpid()}})
			fmt.Fprintf(conn, "%s\n", response)
		} else {
			fmt.Fprintln(conn, `{"jsonrpc":"2.0","method":"notifications/progress","params":{"progress":1}}`)
		}
	}
	conn.Close()
	os.Exit(0)
}

func helperCommand(mode string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestWorkerProcess$")
	cmd.Env = append(os.Environ(), "TEST_MCP_WORKER=1", "TEST_MCP_MODE="+mode)
	return cmd
}

func decodeOutput(t *testing.T, output []byte) []envelope {
	t.Helper()
	var messages []envelope
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		var message envelope
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil || message.JSONRPC != "2.0" {
			t.Fatalf("invalid stdout frame %q: %v", scanner.Text(), err)
		}
		messages = append(messages, message)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return messages
}

func TestRunIsolatesWorkerStreamsAndDrainsResponses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var payload strings.Builder
	ids := []string{`1`, `0`, `"call-2"`, `9007199254740993`}
	for _, id := range ids {
		fmt.Fprintf(&payload, "{\"jsonrpc\":\"2.0\",\"id\":%s,\"method\":\"ping\"}\n", id)
	}
	payload.WriteString("{\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}\n")
	var output bytes.Buffer
	logs := newDiagnosticFile(t)
	cmd := helperCommand("echo")
	if err := Run(ctx, cmd, io.NopCloser(strings.NewReader(payload.String())), nopWriteCloser{&output}, logs); err != nil {
		t.Fatalf("run: %v; stdout: %s; logs: %s", err, &output, readDiagnostics(t, logs))
	}
	messages := decodeOutput(t, output.Bytes())
	if len(messages) != len(ids)+1 {
		t.Fatalf("missing or unexpected messages: %s", &output)
	}
	for i, id := range ids {
		if string(messages[i].ID) != id || len(messages[i].Error) > 0 {
			t.Fatalf("response %d: %+v", i, messages[i])
		}
		var result struct {
			PID int `json:"pid"`
		}
		json.Unmarshal(messages[i].Result, &result)
		if result.PID != cmd.Process.Pid || result.PID == os.Getpid() {
			t.Fatalf("tool did not execute in worker: %d", result.PID)
		}
	}
	if messages[len(ids)].Method != "notifications/progress" {
		t.Fatalf("notification was lost: %s", &output)
	}
	for _, marker := range []string{"INITIALIZER_STDOUT", "TOOL_STDOUT", "TOOL_STDERR", "LOG_JSON"} {
		if strings.Contains(output.String(), marker) || !strings.Contains(readDiagnostics(t, logs), marker) {
			t.Fatalf("diagnostic %s not isolated; stdout: %s; logs: %s", marker, &output, readDiagnostics(t, logs))
		}
	}
	if cmd.ProcessState == nil || !cmd.ProcessState.Success() {
		t.Fatalf("worker not reaped successfully: %v", cmd.ProcessState)
	}
}

func TestRunReportsWorkerFailures(t *testing.T) {
	for _, mode := range []string{"startup-exit", "exit", "panic", "partial", "unterminated", "invalid", "clean-exit"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			var output bytes.Buffer
			logs := newDiagnosticFile(t)
			cmd := helperCommand(mode)
			err := Run(ctx, cmd, io.NopCloser(strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":\"crash-id\",\"method\":\"crash\"}\n")), nopWriteCloser{&output}, logs)
			if err == nil {
				t.Fatal("expected worker failure")
			}
			messages := decodeOutput(t, output.Bytes())
			if len(messages) != 1 || string(messages[0].ID) != `"crash-id"` || len(messages[0].Error) == 0 {
				t.Fatalf("expected one matching error, got %s (error: %v)", &output, err)
			}
			var failure struct {
				Code int `json:"code"`
				Data struct {
					ExitCode int `json:"exitCode"`
				} `json:"data"`
			}
			if err := json.Unmarshal(messages[0].Error, &failure); err != nil || failure.Code != -32603 {
				t.Fatalf("invalid failure: %s", &output)
			}
			if (mode == "exit" || mode == "startup-exit") && failure.Data.ExitCode != 23 {
				t.Fatalf("missing exit code: %s", &output)
			}
			if mode == "panic" && !strings.Contains(readDiagnostics(t, logs), "MCP_WORKER_PANIC") {
				t.Fatalf("missing panic diagnostics: %s", readDiagnostics(t, logs))
			}
			if cmd.ProcessState == nil {
				t.Fatal("worker was not reaped")
			}
		})
	}
}

func TestRunStartupCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	var output bytes.Buffer
	logs := newDiagnosticFile(t)
	cmd := helperCommand("startup-hang")
	err := Run(ctx, cmd, io.NopCloser(strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\"}\n")), nopWriteCloser{&output}, logs)
	if err == nil || cmd.ProcessState == nil {
		t.Fatalf("startup cancellation did not reap worker: %v", err)
	}
	if messages := decodeOutput(t, output.Bytes()); len(messages) != 1 || len(messages[0].Error) == 0 {
		t.Fatalf("missing initialize failure: %s", &output)
	}
}

func TestRunMissingExecutable(t *testing.T) {
	var output bytes.Buffer
	cmd := exec.Command(t.TempDir() + "/missing")
	err := Run(context.Background(), cmd, io.NopCloser(strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":0,\"method\":\"initialize\"}\n")), nopWriteCloser{&output}, nil)
	if err == nil {
		t.Fatal("expected launch error")
	}
	if messages := decodeOutput(t, output.Bytes()); len(messages) != 1 || string(messages[0].ID) != "0" || len(messages[0].Error) == 0 {
		t.Fatalf("missing initialize failure: %s", &output)
	}
}

func TestRunEmptySessionDoesNotStartWorker(t *testing.T) {
	cmd := helperCommand("echo")
	if err := Run(context.Background(), cmd, io.NopCloser(strings.NewReader("")), nopWriteCloser{io.Discard}, nil); err != nil || cmd.Process != nil {
		t.Fatalf("empty session launched a worker: %v", err)
	}
}

func TestRunExternalKillWhileClientStaysOpen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	input, clientWriter := io.Pipe()
	outputReader, output := io.Pipe()
	defer clientWriter.Close()
	defer outputReader.Close()
	cmd := helperCommand("hang")
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, cmd, input, output, nil)
		output.Close()
	}()
	fmt.Fprintln(clientWriter, `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	scanner := bufio.NewScanner(outputReader)
	if !scanner.Scan() {
		t.Fatal("missing first response")
	}
	fmt.Fprintln(clientWriter, `{"jsonrpc":"2.0","id":"pending","method":"crash"}`)
	if !scanner.Scan() || !strings.Contains(scanner.Text(), "notifications/progress") {
		t.Fatal("worker did not start tool call")
	}
	// A progress notification proves the pending call reached the worker.
	// Resolve the PID from the completed response to avoid racing cmd.Start.
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if !scanner.Scan() {
		t.Fatal("missing crash response with client stdin still open")
	}
	messages := decodeOutput(t, scanner.Bytes())
	if len(messages) != 1 || string(messages[0].ID) != `"pending"` || len(messages[0].Error) == 0 {
		t.Fatalf("wrong crash response: %s", scanner.Text())
	}
	if scanner.Scan() {
		t.Fatalf("duplicate response: %s", scanner.Text())
	}
	if err := <-done; err == nil {
		t.Fatal("expected abnormal exit")
	}
}

func TestRelayDoesNotConsumePendingIDForServerRequest(t *testing.T) {
	var output bytes.Buffer
	r := relay{output: &output, pending: make(map[string]json.RawMessage)}
	r.track([]byte(`{"jsonrpc":"2.0","id":"same","method":"tools/call"}`))
	if err := r.forward([]byte("{\"jsonrpc\":\"2.0\",\"id\":\"same\",\"method\":\"sampling/createMessage\"}\n")); err != nil {
		t.Fatal(err)
	}
	if len(r.pending) != 1 {
		t.Fatal("server request consumed the client's pending ID")
	}
}

func TestRunDrainsSuccessBeforeFailingRemainingRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var output bytes.Buffer
	payload := "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"crash\"}\n{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"ping\"}\n"
	err := Run(ctx, helperCommand("drain-exit"), io.NopCloser(strings.NewReader(payload)), nopWriteCloser{&output}, nil)
	if err == nil {
		t.Fatal("expected worker exit")
	}
	messages := decodeOutput(t, output.Bytes())
	if len(messages) != 2 || string(messages[0].ID) != "1" || len(messages[0].Error) != 0 || string(messages[1].ID) != "2" || len(messages[1].Error) == 0 {
		t.Fatalf("success was lost or duplicated: %s", &output)
	}
}

func TestRunShutdownReapsBusyWorker(t *testing.T) {
	for _, cancelSession := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%v", cancelSession), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			input, client := io.Pipe()
			defer client.Close()
			reader, output := io.Pipe()
			defer reader.Close()
			cmd := helperCommand("hang")
			done := make(chan error, 1)
			go func() {
				done <- Run(ctx, cmd, input, output, nil)
				output.Close()
			}()
			fmt.Fprintln(client, `{"jsonrpc":"2.0","id":1,"method":"crash"}`)
			scanner := bufio.NewScanner(reader)
			if !scanner.Scan() {
				t.Fatal("worker did not start")
			}
			if cancelSession {
				cancel()
			} else {
				client.Close()
			}
			if !scanner.Scan() {
				t.Fatal("missing shutdown response")
			}
			if messages := decodeOutput(t, scanner.Bytes()); len(messages) != 1 || len(messages[0].Error) == 0 {
				t.Fatalf("unexpected shutdown response: %s", scanner.Text())
			}
			if err := <-done; err == nil || cmd.ProcessState == nil {
				t.Fatalf("worker was not stopped and reaped: %v", err)
			}
		})
	}
}

func TestChildEnvironment(t *testing.T) {
	child := childEnvironment([]string{"KEEP=value", workerEnv + "=stale", addressEnv + "=stale", strings.ToLower(tokenEnv) + "=stale", "YAK_MCP_STDIO=0"}, "127.0.0.1:1234", "secret")
	joined := strings.Join(child, "\n")
	if strings.Contains(joined, "stale") || !strings.Contains(joined, "KEEP=value") || !strings.Contains(joined, "YAK_MCP_STDIO=1") || !strings.Contains(joined, workerEnv+"=1") {
		t.Fatalf("incorrect worker environment: %v", child)
	}
}

func TestRelayMatchesEquivalentIDsWithoutRounding(t *testing.T) {
	var output bytes.Buffer
	r := relay{output: &output, pending: make(map[string]json.RawMessage)}
	for _, id := range []string{"1.0", "9007199254740992", "9007199254740993", `"\u0061"`} {
		r.track([]byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"method":"ping"}`, id)))
	}
	for _, id := range []string{"1e0", "9007199254740992", `"a"`} {
		if err := r.forward([]byte(fmt.Sprintf("{\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{}}\n", id))); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.pending) != 1 || string(r.pending[idKey([]byte("9007199254740993"))]) != "9007199254740993" {
		t.Fatalf("incorrect ID correlation: %v", r.pending)
	}
}

func TestRunRejectsUnauthenticatedConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var output bytes.Buffer
	err := Run(ctx, helperCommand("unauthenticated"), io.NopCloser(strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}\n")), nopWriteCloser{&output}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if messages := decodeOutput(t, output.Bytes()); len(messages) != 1 || len(messages[0].Error) != 0 {
		t.Fatalf("authenticated worker could not connect after rejecting intruder: %s", &output)
	}
}

type observedWriteCloser struct {
	io.WriteCloser
	entered chan struct{}
	once    sync.Once
}

func (w *observedWriteCloser) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	return w.WriteCloser.Write(p)
}

func TestRunCancellationWithBlockedOutput(t *testing.T) {
	for _, stream := range []string{"stdout", "stderr"} {
		t.Run(stream, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			input, client := io.Pipe()
			reader, output := io.Pipe()
			logReader, logWriter, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			observed := &observedWriteCloser{WriteCloser: output, entered: make(chan struct{})}
			mode := "hang"
			if stream == "stderr" {
				mode = "flood-logs"
			}
			cmd := helperCommand(mode)
			done := make(chan error, 1)
			go func() {
				done <- Run(ctx, cmd, input, observed, logWriter)
			}()
			finished := false
			t.Cleanup(func() {
				cancel()
				client.Close()
				reader.Close()
				output.Close()
				logReader.Close()
				logWriter.Close()
				if !finished {
					select {
					case <-done:
					case <-time.After(10 * time.Second):
						t.Error("supervisor failed to clean up after releasing blocked outputs")
					}
				}
			})
			fmt.Fprintln(client, `{"jsonrpc":"2.0","id":"blocked","method":"crash"}`)
			select {
			case <-observed.entered:
			case <-time.After(10 * time.Second):
				t.Fatal("worker did not produce its progress notification")
			}
			if stream == "stderr" {
				// Keep protocol output draining while the worker fills a real,
				// deliberately unread diagnostic pipe.
				go io.Copy(io.Discard, reader)
				time.Sleep(100 * time.Millisecond)
			}
			cancel()
			select {
			case err := <-done:
				finished = true
				if err == nil || cmd.ProcessState == nil {
					t.Fatalf("cancellation must return an error and reap the worker: err=%v state=%v", err, cmd.ProcessState)
				}
			case <-time.After(3 * time.Second):
				t.Fatalf("supervisor still blocked on %s after cancellation", stream)
			}
		})
	}
}

// Only nonblocking in-memory writers use this adapter. Blocking test outputs
// use io.Pipe or os.Pipe so that Close really interrupts an active Write.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func newDiagnosticFile(t *testing.T) *os.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "diagnostics")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	return file
}

func readDiagnostics(t *testing.T, file *os.File) string {
	t.Helper()
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
