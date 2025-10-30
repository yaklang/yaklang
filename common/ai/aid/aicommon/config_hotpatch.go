package aicommon

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
)

func (c *Config) StartHotPatchLoop(ctx context.Context) {
	c.StartHotPatchOnce.Do(func() {
		if c.HotPatchOptionChan == nil {
			return
		}
		go func() {
			for {
				select {
				case <-ctx.Done():
				case hotPatchOption := <-c.HotPatchOptionChan.OutputChannel():
					if hotPatchOption == nil {
						log.Errorf("hotpatch option is nil, will return")
						return
					}
					err := hotPatchOption(c)
					if err != nil {
						log.Errorf("hotpatch option err: %v", err)
					}
					c.EmitCurrentConfigInfo()
				}
			}
		}()
	})
}


func (c *Config) SimpleInfoMap() map[string]interface{} {
	return map[string]interface{}{
		"ID":                          c.Id,
		"AllowPlanUserInteract":       c.AllowPlanUserInteract,
		"PlanUserInteractMaxCount":    c.PlanUserInteractMaxCount,
		"PersistentMemory":            c.PersistentMemory,
		"TimelineRecordLimit":         0,
		"TimelineContentSizeLimit":    c.TimelineContentSizeLimit,
		"TimelineTotalContentLimit":   c.TimelineTotalContentLimit,
		"Keywords":                    c.Keywords,
		"DebugPrompt":                 c.DebugPrompt,
		"DebugEvent":                  c.DebugEvent,
		"AllowRequireForUserInteract": c.AllowRequireForUserInteract,
		"AgreePolicy":                 c.AgreePolicy,
		"AgreeInterval":               c.AgreeInterval,
		"AgreeAIScoreLow":             c.AgreeAIScoreLow,
		"AgreeAIScoreMiddle":          c.AgreeAIScoreMiddle,
		"InputConsumption":            c.InputConsumption,
		"OutputConsumption":           c.OutputConsumption,
		"AICallTokenLimit":            c.AiCallTokenLimit,
		"AIAutoRetry":                 c.AiAutoRetry,
		"AIAutoTransactionRetry":      c.AiTransactionAutoRetry,
		"GenerateReport":              c.GenerateReport,
		"ForgeName":                   c.ForgeName,
	}
}