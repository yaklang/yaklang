package tests

import (
	"os"
	"path/filepath"
	"strings"
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

// TestClosureScalarWritebackAsyncGo covers a goroutine that mutates a captured
// scalar, then joins before the parent reads it. The read is also an element
// of a format list, which binds a second variable name on the side effect.
// The parent must still observe the updated cell.
func TestClosureScalarWritebackAsyncGo(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "closure_async_go.yak")
	if err := os.WriteFile(script, []byte(`
var count = 0
ch = make(chan any)
go func {
    count++
    go func {
        ch <- 0
    }
}
<-ch
assert count == 1, "got %v" % [count]
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

// TestClosureScalarWritebackLockedConcurrent covers a captured counter
// incremented under a mutex from many goroutines. The parent joins after each
// call returns, so it observes the cell only once every invocation has
// finished. An exit replay of the increment would land after Unlock and drop
// updates.
func TestClosureScalarWritebackLockedConcurrent(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "closure_locked_concurrent.yak")
	if err := os.WriteFile(script, []byte(`
count = 0
lock = sync.NewLock()
f = func() {
    lock.Lock()
    count++
    lock.Unlock()
    spin = 0
    for k = 0; k < 2000; k++ {
        spin = spin + k
    }
}
ch = make(chan any)
n = 32
for i = 0; i < n; i++ {
    go func {
        f()
        ch <- 1
    }
}
for i = 0; i < n; i++ {
    <-ch
}
println(count)
assert count == n, count
`), 0o644); err != nil {
		t.Fatal(err)
	}
	output := RunYakScriptFileWithCLI(t, script, nil)
	if strings.TrimSpace(output) != "32" {
		t.Fatalf("got %q, want 32", output)
	}
}

// TestClosureScalarWritebackBranchIf covers several branch-local assignments
// of outer bools in one closure. Each if merges through its own phi, and only
// the last of those phis shares the function exit. Every assignment must still
// reach the captured cell, and a later call that skips an assignment must
// leave the previous update in place.
func TestClosureScalarWritebackBranchIf(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "closure_branch_if.yak")
	if err := os.WriteFile(script, []byte(`
check = false
check2 = false
check3 = false
f = func(raw) {
    if raw { check = true }
    if raw { check2 = true }
    if !raw { check3 = true }
}
f(true)
assert check
assert check2
assert check3 == false
f(false)
assert check
assert check2
assert check3
`), 0o644); err != nil {
		t.Fatal(err)
	}
	output := RunYakScriptFileWithCLI(t, script, nil)
	if output != "" {
		t.Fatalf("unexpected output: %s", output)
	}
}

// TestClosureSliceWritebackEarlyReturn covers a closure that appends to an
// outer slice while an early-return branch skips the assignment: the parent
// must observe only the appended elements, and the early-return path must not
// clobber the captured slot with an uninitialized value.
func TestClosureSliceWritebackEarlyReturn(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "closure_slice_early_return.yak")
	if err := os.WriteFile(script, []byte(`
files = []
f = (dir, path) => {
    if dir {
        return
    }
    files = append(files, path)
}
f(1, "a")
f(0, "b")
f(0, "c")
assert len(files) == 2, len(files)
assert files[0] == "b", files
assert files[1] == "c", files
`), 0o644); err != nil {
		t.Fatal(err)
	}
	output := RunYakScriptFileWithCLI(t, script, nil)
	if output != "" {
		t.Fatalf("unexpected output: %s", output)
	}
}

// TestClosureSliceWritebackIndirect covers a closure invoked through a yaklib
// callback (filesys.walk-style indirection) that appends to an outer slice:
// the parent reads the final value through the closure's by-ref free-value
// slot after the callback completes.
func TestClosureSliceWritebackIndirect(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "closure_slice_indirect.yak")
	if err := os.WriteFile(script, []byte(`
files = []
each = (items, cb) => {
    for item in items {
        cb(item)
    }
}
each(["a", "b", "c"], (p) => {
    if p == "b" {
        return
    }
    files = append(files, p)
})
assert len(files) == 2, len(files)
assert files[0] == "a", files
assert files[1] == "c", files
`), 0o644); err != nil {
		t.Fatal(err)
	}
	output := RunYakScriptFileWithCLI(t, script, nil)
	if output != "" {
		t.Fatalf("unexpected output: %s", output)
	}
}
