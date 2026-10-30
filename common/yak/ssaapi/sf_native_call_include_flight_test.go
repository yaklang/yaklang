package ssaapi

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// includeFlightFixture compiles a small named program and binds a task-local
// lib rule (the production scan path), returning the program and the ctx that
// resolves the lib. The profile DB is deliberately unavailable so resolution
// can only succeed through the task-local snapshot.
func includeFlightFixture(t *testing.T) (*Program, context.Context) {
	t.Helper()
	bindUnavailableProfileDatabase(t)
	GetSFIncludeCache().Purge()
	t.Cleanup(func() { GetSFIncludeCache().Purge() })

	program, err := Parse(
		`a1 = 1; a2 = "hello world";`,
		WithLanguage(ssaconfig.Yak),
	)
	if err != nil {
		t.Fatalf("compile test program: %v", err)
	}
	program.Program.Name = "include-flight-test-" + t.Name()

	const libName = "include-flight-lib"
	library, err := sfdb.CheckSyntaxFlowRuleContent(`
desc(lib: "include-flight-lib");
$input?{have: "hello world"} as $output;
alert $output;
`)
	if err != nil {
		t.Fatalf("compile task-local library: %v", err)
	}
	ctx := WithTaskLocalSyntaxFlowRuleLibraries(context.Background(), map[string]*schema.SyntaxFlowRule{
		libName: library,
	})
	return program, ctx
}

// TestIncludeFlightConcurrentMissComputesOnce asserts that concurrent callers
// of one flight key share a single computation and that the result is cached.
func TestIncludeFlightConcurrentMissComputesOnce(t *testing.T) {
	const key = "flight-share-test"
	GetSFIncludeCache().Purge()
	t.Cleanup(func() { GetSFIncludeCache().Purge() })

	loaderStarted := make(chan struct{})
	releaseLoader := make(chan struct{})
	var computations int64
	compute := func() (Values, error) {
		if atomic.AddInt64(&computations, 1) == 1 {
			close(loaderStarted)
		}
		<-releaseLoader
		return Values{}, nil
	}

	const callers = 8
	var wg sync.WaitGroup
	started := make(chan struct{}, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			started <- struct{}{}
			if _, err := includeFlightValues(context.Background(), key, "flight-lib", compute); err != nil {
				t.Errorf("include flight: %v", err)
			}
		}()
	}
	for i := 0; i < callers; i++ {
		<-started
	}
	// The key is registered before the loader goroutine starts, so once it has
	// begun every later caller joins it. Give the remaining callers time to
	// park before releasing the loader.
	<-loaderStarted
	time.Sleep(100 * time.Millisecond)
	close(releaseLoader)
	wg.Wait()

	if got := atomic.LoadInt64(&computations); got != 1 {
		t.Fatalf("include computed %d times for one flight key, want 1", got)
	}
	if _, ok := GetSFIncludeCache().Get(key); !ok {
		t.Fatal("flight result was not cached")
	}
}

// TestIncludeFlightWaiterHonorsItsContext asserts a waiter leaves while the
// loader is still running: a slow include must not block the waiting rules.
func TestIncludeFlightWaiterHonorsItsContext(t *testing.T) {
	const key = "flight-waiter-ctx-test"
	GetSFIncludeCache().Purge()
	t.Cleanup(func() { GetSFIncludeCache().Purge() })

	loaderStarted := make(chan struct{})
	releaseLoader := make(chan struct{})
	loaderDone := make(chan error, 1)
	go func() {
		_, err := includeFlightValues(context.Background(), key, "flight-lib", func() (Values, error) {
			close(loaderStarted)
			<-releaseLoader
			return Values{}, nil
		})
		loaderDone <- err
	}()
	<-loaderStarted // the flight key is registered before the loader starts

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := includeFlightValues(ctx, key, "flight-lib", func() (Values, error) {
		return nil, utils.Errorf("waiter must not compute")
	})
	if err == nil || !strings.Contains(err.Error(), "waiting canceled") {
		t.Fatalf("waiter did not bail on its own ctx: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("waiter did not return promptly after ctx cancellation")
	}

	close(releaseLoader)
	if err := <-loaderDone; err != nil {
		t.Fatalf("loader failed: %v", err)
	}
}

// TestIncludeFlightLoaderPanicBecomesError asserts a panicking include fails
// the call instead of crashing the process from singleflight's goroutine.
func TestIncludeFlightLoaderPanicBecomesError(t *testing.T) {
	const key = "flight-panic-test"
	GetSFIncludeCache().Purge()
	t.Cleanup(func() { GetSFIncludeCache().Purge() })

	_, err := includeFlightValues(context.Background(), key, "flight-lib", func() (Values, error) {
		panic("boom")
	})
	if err == nil || !strings.Contains(err.Error(), "panic") {
		t.Fatalf("panic was not converted to an include error: %v", err)
	}
}

// TestIncludeConcurrentSameLibQueriesShareCache asserts the end-to-end path:
// many rules including the same lib on the same program concurrently all get
// the correct result and the cache holds it after the first computation.
func TestIncludeConcurrentSameLibQueriesShareCache(t *testing.T) {
	program, ctx := includeFlightFixture(t)

	const queries = 8
	var wg sync.WaitGroup
	for i := 0; i < queries; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := program.SyntaxFlowWithError(`
a* as $check;
$check<include("include-flight-lib")> as $target;
`, QueryWithContext(ctx), QueryWithMemory())
			if err != nil {
				t.Errorf("concurrent include query: %v", err)
				return
			}
			if got := result.GetValues("target"); len(got) == 0 {
				t.Error("concurrent include query returned no values")
			}
		}()
	}
	wg.Wait()

	if count := GetSFIncludeCache().Count(); count == 0 {
		t.Fatal("include result was not cached after concurrent queries")
	}
}
