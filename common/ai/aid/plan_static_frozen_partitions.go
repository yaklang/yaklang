package aid

import "github.com/yaklang/yaklang/common/ai/aid/aicommon"

const (
	planFactsFrozenPartitionOrder    = 100
	planDocumentFrozenPartitionOrder = 110
)

func BuildPlanStaticFrozenPartitions(c *Coordinator) []aicommon.FrozenBlockPartition {
	if c == nil || c.ContextProvider == nil {
		return nil
	}
	var out []aicommon.FrozenBlockPartition
	if p, ok := aicommon.NewFrozenBlockPartition("plan_facts", "Plan Facts", getCoordinatorPlanPersistentData(c, planFactsPersistentKey), planFactsFrozenPartitionOrder); ok {
		out = append(out, p)
	}
	if p, ok := aicommon.NewFrozenBlockPartition("plan_document", "Plan Document", getCoordinatorPlanPersistentData(c, planDocumentPersistentKey), planDocumentFrozenPartitionOrder); ok {
		out = append(out, p)
	}
	return out
}

func getCoordinatorPlanPersistentData(c *Coordinator, key string) string {
	if c == nil || c.ContextProvider == nil {
		return ""
	}
	content, ok := c.ContextProvider.GetPersistentData(key)
	if !ok {
		return ""
	}
	return content
}
