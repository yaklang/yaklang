package ssaapi

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

// unitBuildNode is one SCC, the smallest unit whose dependencies can be
// released independently. A batch may contain many unrelated SCCs, so a wave
// barrier needlessly makes their dependents wait for the slowest sibling.
type unitBuildNode struct {
	keys       []string
	libraries  []*ssa.Program
	remaining  int
	dependents []int
	exclusive  bool
}

func compileUnitBuildNodes(prog *ssa.Program, plan *UnitPlan, batch compileUnitExecutionBatch) []unitBuildNode {
	selected := make(map[string]bool, len(batch.unitKeys))
	for _, key := range batch.unitKeys {
		selected[key] = true
	}
	nodes := make([]unitBuildNode, 0, len(plan.Order))
	byKey := make(map[string]int, len(batch.unitKeys))
	builtOwner := make(map[*ssa.Program]bool)
	application := prog.GetApplication()
	for _, scc := range plan.Order {
		var node unitBuildNode
		for _, unit := range scc {
			if unit == nil || !selected[unit.Key] {
				continue
			}
			byKey[unit.Key] = len(nodes)
			node.keys = append(node.keys, unit.Key)
			library := prog.ProgramForCompileUnit(unit.Key)
			if library == nil || builtOwner[library] {
				continue
			}
			builtOwner[library] = true
			node.libraries = append(node.libraries, library)
			node.exclusive = node.exclusive || library == application
		}
		if len(node.keys) != 0 {
			nodes = append(nodes, node)
		}
	}
	seenEdges := make(map[[2]int]bool)
	for _, edge := range plan.Edges {
		from, fromOK := byKey[edge.From]
		to, toOK := byKey[edge.To]
		if !fromOK || !toOK || from == to || seenEdges[[2]int{from, to}] {
			continue
		}
		seenEdges[[2]int{from, to}] = true
		nodes[from].remaining++
		nodes[to].dependents = append(nodes[to].dependents, from)
	}
	return nodes
}

type unitBuildResult struct {
	index int
	err   error
}

// executeCompileUnitBatch is the single entry point for one compile batch: it
// builds every library in the batch through the DAG scheduler (concurrency 1 is
// the scheduler's own serial mode, not a separate code path), then runs the
// batch cleanup — struct scan, unit/instruction/aux flushes — in the same
// goroutine, so the next batch can never observe a half-finished previous one.
//
// drainUnscheduled tells the scheduler to cover upstream libraries the plan did
// not schedule itself; frontends without a concurrency-safe unit build rely on
// that single-pass coverage to reach their shared application scope.
//
// The returned duration covers only the pipeline build so callers can keep
// their existing pre-handler accounting unchanged.
func executeCompileUnitBatch(
	ctx context.Context,
	prog *ssa.Program,
	plan *UnitPlan,
	batch compileUnitExecutionBatch,
	concurrency, batchIndex int,
	cleanup compileBatchCleanup,
	drainUnscheduled bool,
) (time.Duration, error) {
	buildStart := time.Now()
	peak, err := buildCompileUnitPipeline(ctx, prog, plan, batch, concurrency)
	buildCost := time.Since(buildStart)
	log.Infof("compile unit pipeline batch=%d groups=%d concurrency=%d peak_active=%d cost=%v",
		batchIndex+1, len(batch.unitKeys), concurrency, peak, buildCost)
	if err != nil {
		return buildCost, err
	}
	if drainUnscheduled {
		prog.DrainUnscheduledApplicationUpstreams(batch.unitKeys)
	}
	if err := cleanup.run(); err != nil {
		return buildCost, err
	}
	return buildCost, nil
}

// buildCompileUnitPipeline runs ready SCCs immediately when their own
// prerequisites finish. Every library has one owner in the batch, a global
// semaphore bounds all library builds, and application work is exclusive.
// On cancellation or panic it joins all running work before returning.
func buildCompileUnitPipeline(ctx context.Context, prog *ssa.Program, plan *UnitPlan, batch compileUnitExecutionBatch, concurrency int) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if concurrency < 1 {
		concurrency = 1
	}
	nodes := compileUnitBuildNodes(prog, plan, batch)
	ready := make([]int, 0, len(nodes))
	for index := range nodes {
		if nodes[index].remaining == 0 {
			ready = append(ready, index)
		}
	}
	results := make(chan unitBuildResult, len(nodes))
	slots := make(chan struct{}, concurrency)
	var activeLibraries, peakLibraries atomic.Int32
	activeGroups := 0
	exclusive := false
	completed := 0
	var runErr error
	for completed < len(nodes) {
		if runErr == nil {
			runErr = ctx.Err()
		}
		for runErr == nil && activeGroups < concurrency && len(ready) > 0 && !exclusive {
			index := ready[0]
			node := nodes[index]
			if node.exclusive && activeGroups != 0 {
				break
			}
			ready = ready[1:]
			activeGroups++
			exclusive = node.exclusive
			go func(index int, node unitBuildNode) {
				result := unitBuildResult{index: index}
				defer func() {
					if recovered := recover(); recovered != nil {
						result.err = fmt.Errorf("compile unit %v panic: %v\n%s", node.keys, recovered, debug.Stack())
					}
					results <- result
				}()
				result.err = runCompileUnitNode(ctx, node, slots, &activeLibraries, &peakLibraries)
			}(index, node)
		}
		if activeGroups == 0 {
			if runErr != nil {
				return int(peakLibraries.Load()), runErr
			}
			return int(peakLibraries.Load()), fmt.Errorf("compile unit dependency graph cannot progress: %d/%d SCCs complete", completed, len(nodes))
		}
		result := <-results
		activeGroups--
		if nodes[result.index].exclusive {
			exclusive = false
		}
		completed++
		if runErr == nil {
			runErr = result.err
		}
		for _, dependent := range nodes[result.index].dependents {
			nodes[dependent].remaining--
			if nodes[dependent].remaining == 0 {
				ready = append(ready, dependent)
			}
		}
	}
	return int(peakLibraries.Load()), runErr
}

func runCompileUnitNode(ctx context.Context, node unitBuildNode, slots chan struct{}, active, peak *atomic.Int32) error {
	if len(node.libraries) <= 1 || node.exclusive {
		for _, library := range node.libraries {
			if err := runCompileUnitLibrary(ctx, node.keys, library, slots, active, peak); err != nil {
				return err
			}
		}
		return ctx.Err()
	}
	var wg sync.WaitGroup
	errors := make(chan error, len(node.libraries))
	for _, library := range node.libraries {
		wg.Add(1)
		go func(library *ssa.Program) {
			defer wg.Done()
			if err := runCompileUnitLibrary(ctx, node.keys, library, slots, active, peak); err != nil {
				errors <- err
			}
		}(library)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		return err
	}
	return ctx.Err()
}

func runCompileUnitLibrary(ctx context.Context, keys []string, library *ssa.Program, slots chan struct{}, active, peak *atomic.Int32) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-slots }()
	current := active.Add(1)
	for previous := peak.Load(); current > previous; previous = peak.Load() {
		if peak.CompareAndSwap(previous, current) {
			break
		}
	}
	defer active.Add(-1)
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("compile unit %v panic: %v\n%s", keys, recovered, debug.Stack())
		}
	}()
	library.LazyBuildCurrentProgram()
	return ctx.Err()
}
