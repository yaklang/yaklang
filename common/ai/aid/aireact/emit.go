package aireact

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// EmitAction emits an action event using the embedded Emitter
func (r *ReAct) EmitAction(action string) {
	r.Emitter.EmitAction("action", action, "react")
}

// EmitIteration emits an iteration start event using the embedded Emitter
func (r *ReAct) EmitIteration(iteration int, maxIterations int) {
	description := fmt.Sprintf("ReAct iteration %d/%d started", iteration, maxIterations)
	r.Emitter.EmitIteration("iteration", iteration, maxIterations, description)
}

// EmitResult emits a final result event using the embedded Emitter
func (r *ReAct) EmitResult(result interface{}) {
	r.Emitter.EmitResult("result", result, true)
}

func (r *ReAct) EmitFileArtifactWithExt(identifier string, ext string, i any) string {
	var name string
	var suffix string
	if r.artifacts.Ext(identifier) != ext {
		suffix = ext
	}
	if !strings.HasSuffix(identifier, "_") {
		identifier = identifier + "_"
	}
	name = identifier + utils.DatetimePretty2() + suffix
	err := r.artifacts.WriteFile(name, utils.InterfaceToBytes(i), 0644)
	if err != nil {
		log.Errorf("Error writing file: %v", err)
		return ""
	}
	wd, err := r.artifacts.Getwd()
	if err != nil {
		log.Errorf("Error getting working directory: %v", err)
		return ""
	}
	filename := r.artifacts.Join(wd, name)
	r.Emitter.EmitPinFilename(filename)
	return filename
}

func (r *ReAct) EmitTextArtifact(identifier string, i any) {
	r.EmitFileArtifactWithExt(identifier, ".txt", i)
}

func (r *ReAct) EmitResultAfterStream(result interface{}) {
	r.Emitter.EmitResultAfterStream("result", result, false)
}

// EmitKnowledge emits a knowledge event using the embedded Emitter
func (r *ReAct) EmitKnowledge(enhanceID string, knowledge aicommon.EnhanceKnowledge) {
	r.knowledgeEmitCounter++
	r.Emitter.EmitKnowledge("knowledge", enhanceID, knowledge)
}

// EmitKnowledgeReferenceArtifact saves all knowledge items to a single artifact file
// Call this after all knowledge items have been collected
func (r *ReAct) EmitKnowledgeReferenceArtifact(knowledgeList []aicommon.EnhanceKnowledge, query string) string {
	if len(knowledgeList) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# 知识增强查询结果\n\n"))
	sb.WriteString(fmt.Sprintf("**查询内容**: %s\n", query))
	sb.WriteString(fmt.Sprintf("**结果数量**: %d\n\n", len(knowledgeList)))
	sb.WriteString("---\n\n")

	for i, k := range knowledgeList {
		sb.WriteString(fmt.Sprintf("## 知识条目 #%d\n\n", i+1))
		sb.WriteString(fmt.Sprintf("- **标题**: %s\n", k.GetTitle()))
		sb.WriteString(fmt.Sprintf("- **类型**: %s\n", k.GetType()))
		sb.WriteString(fmt.Sprintf("- **来源**: %s\n", k.GetSource()))
		sb.WriteString(fmt.Sprintf("- **相关度评分**: %.4f\n", k.GetScore()))
		sb.WriteString(fmt.Sprintf("- **评分方法**: %s\n\n", k.GetScoreMethod()))
		sb.WriteString("### 详细内容\n\n")
		sb.WriteString(k.GetContent())
		sb.WriteString("\n\n---\n\n")
	}

	// Save to artifacts directory using the existing method
	return r.EmitFileArtifactWithExt("knowledge_reference", ".md", sb.String())
}

// EmitKnowledgeListAboutTask emits a list of knowledge items related to a specific task using the embedded Emitter, for sync
func (r *ReAct) EmitKnowledgeListAboutTask(taskID string, knowledgeList []aicommon.EnhanceKnowledge, SyncId string) {
	r.Emitter.EmitKnowledgeListAboutTask("knowledge_list", taskID, knowledgeList, SyncId)
}
