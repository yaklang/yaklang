package loop_risk_enrich

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const environmentKey = "risk_enrich_environment"

// riskEnvironment is fixed by InitTask, never by a model-generated action.
type riskEnvironment struct {
	RiskID     int64
	RuntimeIDs []string
	Scope      targetScope
}

func environment(loop *reactloops.ReActLoop) (*riskEnvironment, error) {
	if loop != nil {
		if env, ok := loop.GetVariable(environmentKey).(*riskEnvironment); ok && env != nil && env.RiskID > 0 && len(env.RuntimeIDs) > 0 {
			return env, nil
		}
	}
	return nil, fmt.Errorf("risk environment was not initialized from an attached risk_id resource")
}

func attachedRiskID(resources []aicommon.AttachedResourceData) (int64, error) {
	var id int64
	for _, resource := range resources {
		risk, ok := resource.(*aicommon.AttachedRiskResourceData)
		if !ok {
			continue
		}
		if id != 0 && id != risk.ID {
			return 0, fmt.Errorf("attach exactly one risk_id; multiple different risks are not supported")
		}
		id = risk.ID
	}
	if id <= 0 {
		return 0, fmt.Errorf("risk enrichment requires an attached resource of type risk_id (existing risk ID)")
	}
	return id, nil
}

func buildInitTask(invoker aicommon.AIInvokeRuntime) func(*reactloops.ReActLoop, aicommon.AIStatefulTask, *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		if err := bootstrapRiskEnvironment(loop, invoker, task); err != nil {
			operator.Failed(err)
			return
		}
		operator.Continue()
	}
}

func bootstrapRiskEnvironment(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask) error {
	if loop == nil || invoker == nil || task == nil {
		return fmt.Errorf("risk enrichment requires an active task and invoker")
	}
	// Never reuse the previous task's risk if this task has no valid attachment.
	loop.Set(environmentKey, nil)
	loop.Set("risk_enrich_context", "")
	loop.Set("risk_enrich_inventory", "")
	loop.Set("risk_enrich_assessments", map[int64]riskAssessment{})
	loop.Set("risk_enrich_searches", []string{})
	for _, attachment := range task.GetAttachedDatas() {
		if attachment != nil && attachment.HasType(aicommon.AttachedResourceTypeRiskID, "risk") {
			if _, err := aicommon.ParseAttachedResourceData(attachment); err != nil {
				return fmt.Errorf("invalid attached risk: %w", err)
			}
		}
	}
	resources := reactloops.RunAttachedExtraResourcesInit(invoker, loop, task.GetAttachedDatas())
	id, err := attachedRiskID(resources)
	if err != nil {
		return err
	}
	db := projectDB(invoker)
	if db == nil {
		return fmt.Errorf("project database unavailable for attached risk")
	}
	risk, err := yakit.GetRisk(db, id)
	if err != nil {
		return fmt.Errorf("attached risk %d not found: %w", id, err)
	}
	ids, err := runtimeIDsForRisk(db, risk)
	if err != nil {
		return fmt.Errorf("resolve attached risk runtime boundary: %w", err)
	}
	if len(ids) == 0 {
		return fmt.Errorf("attached risk %d has neither a runtime nor a resolvable AI session runtime boundary", id)
	}
	env := &riskEnvironment{RiskID: id, RuntimeIDs: append([]string(nil), ids...), Scope: scopeForRisk(risk)}
	// The initial inventory is bounded, but the total counts are exact. This
	// gives the model a landscape before choosing its first focused search.
	inventory, err := surveyEnvironment(db, env)
	if err != nil {
		return fmt.Errorf("survey attached risk evidence: %w", err)
	}
	loop.Set(environmentKey, env)
	loop.Set("risk_enrich_context", riskContext(risk))
	loop.Set("risk_enrich_inventory", inventory)
	invoker.AddToTimeline("risk_enrich_bootstrap", fmt.Sprintf("Risk #%d bound to %d related runtimes. %s", id, len(ids), strings.ReplaceAll(inventory, "\n", " ")))
	return nil
}

// loadBoundRisk only takes the ID from the initialized environment. There is
// deliberately no risk_id action parameter that could switch the subject.
func loadBoundRisk(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime) (*riskEnvironment, *schema.Risk, error) {
	env, err := environment(loop)
	if err != nil {
		return nil, nil, err
	}
	db := projectDB(invoker)
	if db == nil {
		return nil, nil, fmt.Errorf("project database unavailable")
	}
	risk, err := yakit.GetRisk(db, env.RiskID)
	if err != nil {
		return nil, nil, err
	}
	return env, risk, nil
}
