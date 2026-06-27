package loop_ssa_api_discovery

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// 环境变量 YAK_SSA_API_DISCOVERY_PHASE1_REACT_TIMEOUT：单次 Phase1 ReAct 子循环
//（tech_arch / business_function / auth_react 等）最长墙钟，默认 90m。
// 使用 context.WithoutCancel 避免父 gRPC stream 先结束导致 ReAct 迭代 context canceled。

func phase1ReactMaxDuration() time.Duration {
	const def = 90 * time.Minute
	s := strings.TrimSpace(os.Getenv("YAK_SSA_API_DISCOVERY_PHASE1_REACT_TIMEOUT"))
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	if d < 5*time.Minute {
		return 5 * time.Minute
	}
	return d
}

func detachPhase1ReactContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(parent), phase1ReactMaxDuration())
}

// runPhase1ReActLoop executes a Phase1 ReAct sub-loop on a sub-task whose context is
// detached from the parent gRPC stream but bounded by phase1ReactMaxDuration().
func runPhase1ReActLoop(parent aicommon.AIStatefulTask, subName string, loop *reactloops.ReActLoop) error {
	if parent == nil {
		return utils.Error("nil parent task")
	}
	if loop == nil {
		return utils.Error("nil react loop")
	}
	detached, cancel := detachPhase1ReactContext(parent.GetContext())
	defer cancel()

	subID := fmt.Sprintf("%s-%s", parent.GetId(), subName)
	sub := aicommon.NewStatefulTaskBase(subID, parent.GetUserInput(), detached, parent.GetEmitter(), true)
	log.Infof("ssa_api_discovery: phase1 react %s detached timeout=%s", subName, phase1ReactMaxDuration())
	return loop.ExecuteWithExistedTask(sub)
}
