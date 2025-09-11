package aireact

func (r *ReAct) invokeBlueprint(forgeName string) error {
	manager := r.config.aiBlueprintManager
	_, _ = manager.GenerateAIForgeListForPrompt(nil)
	return nil
}
