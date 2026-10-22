package ssa

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// Concurrent missing-import materialization used to share one virtual-function
// builder. Each type now owns a private function; same-name imports share one result.
func TestFakeImportTypeConcurrentVirtualBuilder(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(t.Name()))
	require.NoError(t, err)
	app := NewProgram(cfg, ProgramCacheMemory, Application, nil, "", 0)
	app.config.VirtualImport = true
	lib, err := app.GenerateVirtualLib("com.missing")
	require.NoError(t, err)
	require.NotNil(t, lib)
	lib.config.VirtualImport = true

	const goroutines = 16
	const each = 40
	errCh := make(chan error, goroutines)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("goroutine %d panic: %v", g, r)
				}
			}()
			<-start
			for n := 0; n < each; n++ {
				typ := fakeImportType(lib, fmt.Sprintf("T%d_%d", g, n), nil)
				if typ == nil {
					errCh <- fmt.Errorf("goroutine %d iteration %d returned nil type", g, n)
					return
				}
			}
		}(g)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	close(start)
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("timed out: virtual builder use deadlocked")
	}
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

// Same-name imports take one singleflight slot. The callback must not wait on
// that slot again, and the whole set must finish without a mutex held across Emit.
func TestFakeImportSameTypeDoesNotDeadlock(t *testing.T) {
	cfg, err := ssaconfig.New(ssaconfig.ModeSSACompile, ssaconfig.WithSetProgramName(t.Name()))
	require.NoError(t, err)
	app := NewProgram(cfg, ProgramCacheMemory, Application, nil, "", 0)
	app.config.VirtualImport = true
	lib, err := app.GenerateVirtualLib("com.missing")
	require.NoError(t, err)
	lib.config.VirtualImport = true

	var first Type
	var once sync.Once
	var wg sync.WaitGroup
	errCh := make(chan error, 16)
	start := make(chan struct{})
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("panic: %v", r)
				}
			}()
			<-start
			typ := fakeImportType(lib, "Foo", nil)
			if typ == nil {
				errCh <- fmt.Errorf("nil type")
				return
			}
			once.Do(func() { first = typ })
			if first != nil && typ != first {
				errCh <- fmt.Errorf("same name produced two types")
			}
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	close(start)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out: same-type virtual import deadlocked")
	}
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}
