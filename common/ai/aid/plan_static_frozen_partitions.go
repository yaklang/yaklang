package aid

import "github.com/yaklang/yaklang/common/ai/aid/aicommon"

func appendPlanFactsFrozenPartition(config *aicommon.Config, facts string) {
	if config == nil {
		return
	}
	config.AppendFrozenBlockPartition("plan_facts", "Plan Facts", facts, aicommon.PlanFactsFrozenPartitionOrder)
}

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
		case "plan_facts", "plan_document":
			out = append(out, partition)
		}
	}
	return out
}

// snapshotPlanFrozenPartitions persists only the two fixed plan reference kinds,
// never tool/schema/skill partitions or a cached prompt containing control tags.
func snapshotPlanFrozenPartitions(partitions []aicommon.FrozenBlockPartition) []aicommon.FrozenBlockPartition {
	var out []aicommon.FrozenBlockPartition
	for _, partition := range aicommon.NormalizeFrozenBlockPartitions(partitions) {
		if partition.ID == "plan_facts" || partition.ID == "plan_document" {
			// Archive metadata is framework-owned. A saved title/nonce must not
			// smuggle delimiter syntax outside the protected Content reference.
			partition.Nonce = ""
			if partition.ID == "plan_facts" {
				partition.Title, partition.Order = "Plan Facts", aicommon.PlanFactsFrozenPartitionOrder
			} else {
				partition.Title, partition.Order = "Plan Document", aicommon.PlanDocumentFrozenPartitionOrder
			}
			out = append(out, partition)
		}
	}
	return aicommon.NormalizeFrozenBlockPartitions(out)
}

func (t *AiTask) planPromptFrozenPartitions() []aicommon.FrozenBlockPartition {
	partitions := append([]aicommon.FrozenBlockPartition(nil), t.planFrozenPartitions...)
	if t.Coordinator != nil && t.Coordinator.Config != nil {
		partitions = append(partitions, aicommon.FrozenBlockPartitionsFromConfig(t.Coordinator.Config)...)
	}
	return snapshotPlanFrozenPartitions(partitions)
}
