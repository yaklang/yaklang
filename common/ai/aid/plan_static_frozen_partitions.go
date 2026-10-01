package aid

import "github.com/yaklang/yaklang/common/ai/aid/aicommon"

func appendPlanDocumentFrozenPartition(config *aicommon.Config, document string) {
	if config == nil {
		return
	}
	config.AppendFrozenBlockPartition("plan_document", "Plan Document", document, aicommon.PlanDocumentFrozenPartitionOrder)
}

func BuildPlanStaticFrozenPartitions(producer *aicommon.FrozenBlockPartitionProducer) []aicommon.FrozenBlockPartition {
	if producer == nil {
		return nil
	}
	var out []aicommon.FrozenBlockPartition
	for _, partition := range producer.ProducePartitions() {
		switch partition.ID {
		case "plan_document":
			out = append(out, partition)
		}
	}
	return out
}
