package yakitenv

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsTestProcess(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{"empty", nil, false},
		{"unix", []string{"/tmp/consts.test"}, true},
		{"windows", []string{`C:\Temp\yakgrpc.test.exe`}, true},
		{"renamed", []string{"/tmp/ide-runner", "-test.paniconexit0"}, true},
		{"benchmark", []string{"runner", "-test.bench=."}, true},
		{"double dash", []string{"runner", "--test.run=TestX"}, true},
		{"production", []string{"yak", "grpc"}, false},
		{"script argument", []string{"yak", "script.yak", "--url=https://x/-test.run"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTestProcess(tc.args); got != tc.want {
				t.Fatalf("isTestProcess(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestIsolationBeforePackageInitialization(t *testing.T) {
	if os.Getenv("YAK_TEST_ENV_CHILD") == "1" {
		if !InTest() {
			t.Fatal("test process was not detected")
		}
		for _, key := range []string{"YAK_DEFAULT_PROJECT_DATABASE_NAME", "YAK_DEFAULT_PROFILE_DATABASE_NAME", "SSA_DATABASE_RAW"} {
			if os.Getenv(key) != "" {
				t.Fatalf("inherited database setting %s survived initialization", key)
			}
		}
		if err := os.WriteFile(os.Getenv("YAK_TEST_ENV_RESULT"), []byte(os.Getenv("YAKIT_HOME")), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	production := t.TempDir()
	result := filepath.Join(t.TempDir(), "home")
	// Use the running executable, never a caller-controlled argv[0]. No shell
	// or externally supplied command arguments are involved in these probes.
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var previous string
	for _, inheritedHome := range []string{production, "", production} {
		cmd := exec.Command(executable, "-test.run=^TestIsolationBeforePackageInitialization$")
		cmd.Env = append(os.Environ(), "YAK_TEST_ENV_CHILD=1", "YAK_TEST_ENV_RESULT="+result,
			"YAKIT_HOME="+inheritedHome,
			"YAK_DEFAULT_PROJECT_DATABASE_NAME="+filepath.Join(production, "project.db"),
			"YAK_DEFAULT_PROFILE_DATABASE_NAME="+filepath.Join(production, "profile.db"),
			"SSA_DATABASE_RAW=postgres://production.example/database")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("child: %v\n%s", err, output)
		}
		raw, err := os.ReadFile(result)
		if err != nil {
			t.Fatal(err)
		}
		home := string(raw)
		if home == "" || !filepath.IsAbs(home) || home == production || home == previous {
			t.Fatalf("test home must be absolute, fresh and isolated: %q (previous %q)", home, previous)
		}
		if filepath.Base(home) != "yakit-projects" || !strings.HasPrefix(filepath.Base(filepath.Dir(home)), "yaklang-test-") {
			t.Fatalf("refusing to clean up an unexpected test home: %q", home)
		}
		// These children have exited and have no async users left.
		if err := os.RemoveAll(filepath.Dir(home)); err != nil {
			t.Fatal(err)
		}
		previous = home
	}
	entries, err := os.ReadDir(production)
	if err != nil || len(entries) != 0 {
		t.Fatalf("inherited home changed: entries=%v, err=%v", entries, err)
	}
}

func TestProductionEnvironmentUnchanged(t *testing.T) {
	// Build a regular executable: unlike a test subprocess it must retain all
	// configured paths, including remote SSA DSNs.
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	program := `package main
import (
 "encoding/json"
 "os"
 _ "github.com/yaklang/yaklang/common/utils/yakitenv"
)
func main() {
 values := map[string]string{}
 for _, key := range []string{"YAKIT_HOME", "YAK_DEFAULT_PROJECT_DATABASE_NAME", "YAK_DEFAULT_PROFILE_DATABASE_NAME", "SSA_DATABASE_RAW"} { values[key] = os.Getenv(key) }
 json.NewEncoder(os.Stdout).Encode(values)
}`
	if err := os.WriteFile(source, []byte(program), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "yak-env-probe.exe")
	if output, err := exec.Command("go", "build", "-o", binary, source).CombinedOutput(); err != nil {
		t.Fatalf("build probe: %v\n%s", err, output)
	}
	for _, configured := range []bool{false, true} {
		cmd := exec.Command(binary)
		want := map[string]string{
			"YAKIT_HOME": "", "YAK_DEFAULT_PROJECT_DATABASE_NAME": "",
			"YAK_DEFAULT_PROFILE_DATABASE_NAME": "", "SSA_DATABASE_RAW": "",
		}
		if configured {
			want["YAKIT_HOME"] = dir
			want["YAK_DEFAULT_PROJECT_DATABASE_NAME"] = filepath.Join(dir, "project.db")
			want["YAK_DEFAULT_PROFILE_DATABASE_NAME"] = filepath.Join(dir, "profile.db")
			want["SSA_DATABASE_RAW"] = "postgres://production.example/database"
		}
		for _, env := range os.Environ() {
			key, _, _ := strings.Cut(env, "=")
			if _, exists := want[key]; !exists {
				cmd.Env = append(cmd.Env, env)
			}
		}
		for key, value := range want {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("production probe: %v\n%s", err, output)
		}
		got := map[string]string{}
		if err := json.Unmarshal(output, &got); err != nil {
			t.Fatal(err)
		}
		for key, value := range want {
			if got[key] != value {
				t.Errorf("production setting %s changed: got %q, want %q", key, got[key], value)
			}
		}
	}
}
