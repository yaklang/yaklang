package tests

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireOutputLinesUnordered(t *testing.T, output string, want ...string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	sort.Strings(lines)
	sort.Strings(want)
	require.Equal(t, want, lines)
}

func TestGoStmt_PrintlnAutoWait(t *testing.T) {
	code := `
func main() {
	go println(1)
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	require.Equal(t, "1\n", output)
}

func TestGoStmt_DirectFunctionCallAutoWait(t *testing.T) {
	code := `
func f(x) {
	println(x)
}

func main() {
	go f(10)
	go println(20)
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	requireOutputLinesUnordered(t, output, "10", "20")
}

func TestGoStmt_ObjectMemberUpdate_WithSyncWaitGroup(t *testing.T) {
	code := `
func update(obj, wg) {
	obj.key = 2
	wg.Done()
}

func main() {
	a = {
		"key": 1,
	}
	wg = sync.NewWaitGroup()
	wg.Add(1)
	go update(a, wg)
	wg.Wait()
	println(a.key)
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	require.Equal(t, "2\n", output)
}

func TestSync_WaitGroup_GoroutinesComplete(t *testing.T) {
	code := `
func worker(wg, value) {
	println(value)
	wg.Done()
}

func main() {
	wg = sync.NewWaitGroup()
	wg.Add(2)
	go worker(wg, 1)
	go worker(wg, 2)
	wg.Wait()
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	requireOutputLinesUnordered(t, output, "1", "2")
}

func TestSync_WaitGroup_DefaultAdd(t *testing.T) {
	code := `
func worker(wg, value) {
	println(value)
	wg.Done()
}

func main() {
	wg = sync.NewWaitGroup()
	wg.Add()
	go worker(wg, 11)
	wg.Wait()
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	require.Equal(t, "11\n", output)
}

func TestSync_SizedWaitGroup_GoroutinesComplete(t *testing.T) {
	code := `
func sizedWorker(wg, value) {
	println(value)
	wg.Done()
}

func main() {
	wg = sync.NewSizedWaitGroup(1)
	wg.Add()
	go sizedWorker(wg, 3)
	wg.Add()
	go sizedWorker(wg, 4)
	wg.Wait()
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	requireOutputLinesUnordered(t, output, "3", "4")
}

func TestSync_Mutex_SerializesGoRoutineUpdates(t *testing.T) {
	code := `
func worker(mu, value) {
	mu.Lock()
	println(value)
	mu.Unlock()
}

func main() {
	mu = sync.NewMutex()
	go worker(mu, 5)
	go worker(mu, 6)
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	requireOutputLinesUnordered(t, output, "5", "6")
}

func TestSync_LockAlias_SerializesGoRoutineUpdates(t *testing.T) {
	code := `
func worker(mu, value) {
	mu.Lock()
	println(value)
	mu.Unlock()
}

func main() {
	mu = sync.NewLock()
	go worker(mu, 9)
	go worker(mu, 10)
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	requireOutputLinesUnordered(t, output, "9", "10")
}

func TestSync_RWMutex_ReadWriteMethods(t *testing.T) {
	code := `
func reader(mu, value) {
	mu.RLock()
	println(value)
	mu.RUnlock()
}

func writer(mu, value) {
	mu.Lock()
	println(value)
	mu.Unlock()
}

func main() {
	mu = sync.NewRWMutex()
	go reader(mu, 7)
	go writer(mu, 8)
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	requireOutputLinesUnordered(t, output, "7", "8")
}

func TestSync_Map_StoreLoad(t *testing.T) {
	code := `
func main() {
	m = sync.NewMap()
	m.Store("a", 12)
	println(m.Load("a"))
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	require.Equal(t, "12\n", output)
}

func TestSync_Pool_PutGet(t *testing.T) {
	code := `
func main() {
	p = sync.NewPool()
	p.Put(13)
	println(p.Get())
}
`
	output := runBinaryWithEnv(t, code, "main", nil)
	require.Equal(t, "13\n", output)
}
