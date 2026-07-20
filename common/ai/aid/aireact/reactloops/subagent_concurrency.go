package reactloops

import (
	"context"
	"sort"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// runJobsConcurrently 通过有界 worker 池并发运行一组子 Agent 任务。它直接操作
// 统一的 SubAgentResult 类型：每个元素内嵌 SubAgentJob 携带任务身份，runSingle
// 负责填入执行结果（SubLoop / ExecErr / Record / ...）。这样 dispatch / fork /
// nested 三条路径共用同一份 worker-pool 逻辑，不再需要 [Job, Result] 泛型对。
//
// 并发上限由 concurrency 决定（<= 1 时串行执行）。结果按完成顺序返回；需要确定性
// 排序的调用方应通过 sortSubAgentResultsByOrder 按 Order 重新排序。
//
// ctx 用于投递阶段快速 drain：swg 不绑定 ctx（避免 SetZero 提前清零计数导致
// Done 触发 negative WaitGroup counter），只在 AddWithContext 投递时传 ctx，
// ctx 取消时未投递的 job 直接记为 cancelled。已启动的 worker 由 runSingle
// 内部自行感知 ctx 尽快退出。
//
// 每个 worker 在 recover 中执行 runSingle，把 panic 转写成失败结果，避免单个
// 子 Agent 崩溃导致整批任务静默丢失。runSingle 契约：即使出错也必须返回非 nil
// 结果；违反时兜底成失败结果，防止下游 nil 解引用。
func runJobsConcurrently(
	ctx context.Context,
	jobs []*SubAgentResult,
	concurrency int,
	runSingle func(r *SubAgentResult) *SubAgentResult,
) []*SubAgentResult {
	if len(jobs) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// 串行路径：仍然走 runSingleWithRecover 以获得 recover 保护。
	if concurrency <= 1 {
		results := make([]*SubAgentResult, 0, len(jobs))
		for _, job := range jobs {
			results = append(results, runSingleWithRecover(job, runSingle))
		}
		return results
	}

	swg := utils.NewSizedWaitGroup(concurrency)
	var mu sync.Mutex
	results := make([]*SubAgentResult, 0, len(jobs))
	for _, job := range jobs {
		// 投递阶段响应取消：ctx 取消时不再占用并发槽位，直接记录 cancelled 结果。
		// 注意：AddWithContext 返回 err 时内部 wg 计数未增加，切勿调用 swg.Done()。
		if err := swg.AddWithContext(ctx, 1); err != nil {
			mu.Lock()
			results = append(results, failedJobResult(job, "cancelled", err))
			mu.Unlock()
			continue
		}
		job := job
		go func() {
			defer swg.Done()
			res := runSingleWithRecover(job, runSingle)
			mu.Lock()
			results = append(results, res)
			mu.Unlock()
		}()
	}
	swg.Wait()

	return results
}

// runSingleWithRecover 在 recover 中执行 runSingle，把 panic 转写成失败结果，
// 避免单个 job 崩溃静默吞掉整批任务。runSingle 返回 nil 时兜底成失败结果，
// 防止下游 nil 解引用。
func runSingleWithRecover(job *SubAgentResult, runSingle func(r *SubAgentResult) *SubAgentResult) (result *SubAgentResult) {
	if job == nil {
		return nil
	}
	defer func() {
		if rec := recover(); rec != nil {
			log.Errorf("subagent: runSingle panic for job %q (order=%d): %v", job.Identifier, job.Order, rec)
			result = failedJobResult(job, "panic", utils.Errorf("subagent panic: %v", rec))
			return
		}
		if result == nil {
			log.Errorf("subagent: runSingle returned nil result for job %q (order=%d)", job.Identifier, job.Order)
			result = failedJobResult(job, "panic", utils.Error("runSingle returned nil result"))
		}
	}()
	result = runSingle(job)
	return
}

// failedJobResult 构建一个失败的 SubAgentResult，统一 cancelled / panic / nil
// 兜底等失败场景。status 标识失败类别（"cancelled" / "panic" / ...），err 为
// 失败原因，同时写入 Record 与 Feedback 供上层展示。
func failedJobResult(job *SubAgentResult, status string, err error) *SubAgentResult {
	r := &SubAgentResult{SubAgentJob: job.SubAgentJob, ExecErr: err}
	r.Record = TimelineRecord{
		SubAgentID: job.Identifier,
		Order:      job.Order,
		LoopName:   job.LoopName,
		Goal:       job.Goal,
		Status:     status,
		Error:      err.Error(),
	}
	r.Feedback = err.Error()
	return r
}

// sortSubAgentResultsByOrder 按 Order 升序原地排序结果。
func sortSubAgentResultsByOrder(results []*SubAgentResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Order < results[j].Order
	})
}
