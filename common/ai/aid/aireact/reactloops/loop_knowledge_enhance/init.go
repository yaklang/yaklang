package loop_knowledge_enhance

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_default"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

func init() {
	err := reactloops.RegisterLoopFactory(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			var loadErr error
			prepareSkill := func(loop *reactloops.ReActLoop) {
				var loader aiskillloader.SkillLoader
				if cfg, ok := r.GetConfig().(*aicommon.Config); ok {
					loader = cfg.GetSkillLoader()
				}
				if mgr := loop.GetSkillsContextManager(); mgr != nil {
					loader = mgr.GetLoader()
				}
				if loader != nil {
					for _, meta := range loader.AllSkillMetas() {
						if meta != nil && meta.Name == "yaklang-system" {
							return // Preserve normal startup loading and the caller's manager options.
						}
					}
				}
				provider, ok := r.(interface {
					GetKnowledgeSkillLoader(aiskillloader.SkillLoader) (aiskillloader.SkillLoader, error)
				})
				if !ok {
					loadErr = fmt.Errorf("knowledge_enhance requires the system Skill loader")
					return
				}
				loader, loadErr = provider.GetKnowledgeSkillLoader(loader)
				if loadErr == nil {
					reactloops.WithSkillLoader(loader)(loop)
				}
			}
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowAIForge(reactloops.IsPlanAndExecAllowed(nil, r)),
				reactloops.WithAllowPlanAndExec(reactloops.IsPlanAndExecAllowed(nil, r)),
			}
			preset = append(preset, opts...)
			preset = append(preset, prepareSkill, reactloops.WithAllowRAG(false), reactloops.WithInitTask(initKnowledgeTask(r)))
			loop, err := loop_default.NewLoop(schema.AI_REACT_LOOP_NAME_KNOWLEDGE_ENHANCE, r, preset...)
			if loadErr != nil {
				if loop != nil {
					loop.Release()
				}
				return nil, loadErr
			}
			return loop, err
		},
		reactloops.WithLoopDescription("General-purpose assistant with the system Skill and enabled knowledge tools automatically loaded into the ordinary context cache."),
		reactloops.WithLoopDescriptionZh("知识增强检索：基于通用助手，首次自动加载系统 Skill 和知识库工具，遵循普通上下文缓存规则。"),
		reactloops.WithLoopUsagePrompt("Answer with knowledge bases using the preloaded Skill and tools. Respect attached preferences and explicit query scope and limits. Reload definitions normally if evicted."),
		reactloops.WithVerboseName("Knowledge Enhance"),
		reactloops.WithVerboseNameZh("知识增强检索"),
	)
	if err != nil {
		log.Errorf("register knowledge_enhance loop: %v", err)
	}
}

func initKnowledgeTask(r aicommon.AIInvokeRuntime) func(*reactloops.ReActLoop, aicommon.AIStatefulTask, *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
		// These are ordinary Timeline items, with the same promotion, compression
		// and eviction rules as loading_skills and require_tool.
		if _, err := loop.GetSkillsContextManager().LoadForcedSkill("yaklang-system"); err != nil {
			op.Failed(err)
			return
		}
		var names []string
		if manager := loop.GetConfig().GetAiToolManager(); manager != nil {
			enabled, err := manager.GetEnableTools()
			if err != nil {
				op.Failed(err)
				return
			}
			for _, tool := range enabled {
				if tool != nil && (tool.Name == "manage_knowledge_base" || tool.Name == "query_knowledge_base") {
					names = append(names, tool.Name)
				}
			}
		}
		for _, result := range loop.LoadToolSchemas(task.GetContext(), names) {
			if result["status"] == "error" {
				op.Failed(fmt.Errorf("load knowledge tool %s: %s", result["tool_name"], result["detail"]))
				return
			}
		}
		r.AddToTimeline("knowledge_focus", "The system Skill and enabled knowledge tool definitions were automatically loaded using the ordinary context cache. Reuse visible definitions; reload evicted definitions with loading_skills or require_tool. Loading definitions does not execute a query. Preserve attached knowledge base preferences.")
		loop_default.InitTask(r)(loop, task, op)
	}
}
