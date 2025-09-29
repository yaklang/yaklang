package aid

func (c *Config) SimpleInfoMap() map[string]interface{} {
	return map[string]interface{}{
		"ID":                          c.id,
		"AllowPlanUserInteract":       c.allowPlanUserInteract,
		"PlanUserInteractMaxCount":    c.planUserInteractMaxCount,
		"PersistentMemory":            c.persistentMemory,
		"TimelineRecordLimit":         0,
		"TimelineContentSizeLimit":    c.timelineContentSizeLimit,
		"TimelineTotalContentLimit":   c.timelineTotalContentLimit,
		"Keywords":                    c.keywords,
		"DebugPrompt":                 c.debugPrompt,
		"DebugEvent":                  c.debugEvent,
		"AllowRequireForUserInteract": c.allowRequireForUserInteract,
		"AgreePolicy":                 c.agreePolicy,
		"AgreeInterval":               c.agreeInterval,
		"AgreeAIScore":                c.agreeAIScore,
		"InputConsumption":            c.inputConsumption,
		"OutputConsumption":           c.outputConsumption,
		"AICallTokenLimit":            c.aiCallTokenLimit,
		"AIAutoRetry":                 c.aiAutoRetry,
		"AIAutoTransactionRetry":      c.aiTransactionAutoRetry,
		"GenerateReport":              c.generateReport,
		"ForgeName":                   c.forgeName,
	}
}
