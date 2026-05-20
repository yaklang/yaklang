package aicommon

import (
	"sort"
	"strings"
)

type FrozenBlockPartition struct {
	ID      string
	Title   string
	Nonce   string
	Content string
	Order   int
}

type FrozenBlockPartitionProducer func() []FrozenBlockPartition

const frozenBlockPartitionProducersConfigKey = "frozen_block_partition_producers"

func NewFrozenBlockPartition(id string, title string, content string, order int) (FrozenBlockPartition, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return FrozenBlockPartition{}, false
	}
	id = NormalizeFrozenPartitionID(id)
	title = strings.TrimSpace(title)
	if title == "" {
		title = id
	}
	nonce := StablePromptNonce("frozen-partition", id, content)
	return FrozenBlockPartition{
		ID:      id,
		Title:   title,
		Nonce:   nonce,
		Content: content,
		Order:   order,
	}, true
}

func NormalizeFrozenPartitionID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range id {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if r == '_' || r == '-' || r == '.' || r == '/' || r == ' ' || r == '\t' {
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	normalized := strings.Trim(b.String(), "_")
	if normalized == "" {
		return "partition"
	}
	return normalized
}

func NormalizeFrozenBlockPartitions(partitions []FrozenBlockPartition) []FrozenBlockPartition {
	if len(partitions) == 0 {
		return nil
	}
	byID := make(map[string]FrozenBlockPartition, len(partitions))
	for _, partition := range partitions {
		content := strings.TrimSpace(partition.Content)
		if content == "" {
			continue
		}
		id := NormalizeFrozenPartitionID(partition.ID)
		title := strings.TrimSpace(partition.Title)
		if title == "" {
			title = id
		}
		nonce := strings.TrimSpace(partition.Nonce)
		if nonce == "" {
			nonce = StablePromptNonce("frozen-partition", id, content)
		}
		partition.ID = id
		partition.Title = title
		partition.Nonce = nonce
		partition.Content = content
		byID[id] = partition
	}
	if len(byID) == 0 {
		return nil
	}
	out := make([]FrozenBlockPartition, 0, len(byID))
	for _, partition := range byID {
		out = append(out, partition)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func WithFrozenBlockPartitionProducer(producer FrozenBlockPartitionProducer) ConfigOption {
	return func(c *Config) error {
		if c == nil || producer == nil {
			return nil
		}
		if c.KeyValueConfig == nil {
			c.KeyValueConfig = NewKeyValueConfig()
		}
		var producers []FrozenBlockPartitionProducer
		if existing, ok := c.GetConfig(frozenBlockPartitionProducersConfigKey); ok {
			switch typed := existing.(type) {
			case []FrozenBlockPartitionProducer:
				producers = append(producers, typed...)
			case FrozenBlockPartitionProducer:
				producers = append(producers, typed)
			}
		}
		producers = append(producers, producer)
		c.SetConfig(frozenBlockPartitionProducersConfigKey, producers)
		return nil
	}
}

func FrozenBlockPartitionsFromConfig(config AICallerConfigIf) []FrozenBlockPartition {
	if config == nil {
		return nil
	}
	if cfg, ok := config.(*Config); ok && cfg.KeyValueConfig == nil {
		return nil
	}
	existing, ok := config.GetConfig(frozenBlockPartitionProducersConfigKey)
	if !ok {
		return nil
	}

	var producers []FrozenBlockPartitionProducer
	switch typed := existing.(type) {
	case []FrozenBlockPartitionProducer:
		producers = typed
	case FrozenBlockPartitionProducer:
		producers = []FrozenBlockPartitionProducer{typed}
	default:
		return nil
	}

	var partitions []FrozenBlockPartition
	for _, producer := range producers {
		if producer == nil {
			continue
		}
		partitions = append(partitions, producer()...)
	}
	return NormalizeFrozenBlockPartitions(partitions)
}
