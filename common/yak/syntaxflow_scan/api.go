package syntaxflow_scan

import (
	"context"
)

// StartScan 启动新的SyntaxFlow扫描任务，使用options模式配置扫描参数
//
// 参数:
//   - ctx: 上下文，用于控制扫描任务的生命周期
//   - callback: 回调函数，用于处理扫描结果
//   - opts: 可变数量的选项函数，用于配置扫描参数
//
// 返回值:
//   - error: 如果启动失败则返回错误信息
//
// Example:
// ```
// // 基础扫描示例
//
//	err := syntaxflowscan.StartScan(context.New(), func(result) {
//	    println("任务ID:", result.TaskID)
//	    println("状态:", result.Status)
//	    if result.Risks && len(result.Risks) > 0 {
//	        for _, risk := range result.Risks {
//	            println("发现风险:", risk.Title)
//	        }
//	    }
//	    return nil
//	},
//
//	syntaxflowscan.withProgramNames("my-java-project"),
//	syntaxflowscan.withRuleNames("sql-injection", "xss"),
//	syntaxflowscan.withSeverity("high", "critical"),
//	syntaxflowscan.withConcurrency(10),
//
// )
// die(err)
//
// // 多程序扫描示例
//
//	err := syntaxflowscan.StartScan(context.New(), func(result) {
//	    yakit.Info("扫描进度: %s", result.Status)
//	    return nil
//	},
//
//	syntaxflowscan.withProgramNames("frontend", "backend", "api"),
//	syntaxflowscan.withLanguages("javascript", "java", "go"),
//	syntaxflowscan.withKeyword("security"),
//	syntaxflowscan.withMemory(true),
//
// )
// ```
func StartScan(ctx context.Context, opts ...ScanOption) error {
	opts = append(opts,
		WithControlMode(ControlModeStart),
		WithConcurrency(5),
	)
	return Scan(ctx, opts...)
}

// ResumeScan 恢复之前暂停的扫描任务
//
// 参数:
//   - ctx: 上下文，用于控制扫描任务的生命周期
//   - taskId: 要恢复的任务ID
//   - callback: 回调函数，用于处理扫描结果
//
// 返回值:
//   - error: 如果恢复失败则返回错误信息
//
// Example:
// ```
// // 恢复之前暂停的扫描任务
// taskId := "previous-task-12345"
//
//	err := syntaxflowscan.ResumeScan(context.New(), taskId, func(result) {
//	    println("恢复扫描 - 任务ID:", result.TaskID)
//	    println("当前状态:", result.Status)
//	    if result.Status == "done" {
//	        println("扫描已完成！")
//	    }
//	    return nil
//	})
//
// die(err)
// ```
func ResumeScan(ctx context.Context, taskId string, opts ...ScanOption) error {
	opts = append(opts,
		WithControlMode(ControlModeResume),
		WithResumeTaskId(taskId),
	)
	return Scan(ctx,
		opts...,
	)
}

// GetScanStatus 查询扫描任务的当前状态
//
// 参数:
//   - ctx: 上下文，用于控制查询的生命周期
//   - taskId: 要查询的任务ID
//   - callback: 回调函数，用于处理状态查询结果
//
// 返回值:
//   - error: 如果查询失败则返回错误信息
//
// Example:
// ```
// // 查询扫描任务状态
// taskId := "running-task-67890"
//
//	err := syntaxflowscan.GetScanStatus(context.New(), taskId, func(result) {
//	    println("任务状态查询:")
//	    println("  任务ID:", result.TaskID)
//	    println("  当前状态:", result.Status)
//	    if result.ExecResult {
//	        println("  执行信息:", result.ExecResult.Message)
//	    }
//	    return nil
//	})
//
// die(err)
// ```
func GetScanStatus(ctx context.Context, taskId string, callback ProcessCallback) error {
	return Scan(ctx,
		WithControlMode(ControlModeStatus),
		WithResumeTaskId(taskId),
		WithProcessCallback(callback),
	)
}
