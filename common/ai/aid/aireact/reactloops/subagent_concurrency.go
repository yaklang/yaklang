package reactloops

import (
	"sort"
	"sync"
)

// runJobsConcurrently runs a set of sub-agent jobs through a bounded worker
// pool. It operates on the single unified SubAgentResult type: each element
// carries the originating SubAgentJob (via the embedded SubAgentJob) and
// runSingle fills in the execution outcome (SubLoop / ExecErr / Record / ...).
// This keeps the worker-pool logic in one place for the dispatch / fork /
// nested paths — there is no longer a [Job, Result] generic pair.
//
// When concurrency <= 1 the jobs run sequentially in submission order. Results
// are returned in completion order; callers that need deterministic ordering
// should re-sort by Order via sortSubAgentResultsByOrder.
//
// runSingle must return a non-nil result even on error (mirrors the contract of
// every existing per-job runner in this package).
func runJobsConcurrently(
	jobs []*SubAgentResult,
	concurrency int,
	runSingle func(r *SubAgentResult) *SubAgentResult,
) []*SubAgentResult {
	if len(jobs) == 0 {
		return nil
	}
	if concurrency <= 1 {
		results := make([]*SubAgentResult, 0, len(jobs))
		for _, job := range jobs {
			results = append(results, runSingle(job))
		}
		return results
	}

	jobsCh := make(chan *SubAgentResult)
	resultsCh := make(chan *SubAgentResult, len(jobs))
	var workers sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobsCh {
				resultsCh <- runSingle(job)
			}
		}()
	}
	for _, job := range jobs {
		jobsCh <- job
	}
	close(jobsCh)
	workers.Wait()
	close(resultsCh)

	results := make([]*SubAgentResult, 0, len(jobs))
	for result := range resultsCh {
		results = append(results, result)
	}
	return results
}

// sortSubAgentResultsByOrder sorts results in place by Order ascending.
func sortSubAgentResultsByOrder(results []*SubAgentResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Order < results[j].Order
	})
}
