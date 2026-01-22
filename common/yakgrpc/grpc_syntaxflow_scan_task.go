//go:build !irify_exclude

package yakgrpc

import (
	"context"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// findAllBasePrograms 递归查找所有相关的 base 程序
// 返回所有相关的程序名列表（包括原始程序、所有中间 base、最基础的 base）
func findAllBasePrograms(progName string) ([]string, error) {
	if progName == "" {
		return nil, nil
	}

	var programs []string
	visited := make(map[string]bool) // 防止循环引用

	currentProgName := progName
	for {
		if visited[currentProgName] {
			break // 防止循环引用
		}
		visited[currentProgName] = true

		prog, err := ssadb.GetApplicationProgram(currentProgName)
		if err != nil {
			// 如果获取失败，至少返回已收集的程序
			if len(programs) > 0 {
				return programs, nil
			}
			return nil, err
		}

		// 添加到列表中
		programs = append(programs, currentProgName)

		// 如果这个程序有 BaseProgramName，说明它是增量编译的，继续向上查找
		if prog.BaseProgramName != "" && prog.BaseProgramName != currentProgName {
			currentProgName = prog.BaseProgramName
		} else {
			// 如果没有 BaseProgramName，说明这就是最基础的 base 程序
			break
		}
	}

	return programs, nil
}

func (s *Server) QuerySyntaxFlowScanTask(ctx context.Context, request *ypb.QuerySyntaxFlowScanTaskRequest) (*ypb.QuerySyntaxFlowScanTaskResponse, error) {
	if request.Pagination == nil {
		request.Pagination = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}

	var originalProgName string
	if filter := request.GetFilter(); filter != nil {
		if progNames := filter.GetPrograms(); len(progNames) == 1 && len(filter.GetProjectIds()) == 0 {
			progName := progNames[0]
			originalProgName = progName
			allPrograms, err := findAllBasePrograms(progName)
			if err != nil {
				return nil, err
			}
			// 合并并去重
			request.Filter.Programs = lo.Uniq(append(request.Filter.Programs, allPrograms...))
		} else if progNames := filter.GetPrograms(); len(progNames) == 1 {
			originalProgName = progNames[0]
		}
	}

	p, tasks, err := yakit.QuerySyntaxFlowScanTask(ssadb.GetDB(), request)
	if err != nil {
		return nil, err
	}

	datas := lo.Map(tasks, func(task *schema.SyntaxFlowScanTask, index int) *ypb.SyntaxFlowScanTask {
		data := task.ToGRPCModel()
		return data
	})

	// 使用原始程序名进行 diff 计算
	if originalProgName != "" && request.ShowDiffRisk {
		var lastTask *ypb.SyntaxFlowScanTask
		for i, task := range datas {
			if i == 0 {
				lastTask = task
				continue
			}
			baseline := &ypb.SSARiskDiffItem{
				ProgramName:   originalProgName,
				RiskRuntimeId: lastTask.TaskId,
			}
			compare := &ypb.SSARiskDiffItem{
				ProgramName:   originalProgName,
				RiskRuntimeId: task.TaskId,
			}
			res, err := yakit.DoRiskDiff(ctx, baseline, compare)
			if err != nil {
				return nil, err
			}
			for re := range res {
				if re.Status == yakit.Add {
					lastTask.NewRiskCount++
					switch schema.ValidSeverityType(re.NewValue.Severity) {
					case schema.SFR_SEVERITY_INFO:
						lastTask.NewInfoCount++
					case schema.SFR_SEVERITY_WARNING:
						lastTask.NewWarningCount++
					case schema.SFR_SEVERITY_CRITICAL:
						lastTask.NewCriticalCount++
					case schema.SFR_SEVERITY_HIGH:
						lastTask.NewHighCount++
					case schema.SFR_SEVERITY_LOW:
						lastTask.NewLowCount++
					}
				}
			}
			lastTask = task
		}
	}

	return &ypb.QuerySyntaxFlowScanTaskResponse{
		Pagination: &ypb.Paging{
			Page:     int64(p.Page),
			Limit:    int64(p.Limit),
			OrderBy:  request.Pagination.OrderBy,
			Order:    request.Pagination.Order,
			RawOrder: request.Pagination.RawOrder,
		},
		Data:  datas,
		Total: int64(p.TotalRecord),
	}, nil
}

func (s *Server) DeleteSyntaxFlowScanTask(ctx context.Context, request *ypb.DeleteSyntaxFlowScanTaskRequest) (*ypb.DbOperateMessage, error) {
	dbMsg := &ypb.DbOperateMessage{
		TableName: "syntax_flow_scan_task",
		Operation: DbOperationDelete,
	}
	if request.GetDeleteAll() {
		deleted, err := yakit.DeleteAllSyntaxFlowScanTask(ssadb.GetDB())
		if err != nil {
			return nil, err
		}
		dbMsg.EffectRows += deleted
		return dbMsg, nil
	}
	if request.GetFilter() != nil {
		deleted, err := yakit.DeleteSyntaxFlowScanTask(ssadb.GetDB(), request)
		if err != nil {
			return nil, err
		}
		dbMsg.EffectRows += deleted
		return dbMsg, nil
	}
	return dbMsg, nil
}
