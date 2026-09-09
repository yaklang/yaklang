package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMUSTPASS_MCPCommandStdioKeepsStdoutJSONRPCOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping process-level MCP stdio test in short mode")
	}
	for _, entry := range []struct {
		name   string
		target string
		args   []string
	}{
		{"yak", "./common/yak/cmd", []string{"mcp"}},
		{"standalone", "./common/mcp/cmd", nil},
	} {
		t.Run(entry.name, func(t *testing.T) {
			binary := buildMCPStdioExecutable(t, entry.target)
			t.Run("tool-output", func(t *testing.T) { testMCPStdioToolOutput(t, binary, entry.args) })
			t.Run("large-request-ids", func(t *testing.T) { testMCPStdioLargeRequestIDs(t, binary, entry.args) })
			t.Run("worker-killed", func(t *testing.T) { testMCPStdioWorkerKilled(t, binary, entry.args, false) })
			t.Run("worker-killed-blocked-stderr", func(t *testing.T) { testMCPStdioWorkerKilled(t, binary, entry.args, true) })
		})
	}
}

func testMCPStdioLargeRequestIDs(t *testing.T, binary string, entryArgs []string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	args := append(append([]string(nil), entryArgs...), "--transport", "stdio")
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = appendWithoutEnvKeys(os.Environ(), "YAKIT_HOME", "YAK_MCP_STDIO", "YAK_MCP_WORKER", "YAK_MCP_WORKER_ADDRESS", "YAK_MCP_WORKER_TOKEN", "LOG_LEVEL", "YAK_DEFAULT_PROJECT_DATABASE_NAME", "YAK_DEFAULT_PROFILE_DATABASE_NAME", "SSA_DATABASE_RAW")
	cmd.Env = append(cmd.Env, "YAKIT_HOME="+filepath.Join(t.TempDir(), "home"))
	cmd.Stdin = strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":9007199254740993,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"id-regression","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":9007199254740992,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":-9007199254740993,"method":"ping"}`,
	}, "\n") + "\n")
	var output, logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &logs
	if err := cmd.Run(); err != nil {
		t.Fatalf("stdio ID round trip failed: %v; stdout: %s; stderr: %s", err, &output, &logs)
	}
	want := map[string]bool{"9007199254740993": false, "9007199254740992": false, "-9007199254740993": false}
	scanner := bufio.NewScanner(&output)
	for scanner.Scan() {
		var response struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Error   json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil || response.JSONRPC != "2.0" {
			t.Fatalf("invalid response: %s", scanner.Text())
		}
		id := string(response.ID)
		if id == "" {
			continue
		}
		if seen, exists := want[id]; !exists || seen || len(response.Error) != 0 {
			t.Fatalf("unexpected, duplicate or failed response: %s", scanner.Text())
		}
		want[id] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	for id, seen := range want {
		if !seen {
			t.Fatalf("missing response for %s", id)
		}
	}
}

func buildMCPStdioExecutable(t *testing.T, target string) string {
	t.Helper()
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}

	binaryName := "yak-mcp-stdio-test"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)

	buildContext, cancelBuild := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelBuild()
	build := exec.CommandContext(buildContext, "go", "build", "-o", binaryPath, target)
	build.Dir = repositoryRoot
	if output, buildErr := build.CombinedOutput(); buildErr != nil {
		t.Fatalf("build %s: %v\n%s", target, buildErr, output)
	}
	return binaryPath
}

func testMCPStdioToolOutput(t *testing.T, binaryPath string, entryArgs []string) {
	const stdoutSentinel = "MCP_STDOUT_SENTINEL"
	request := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"stdio-regression-test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"exec_yak_script","arguments":{"pluginType":"yak","code":"println(\"` + stdoutSentinel + `\")"}}}`,
	}, "\n") + "\n"
	runContext, cancelRun := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelRun()
	args := append(append([]string(nil), entryArgs...),
		"--transport",
		"stdio",
		"--enable-aitool-framework",
		"--tool",
		"yak_script",
	)
	run := exec.CommandContext(runContext, binaryPath, args...)
	run.Env = appendWithoutEnvKeys(
		os.Environ(),
		"YAKIT_HOME",
		"YAK_MCP_STDIO",
		"YAK_MCP_WORKER",
		"YAK_MCP_WORKER_ADDRESS",
		"YAK_MCP_WORKER_TOKEN",
		"LOG_LEVEL",
		"YAK_DEFAULT_PROJECT_DATABASE_NAME",
		"YAK_DEFAULT_PROFILE_DATABASE_NAME",
		"SSA_DATABASE_RAW",
	)
	run.Env = append(run.Env, "YAKIT_HOME="+filepath.Join(t.TempDir(), "home"))
	run.Stdin = strings.NewReader(request)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	run.Stdout = &stdout
	run.Stderr = &stderr
	if runErr := run.Run(); runErr != nil {
		t.Fatalf("run yak mcp stdio: %v\nstderr:\n%s\nstdout:\n%s", runErr, stderr.String(), stdout.String())
	}

	if bytes.ContainsRune(stdout.Bytes(), '\x1b') {
		t.Fatalf("stdout contains ANSI escape bytes: %q", stdout.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte(stdoutSentinel)) {
		t.Fatalf("tool stdout leaked into JSON-RPC stream: %q", stdout.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte(stdoutSentinel)) {
		t.Fatalf("tool stdout was not redirected to stderr; stderr: %q", stderr.String())
	}

	responseIDs := make(map[int]bool)
	scanner := bufio.NewScanner(bytes.NewReader(stdout.Bytes()))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if !json.Valid(line) {
			t.Fatalf("stdout contains a non-JSON-RPC line: %q\nfull stderr:\n%s", line, stderr.String())
		}
		var response struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      int             `json:"id"`
			Error   json.RawMessage `json:"error"`
		}
		if decodeErr := json.Unmarshal(line, &response); decodeErr != nil {
			t.Fatalf("decode stdout response: %v", decodeErr)
		}
		if response.JSONRPC != "2.0" {
			t.Fatalf("stdout JSON is not JSON-RPC 2.0: %s", line)
		}
		if response.ID != 0 {
			if len(response.Error) > 0 || responseIDs[response.ID] {
				t.Fatalf("failed or duplicate response: %s", line)
			}
			responseIDs[response.ID] = true
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		t.Fatalf("scan stdout: %v", scanErr)
	}
	if !responseIDs[1] || !responseIDs[2] {
		t.Fatalf("missing initialize or tools/call response; response IDs: %v; stdout: %q", responseIDs, stdout.String())
	}
}

func testMCPStdioWorkerKilled(t *testing.T, binary string, entryArgs []string, blockStderr bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	args := append(append([]string(nil), entryArgs...), "--enable-aitool-framework", "--tool", "yak_script")
	run := exec.CommandContext(ctx, binary, args...)
	run.Env = appendWithoutEnvKeys(os.Environ(), "YAKIT_HOME", "YAK_MCP_STDIO", "YAK_MCP_WORKER", "YAK_MCP_WORKER_ADDRESS", "YAK_MCP_WORKER_TOKEN", "LOG_LEVEL", "YAK_DEFAULT_PROJECT_DATABASE_NAME", "YAK_DEFAULT_PROFILE_DATABASE_NAME", "SSA_DATABASE_RAW")
	run.Env = append(run.Env, "YAKIT_HOME="+filepath.Join(t.TempDir(), "home"))
	pidPath := filepath.Join(t.TempDir(), "worker.pid")
	// Plugin os.Exit is hooked to cancel the script, so kill the reported worker
	// from the test instead. Keep client stdin open until its failure is reported.
	code := fmt.Sprintf("file.Save(%q, sprint(os.Getpid())); sleep(60)", pidPath)
	if blockStderr {
		code = fmt.Sprintf("file.Save(%q, sprint(os.Getpid())); println(str.Repeat(\"MCP_DIAGNOSTIC_FILL\\n\", 1048576)); sleep(60)", pidPath)
	}
	call, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": "worker-killed", "method": "tools/call",
		"params": map[string]any{"name": "exec_yak_script", "arguments": map[string]any{"pluginType": "yak", "code": code}},
	})
	if err != nil {
		t.Fatal(err)
	}
	input, err := run.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	request := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"stdio-crash-test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		string(call),
	}, "\n") + "\n"
	var stdout, stderr bytes.Buffer
	run.Stdout, run.Stderr = &stdout, &stderr
	if blockStderr {
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		defer writer.Close()
		// No reader drains this pipe, including after worker failure. This also
		// catches CLI fatal logging/panic that would block after Run returns.
		run.Stderr = writer
	}
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	wait := make(chan error, 1)
	go func() { wait <- run.Wait() }()
	if _, err := io.WriteString(input, request); err != nil {
		t.Fatal(err)
	}
	var worker *os.Process
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for worker == nil {
		select {
		case err := <-wait:
			t.Fatalf("server exited before tool started: %v; stdout: %s; stderr: %s", err, &stdout, &stderr)
		case <-ctx.Done():
			<-wait
			t.Fatalf("tool did not report worker PID: %v; stderr: %s", ctx.Err(), &stderr)
		case <-ticker.C:
			data, err := os.ReadFile(pidPath)
			if err != nil {
				continue
			}
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				continue
			}
			if pid == run.Process.Pid || pid == os.Getpid() {
				t.Fatalf("tool executed outside the worker: %d", pid)
			}
			worker, err = os.FindProcess(pid)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	defer worker.Release()
	if blockStderr {
		time.Sleep(200 * time.Millisecond)
	}
	if err := worker.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-wait:
		if err == nil {
			t.Fatalf("expected worker failure; stdout: %s; stderr: %s", &stdout, &stderr)
		}
	case <-time.After(10 * time.Second):
		cancel()
		<-wait
		t.Fatalf("supervisor failed to exit after worker death (blocked stderr=%v)", blockStderr)
	}
	if ctx.Err() != nil {
		t.Fatalf("supervisor did not exit: %v; stderr: %s", ctx.Err(), &stderr)
	}
	initialized, failures := 0, 0
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		var message struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Error   *struct {
				Code int `json:"code"`
				Data struct {
					ExitCode *int `json:"exitCode"`
				} `json:"data"`
			} `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil || message.JSONRPC != "2.0" {
			t.Fatalf("stdout corrupted: %s; stderr: %s", scanner.Text(), &stderr)
		}
		switch string(message.ID) {
		case "1":
			if message.Error != nil {
				t.Fatalf("initialize failed: %s; stderr: %s", scanner.Text(), &stderr)
			}
			initialized++
		case `"worker-killed"`:
			if message.Error == nil || message.Error.Code != -32603 || message.Error.Data.ExitCode == nil || *message.Error.Data.ExitCode == 0 {
				t.Fatalf("missing crash diagnostic: %s; stderr: %s", scanner.Text(), &stderr)
			}
			failures++
		}
	}
	if err := scanner.Err(); err != nil || initialized != 1 || failures != 1 {
		t.Fatalf("expected initialize success and one tool error: initialized=%d failures=%d scan=%v; stderr: %s", initialized, failures, err, &stderr)
	}
}

func appendWithoutEnvKeys(environment []string, keys ...string) []string {
	blocked := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		blocked[strings.ToUpper(key)] = struct{}{}
	}

	filtered := make([]string, 0, len(environment))
	for _, item := range environment {
		key := item
		if index := strings.IndexByte(item, '='); index >= 0 {
			key = item[:index]
		}
		if _, found := blocked[strings.ToUpper(key)]; found {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}
