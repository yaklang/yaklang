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

// TestIncludeFlightConcurrencySharesOneComputation asserts the flight-map
// semantics the fix builds on: N concurrent acquirers of one key get exactly
// one loader, every waiter receives the loader's result after release, and
// the map entry is cleaned up so a later acquire can load again.
func TestIncludeFlightConcurrencySharesOneComputation(t *testing.T) {
	const key = "flight-unit-test"
	const waiters = 8

	loaderEntry, isLoader := acquireIncludeFlight(key, context.Background())
	if !isLoader {
		t.Fatal("first acquire was not the loader")
	}
	if _, ok := includeFlights.Load(key); !ok {
		t.Fatal("flight entry not registered")
	}

	var wg sync.WaitGroup
	// acquired gates each acquirer past its acquireIncludeFlight call, so the
	// release below cannot race a waiter that has not acquired yet.
	acquired := make(chan struct{}, waiters)
	var loaderSeen int64
	for i := 0; i < waiters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entry, isLoader := acquireIncludeFlight(key, context.Background())
			acquired <- struct{}{}
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
	// Deterministic barrier: every acquirer has registered before the release,
	// so none can slip in as a new loader after it.
	for i := 0; i < waiters; i++ {
		<-acquired
	}
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
	entry, isLoader := acquireIncludeFlight(key, context.Background())
	if !isLoader {
		t.Fatal("first acquire was not the loader")
	}
	t.Cleanup(func() { releaseIncludeFlight(key, entry, nil, nil) })

	if _, isLoader := acquireIncludeFlight(key, context.Background()); isLoader {
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
		t.Fatalf("recursive include error not detected: %v", got)
	}
}

// TestIncludeFlightWaiterTakesOverCanceledLoader asserts the failure-propagation
// semantics of the flight map: when a loader dies because its own rule ctx was
// canceled (timeout / budget overflow), a waiter whose ctx is still healthy does
// not inherit that error — it re-acquires the flight and the first such waiter
// becomes the new loader. Only a waiter whose own ctx is done bails with the
// cancellation error. A canceled loader therefore cannot silently empty the
// healthy waiting rules.
func TestIncludeFlightWaiterTakesOverCanceledLoader(t *testing.T) {
	const key = "flight-takeover-test"
	GetSFIncludeCache().Purge()
	t.Cleanup(func() { GetSFIncludeCache().Purge() })

	loaderCtx, cancelLoader := context.WithCancel(context.Background())
	defer cancelLoader()

	loaderStarted := make(chan struct{})
	failLoader := make(chan struct{})
	loaderDone := make(chan error, 1)
	go func() {
		_, err := includeFlightValues(key, "takeover-lib", loaderCtx, func() (Values, error) {
			close(loaderStarted)
			<-failLoader
			return nil, loaderCtx.Err()
		})
		loaderDone <- err
	}()
	<-loaderStarted // entry registered; loader parked inside compute.

	// Healthy waiter: must end up as the (new) loader and produce a result.
	waiterComputed := make(chan struct{}, 1)
	healthyDone := make(chan error, 1)
	go func() {
		_, err := includeFlightValues(key, "takeover-lib", context.Background(), func() (Values, error) {
			waiterComputed <- struct{}{}
			return Values{}, nil
		})
		healthyDone <- err
	}()

	// Proof waiter: its own ctx is already canceled while the loader's entry is
	// still registered (the loader cannot release before failLoader closes), so
	// it must deterministically bail with its own cancellation error and never
	// compute. Waiting for its return also gives the healthy waiter wall time to
	// park on the same entry; should the healthy waiter instead race past the
	// release and become a fresh loader, the asserts below accept that too.
	proofDone := make(chan error, 1)
	go func() {
		proofCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := includeFlightValues(key, "takeover-lib", proofCtx, func() (Values, error) {
			return nil, utils.Errorf("proof waiter must never compute")
		})
		proofDone <- err
	}()

	select {
	case err := <-proofDone:
		if err == nil || !strings.Contains(err.Error(), "waiting canceled") {
			t.Fatalf("proof waiter did not bail on its own canceled ctx: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("proof waiter never returned")
	}

	// Kill the loader via its own ctx, then let its compute return that error.
	cancelLoader()
	close(failLoader)

	select {
	case err := <-loaderDone:
		if err == nil || !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("loader did not fail with its own ctx error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("loader never returned")
	}

	select {
	case err := <-healthyDone:
		if err != nil {
			t.Fatalf("healthy waiter inherited the canceled loader's error instead of taking over: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("healthy waiter never returned")
	}
	select {
	case <-waiterComputed:
	default:
		t.Fatal("healthy waiter returned without computing as the new loader")
	}

	if _, still := includeFlights.Load(key); still {
		t.Fatal("flight entry not removed after takeover")
	}
}

// TestIncludeFlightWaiterInheritsContentFailure asserts the complementary rule:
// when the loader failed for a reason every waiter would reproduce identically
// (no value found, rule error — detected by the loader's ctx still being alive),
// waiters inherit the error instead of taking over. Unconditional takeover would
// serialize the very herd the flight map exists to collapse.
func TestIncludeFlightWaiterInheritsContentFailure(t *testing.T) {
	const key = "flight-inherit-test"
	entry := &includeFlightEntry{
		done:      make(chan struct{}),
		loaderCtx: context.Background(),
		err:       utils.Errorf("include \"inherit-lib\": no value found"),
	}
	close(entry.done)
	includeFlights.Store(key, entry)
	t.Cleanup(func() { includeFlights.Delete(key) })

	recomputed := false
	_, err := includeFlightValues(key, "inherit-lib", context.Background(), func() (Values, error) {
		recomputed = true
		return Values{}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "no value found") {
		t.Fatalf("waiter did not inherit the loader's content failure: %v", err)
	}
	if recomputed {
		t.Fatal("waiter re-ran the include for a deterministic content failure")
	}
}