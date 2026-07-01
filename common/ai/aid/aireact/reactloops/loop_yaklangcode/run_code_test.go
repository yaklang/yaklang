package loop_yaklangcode

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldAutoRunYakSelfTest(t *testing.T) {
	assert.False(t, ShouldAutoRunYakSelfTest("yakit.AutoInitYakit()\ncli.check()"))
	assert.True(t, ShouldAutoRunYakSelfTest("if YAK_MAIN { runSelfTest() }"))
	assert.True(t, ShouldAutoRunYakSelfTest("YAK_MAIN && runSelfTest()"))
}

func TestRunYakSelfTest_Success(t *testing.T) {
	code := `
func runSelfTest() {
    assert 1 + 1 == 2, "math works"
}
if YAK_MAIN {
    runSelfTest()
}
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := RunYakSelfTest(ctx, code, "test_success.yak", 15)
	require.NoError(t, err)
	assert.False(t, result.Truncated)
}

func TestRunYakSelfTest_AssertFailure(t *testing.T) {
	code := `
DISCOVERED = {}
mirrorNewWebsitePath = func(isHttps, url, req, rsp, body) { DISCOVERED[url] = true }
func runSelfTest() {
    mirrorNewWebsitePath(false, "http://t/a", []byte("GET / HTTP/1.1\r\nHost: t\r\n\r\n"), []byte("HTTP/1.1 200 OK\r\n\r\n"), []byte(""))
    assert len(DISCOVERED) == 2, "want 2"
}
if YAK_MAIN { runSelfTest() }
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := RunYakSelfTest(ctx, code, "test_assert_fail.yak", 15)
	require.Error(t, err)
	feedback := FormatRunFailureForAI(result, err)
	assert.Contains(t, feedback, "modify_code")
	assert.True(t, strings.Contains(feedback, "want 2") || strings.Contains(feedback, "assert") || strings.Contains(err.Error(), "assert"))
}

func TestRunYakSelfTest_Panic(t *testing.T) {
	code := `
func runSelfTest() {
    panic("boom")
}
if YAK_MAIN { runSelfTest() }
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := RunYakSelfTest(ctx, code, "test_panic.yak", 15)
	require.Error(t, err)
	feedback := FormatRunFailureForAI(result, err)
	assert.Contains(t, feedback, "modify_code")
	assert.True(t, strings.Contains(feedback, "boom") || strings.Contains(err.Error(), "boom"))
}

func TestRunYakSelfTest_DebounceTimerDoesNotCrashEngine(t *testing.T) {
	code := `
counter = make(map[string]int)
reloadTarget = func(path, eventType) {
    n = counter[path]
    if n == nil { n = 0 }
    counter[path] = n + 1
    return nil
}
debounceLock = sync.NewMutex()
pendingTimers = make(map[string]any)
lastEventType = make(map[string]string)
DEBOUNCE_DURATION = time.ParseDuration("50ms")~
fireReload = func(path, eventType) {
    if eventType == nil { eventType = "modify" }
    debounceLock.Lock()
    timer = pendingTimers[path]
    if timer != nil { timer.Stop() }
    lastEventType[path] = eventType
    pendingTimers[path] = time.AfterFunc(DEBOUNCE_DURATION, func() {
        debounceLock.Lock()
        delete(pendingTimers, path)
        et = lastEventType[path]
        debounceLock.Unlock()
        reloadTarget(path, et)
    })
    debounceLock.Unlock()
}
func runSelfTest() {
    fireReload("a.yaml", "modify")
    fireReload("a.yaml", "modify")
    time.Sleep(0.15)
    n = counter["a.yaml"]
    if n == nil { n = 0 }
    assert n == 1, sprintf("want 1 got %d", n)
}
if YAK_MAIN { runSelfTest() }
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := RunYakSelfTest(ctx, code, "debounce_test.yak", 15)
	require.NoError(t, err, "output=%s err=%v", result.Output, err)
}

func TestTruncateYakRunOutput(t *testing.T) {
	long := strings.Repeat("x", yakRunOutputMaxBytes+100)
	out, truncated := truncateYakRunOutput(long)
	assert.True(t, truncated)
	assert.Len(t, out, yakRunOutputMaxBytes)
}
