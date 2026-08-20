package tests

import (
	"os"
	"path/filepath"
	"testing"
)

// TestClosureScalarWritebackDirect covers `f(); count` where a closure mutates
// an outer scalar: the parent must observe the updated value after the call.
func TestClosureScalarWritebackDirect(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "closure_direct.yak")
	if err := os.WriteFile(script, []byte(`
count = 0
f = func() {
    count += 1
}
f()
f()
assert count == 2, count
`), 0o644); err != nil {
		t.Fatal(err)
	}
	output := RunYakScriptFileWithCLI(t, script, nil)
	if output != "" {
		t.Fatalf("unexpected output: %s", output)
	}
}

// TestClosureScalarWritebackImplicitExit covers closures without an explicit
// ret statement: the captured writeback must still flush on the implicit exit.
func TestClosureScalarWritebackImplicitExit(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "closure_implicit.yak")
	if err := os.WriteFile(script, []byte(`
count = 0
f = func() {
    count += 1
}
f()
f()
assert count == 2, count
`), 0o644); err != nil {
		t.Fatal(err)
	}
	output := RunYakScriptFileWithCLI(t, script, nil)
	if output != "" {
		t.Fatalf("unexpected output: %s", output)
	}
}

// TestClosureScalarWritebackRetry covers the retry(100, cb) callback contract:
// die() inside the callback must propagate out of the callback (writeback
// included) so retry stops, and count must equal 4.
func TestClosureScalarWritebackRetry(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "retry_die.yak")
	if err := os.WriteFile(script, []byte(`
count = 0
retry(100, () => {
    defer recover()
    count++
    if count > 3 {
        die(111)
    }
    return true
})
assert count == 4, count
`), 0o644); err != nil {
		t.Fatal(err)
	}
	output := RunYakScriptFileWithCLI(t, script, nil)
	if output != "" {
		t.Fatalf("unexpected output: %s", output)
	}
}

// TestClosureScalarWritebackEach covers a yaklib method callback (Set.Each)
// mutating an outer scalar.
func TestClosureScalarWritebackEach(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "container_each.yak")
	if err := os.WriteFile(script, []byte(`
s = container.NewSet("1", "2")
count = 0
s.Each(func(val) {
    count += 1
    return true
})
assert count == 2, count
`), 0o644); err != nil {
		t.Fatal(err)
	}
	output := RunYakScriptFileWithCLI(t, script, nil)
	if output != "" {
		t.Fatalf("unexpected output: %s", output)
	}
}
