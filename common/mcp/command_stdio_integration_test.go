package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMUSTPASS_MCPCommandStdioKeepsStdoutJSONRPCOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping process-level MCP stdio test in short mode")
	}

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
	build := exec.CommandContext(buildContext, "go", "build", "-o", binaryPath, "./common/yak/cmd")
	build.Dir = repositoryRoot
	if output, buildErr := build.CombinedOutput(); buildErr != nil {
		t.Fatalf("build yak command: %v\n%s", buildErr, output)
	}

	request := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"stdio-regression-test","version":"1"}}}` + "\n"
	runContext, cancelRun := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelRun()
	run := exec.CommandContext(
		runContext,
		binaryPath,
		"mcp",
		"--transport",
		"stdio",
		"--enable-aitool-framework",
	)
	run.Env = appendWithoutEnvKeys(
		os.Environ(),
		"YAKIT_HOME",
		"YAK_MCP_STDIO",
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

	responseCount := 0
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
			JSONRPC string `json:"jsonrpc"`
		}
		if decodeErr := json.Unmarshal(line, &response); decodeErr != nil {
			t.Fatalf("decode stdout response: %v", decodeErr)
		}
		if response.JSONRPC != "2.0" {
			t.Fatalf("stdout JSON is not JSON-RPC 2.0: %s", line)
		}
		responseCount++
	}
	if scanErr := scanner.Err(); scanErr != nil {
		t.Fatalf("scan stdout: %v", scanErr)
	}
	if responseCount != 1 {
		t.Fatalf("got %d JSON-RPC responses, want 1; stdout: %q", responseCount, stdout.String())
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
