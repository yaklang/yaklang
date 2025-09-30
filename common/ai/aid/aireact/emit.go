package aireact

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// EmitThought emits a thought event using the embedded Emitter
func (r *ReAct) EmitThought(thought string) {
	r.Emitter.EmitThought("thought", thought)
}

// EmitAction emits an action event using the embedded Emitter
func (r *ReAct) EmitAction(action string) {
	r.Emitter.EmitAction("action", action, "react")
}

// EmitObservation emits an observation event using the embedded Emitter
func (r *ReAct) EmitObservation(observation string) {
	r.Emitter.EmitObservation("observation", observation, "react")
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

func (r *ReAct) EmitYaklangCodeArtifact(identifier string, i any) string {
	return r.EmitFileArtifactWithExt(identifier, ".yak", i)
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
	r.Emitter.EmitKnowledge("knowledge", enhanceID, knowledge)
}

// EmitKnowledgeListAboutTask emits a list of knowledge items related to a specific task using the embedded Emitter, for sync
func (r *ReAct) EmitKnowledgeListAboutTask(taskID string, knowledgeList []aicommon.EnhanceKnowledge) {
	r.Emitter.EmitKnowledgeListAboutTask("knowledge_list", taskID, knowledgeList)
}
