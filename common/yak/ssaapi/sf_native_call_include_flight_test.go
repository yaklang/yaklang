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

// TestIncludeFlightConcurrencySharesOneComputation asserts the flight-map
// semantics the fix builds on: N concurrent acquirers of one key get exactly
// one loader, every waiter receives the loader's result after release, and
// the map entry is cleaned up so a later acquire can load again.
func TestIncludeFlightConcurrencySharesOneComputation(t *testing.T) {
	const key = "flight-unit-test"
	const waiters = 8

	loaderEntry, isLoader := acquireIncludeFlight(key)
	if !isLoader {
		t.Fatal("first acquire was not the loader")
	}
	if _, ok := includeFlights.Load(key); !ok {
		t.Fatal("flight entry not registered")
	}

	var wg sync.WaitGroup
	var loaderSeen int64
	for i := 0; i < waiters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entry, isLoader := acquireIncludeFlight(key)
			if isLoader {
				// While the original entry is in flight no other acquirer may
				// become loader; asserted after the wait below.
				atomic.AddInt64(&loaderSeen, 1)
				releaseIncludeFlight(key, entry, nil, nil)
				return
			}
			<-entry.done
			if entry.err != nil {
				t.Errorf("waiter got error: %v", entry.err)
			}
		}()
	}
	// Give waiters a moment to block on the flight entry before releasing.
	time.Sleep(100 * time.Millisecond)
	releaseIncludeFlight(key, loaderEntry, Values{}, nil)
	wg.Wait()

	if got := atomic.LoadInt64(&loaderSeen); got != 0 {
		t.Fatalf("concurrent acquirers became loaders %d times during flight", got)
	}
	if _, still := includeFlights.Load(key); still {
		t.Fatal("flight entry not removed after release")
	}
}

// TestIncludeFlightWaiterBailsOnDoneOrContext asserts a waiter blocked on a
// flight entry stops honoring only its own context while the loader still
// runs: a runaway include must not hang the waiting rules as well.
func TestIncludeFlightWaiterBailsOnDoneOrContext(t *testing.T) {
	key := "flight-waiter-cancel-test"
	entry, isLoader := acquireIncludeFlight(key)
	if !isLoader {
		t.Fatal("first acquire was not the loader")
	}
	t.Cleanup(func() { releaseIncludeFlight(key, entry, nil, nil) })

	if _, isLoader := acquireIncludeFlight(key); isLoader {
		t.Fatal("second acquire became a loader")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	select {
	case <-entry.done:
		t.Fatal("waiter unblocked before the loader finished")
	case <-ctx.Done():
		// expected: a waiter with a done ctx must stop waiting immediately.
	case <-time.After(2 * time.Second):
		t.Fatal("waiter did not honor its ctx cancellation")
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

// TestIncludeCycleDetectionStopsRecursiveInclude asserts a lib including
// itself fails fast with the recursion error instead of recursing until the
// stack dies (pre-fix) or deadlocking on its own flight entry.
func TestIncludeCycleDetectionStopsRecursiveInclude(t *testing.T) {
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
	program.Program.Name = "include-cycle-test-" + t.Name()

	library, err := sfdb.CheckSyntaxFlowRuleContent(`
desc(lib: "recursive-lib");
$input<include("recursive-lib")> as $output;
alert $output;
`)
	if err != nil {
		t.Fatalf("compile recursive library: %v", err)
	}
	ctx := WithTaskLocalSyntaxFlowRuleLibraries(context.Background(), map[string]*schema.SyntaxFlowRule{
		"recursive-lib": library,
	})

	_, err = program.SyntaxFlowWithError(`
a* as $check;
$check<include("recursive-lib")> as $target;
`, QueryWithContext(ctx), QueryWithMemory())
	if err == nil {
		t.Fatal("recursive include unexpectedly succeeded")
	}
	if got := err.Error(); !strings.Contains(got, "recursive include") {
		t.Fatalf("recursive include error not detected: %v", err)
	}
}