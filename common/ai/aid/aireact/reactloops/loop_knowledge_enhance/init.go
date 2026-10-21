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
		var loaded []string
		for _, result := range loop.LoadToolSchemas(task.GetContext(), names) {
			if result["status"] == "error" {
				op.Failed(fmt.Errorf("load knowledge tool %s: %s", result["tool_name"], result["detail"]))
				return
			}
			if result["status"] == "schema_loaded" {
				loaded = append(loaded, result["tool_name"])
			}
		}
		r.AddToTimeline("knowledge_focus", fmt.Sprintf(`知识增强初始化已完成：yaklang-system Skill 和以下工具的完整参数 Schema 已加载并可见：%v。直接按可见 Schema 调用工具，不要重复 loading_skills 或 require_tool；仅在定义已被普通缓存淘汰、上下文确实缺失时重新加载。
附加资源中的库名必须原样使用完整值，不得截取名称片段或自行替换。按用户附加资源和选库偏好限定范围，不得静默扩大为 all=true。
用户未指定查询预算时，优先使用 token_limit=1024、limit=3、per_entry_token_limit=256，再按实际结果分页或续读。加载定义尚未执行查询，只读任务不执行写入。`, loaded))
		loop_default.InitTask(r)(loop, task, op)
	}
}
