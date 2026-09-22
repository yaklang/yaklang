package ci_antiviral_check

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Exercise the real launcher with an engine that records every invocation.
// A static suite must still run its tests and propagate their failure, while
// existing service suites must still require the structured ready event.
func TestSuiteLauncherEngineRequirement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Essential Tests launcher runs on Unix")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(repoRoot(t), "scripts", "ci", "test-run-suite.sh")
	for _, tc := range []struct {
		name, needs, engine, sync        string
		runnerExit, wantExit             int
		wantEngine, wantRunner, wantSync bool
	}{
		{"static pass", "0", "exit", "0", 0, 0, false, true, false},
		{"static test failure", "0", "exit", "0", 7, 7, false, true, false},
		{"static missing binary", "0", "missing", "0", 0, 0, false, true, false},
		{"default ready", "", "ready", "0", 0, 0, true, true, false},
		{"default test failure", "", "ready", "0", 7, 7, true, true, false},
		{"default startup exit", "", "exit", "0", 0, 1, true, false, false},
		{"default no ready event", "", "silent", "0", 0, 1, true, false, false},
		{"default missing binary", "", "missing", "0", 0, 1, false, false, false},
		{"in-process sync rules", "0", "ready", "1", 0, 0, false, true, true},
		{"in-process sync failure", "0", "exit", "1", 0, 9, false, false, true},
		{"in-process sync missing binary", "0", "missing", "1", 0, 1, false, false, false},
		{"service and sync rules", "1", "ready", "1", 0, 0, true, true, true},
		{"invalid option", "typo", "ready", "0", 0, 1, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write := func(name, body string) string {
				t.Helper()
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
				return path
			}
			engine := write("engine", `#!/usr/bin/env bash
if [[ "$1" == "sync-rule" ]]; then
  echo called >> "$SYNC_CALLS"
  [[ "$ENGINE_MODE" == "exit" ]] && exit 9
  exit 0
fi
[[ "$1" == "grpc" ]] || exit 89
echo called >> "$ENGINE_CALLS"
[[ "$ENGINE_MODE" == "exit" ]] && exit 9
[[ "$ENGINE_MODE" == "ready" ]] && echo 'yak grpc ready {}'
exec sleep 10
`)
			if tc.engine == "missing" {
				engine = filepath.Join(dir, "missing")
			}
			runner := write("runner", "#!/usr/bin/env bash\necho called >> \"$RUNNER_CALLS\"\nexit \"$RUNNER_EXIT\"\n")
			// The timeout utility itself is unchanged. Check its contract and
			// delegate to the fixture runner; this also works on stock macOS.
			write("timeout", `#!/usr/bin/env bash
[[ "$1" == "--signal=TERM" && "$2" == "--kill-after=30s" && "$3" == "10s" ]] || exit 88
shift 3
exec "$@"
`)
			config := write("config.json", "[]\n")
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bash, script)
			cmd.WaitDelay = time.Second
			cmd.Env = append(os.Environ(),
				"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"YAK_BINARY_PATH="+engine, "TEST_CONFIG="+config,
				"TEST_BIN_DIR="+filepath.Join(dir, "logs"), "TEST_LOG_DIR="+filepath.Join(dir, "logs"),
				"TEST_RUNNER="+runner, "SUITE_NEEDS_GRPC="+tc.needs, "SUITE_SYNC_RULE="+tc.sync,
				"GRPC_READY_TIMEOUT=2", "SUITE_TIMEOUT=10s",
				"ENGINE_MODE="+tc.engine, "ENGINE_CALLS="+filepath.Join(dir, "engine.calls"),
				"SYNC_CALLS="+filepath.Join(dir, "sync.calls"),
				"RUNNER_CALLS="+filepath.Join(dir, "runner.calls"), "RUNNER_EXIT="+strconv.Itoa(tc.runnerExit),
			)
			output, runErr := cmd.CombinedOutput()
			code := 0
			if runErr != nil {
				var exit *exec.ExitError
				if !errors.As(runErr, &exit) {
					t.Fatalf("launcher: %v\n%s", runErr, output)
				}
				code = exit.ExitCode()
			}
			if ctx.Err() != nil || code != tc.wantExit {
				t.Fatalf("exit=%d want=%d context=%v\n%s", code, tc.wantExit, ctx.Err(), output)
			}
			for _, check := range []struct {
				file string
				want bool
			}{{"engine.calls", tc.wantEngine}, {"runner.calls", tc.wantRunner}, {"sync.calls", tc.wantSync}} {
				b, err := os.ReadFile(filepath.Join(dir, check.file))
				if check.want && (err != nil || strings.TrimSpace(string(b)) != "called") || !check.want && !os.IsNotExist(err) {
					t.Fatalf("%s want invocation=%v, got %q err=%v\n%s", check.file, check.want, b, err, output)
				}
			}
		})
	}
}
