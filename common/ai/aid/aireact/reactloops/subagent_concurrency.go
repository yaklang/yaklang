package reactloops

import (
	"sort"
	"sync"
)

// runJobsConcurrently 通过有界 worker 池并发运行一组子 Agent 任务。它直接操作
// 统一的 SubAgentResult 类型：每个元素内嵌 SubAgentJob 携带任务身份，runSingle
// 负责填入执行结果（SubLoop / ExecErr / Record / ...）。这样 dispatch / fork /
// nested 三条路径共用同一份 worker-pool 逻辑，不再需要 [Job, Result] 泛型对。
//
// 当 concurrency <= 1 时按提交顺序串行执行。结果按完成顺序返回；需要确定性
// 排序的调用方应通过 sortSubAgentResultsByOrder 按 Order 重新排序。
//
// runSingle 即使出错也必须返回非 nil 的结果（与本包中每个 per-job runner 的
// 契约一致）。
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

// sortSubAgentResultsByOrder 按 Order 升序原地排序结果。
func sortSubAgentResultsByOrder(results []*SubAgentResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Order < results[j].Order
	})
}
