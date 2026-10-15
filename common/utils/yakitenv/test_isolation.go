// Package yakitenv prepares the Yakit environment before packages which cache
// paths or open databases are initialized. Keep this package dependency-free
// apart from the standard library: common/utils imports it before its own init.
package yakitenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var testRoot string

// TestRoot returns the private root allocated at process startup. Unlike
// YAKIT_HOME it cannot be redirected by a later environment change.
func TestRoot() string { return testRoot }

// InTest reports whether this process was started as a Go test binary. Unlike
// testing.Testing, it also works during package initialization.
func InTest() bool {
	return isTestProcess(os.Args)
}

func isTestProcess(args []string) bool {
	if len(args) == 0 {
		return false
	}
	name := strings.ToLower(args[0])
	if strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".test.exe") {
		return true
	}
	// go test -c -o and IDE runners may rename the binary. They still pass
	// testing flags, including -test.paniconexit0 even without -v or -run.
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-test.") || strings.HasPrefix(arg, "--test.") {
			return true
		}
	}
	return false
}

func init() {
	if !InTest() {
		return
	}
	// Never trust an inherited YAKIT_HOME: developers and CI can have it set
	// to a real installation. Each test binary (including subprocesses) gets a
	// fresh root, so packages run by go test ./... cannot share state either.
	root, err := os.MkdirTemp("", "yaklang-test-*")
	if err != nil {
		panic(fmt.Errorf("create isolated Yakit test environment: %w", err))
	}
	home := filepath.Join(root, "yakit-projects")
	if err := os.Mkdir(home, 0o700); err != nil {
		panic(fmt.Errorf("create isolated Yakit test home: %w", err))
	}
	if err := os.Setenv("YAKIT_HOME", home); err != nil {
		panic(fmt.Errorf("set isolated Yakit test home: %w", err))
	}
	testRoot = root
	// Absolute paths and remote DSNs bypass YAKIT_HOME. Only discard inherited
	// values here; tests may subsequently set their own fixture paths explicitly.
	for _, key := range []string{
		"YAK_DEFAULT_PROJECT_DATABASE_NAME",
		"YAK_DEFAULT_PROFILE_DATABASE_NAME",
		"SSA_DATABASE_RAW",
	} {
		if err := os.Unsetenv(key); err != nil {
			panic(fmt.Errorf("clear inherited Yakit database setting %s: %w", key, err))
		}
	}
	// Keep the root for the process lifetime: async database writers and child
	// engines may still use it after a test returns. The OS temp directory also
	// keeps failed-test artifacts separate from the user's persisted data.
}
