package aicommon

import (
	"sort"
	"strings"
	"sync"
)

type FrozenBlockPartition struct {
	ID      string
	Title   string
	Nonce   string
	Content string
	Order   int
}

type FrozenBlockPartitionProducer struct {
	m sync.RWMutex

	partitions []FrozenBlockPartition
}

func NewFrozenBlockPartitionProducer(partitions ...FrozenBlockPartition) *FrozenBlockPartitionProducer {
	producer := &FrozenBlockPartitionProducer{}
	producer.AppendPartitions(partitions...)
	return producer
}

func (p *FrozenBlockPartitionProducer) AppendPartition(partition FrozenBlockPartition) {
	if p == nil {
		return
	}
	partitions := NormalizeFrozenBlockPartitions([]FrozenBlockPartition{partition})
	if len(partitions) == 0 {
		return
	}
	p.m.Lock()
	p.partitions = append(p.partitions, partitions[0])
	p.m.Unlock()
}

func (p *FrozenBlockPartitionProducer) AppendPartitions(partitions ...FrozenBlockPartition) {
	if p == nil || len(partitions) == 0 {
		return
	}
	partitions = NormalizeFrozenBlockPartitions(partitions)
	if len(partitions) == 0 {
		return
	}
	p.m.Lock()
	p.partitions = append(p.partitions, partitions...)
	p.m.Unlock()
}

func (p *FrozenBlockPartitionProducer) AppendNewPartition(id string, title string, content string, order int) {
	if p == nil {
		return
	}
	partition, ok := NewFrozenBlockPartition(id, title, content, order)
	if !ok {
		return
	}
	p.AppendPartition(partition)
}

func (p *FrozenBlockPartitionProducer) ProducePartitions() []FrozenBlockPartition {
	if p == nil {
		return nil
	}
	p.m.RLock()
	partitions := append([]FrozenBlockPartition(nil), p.partitions...)
	p.m.RUnlock()
	return NormalizeFrozenBlockPartitions(partitions)
}

func (c *Config) GetOrCreateFrozenBlockPartitionProducer() *FrozenBlockPartitionProducer {
	if c == nil {
		return nil
	}
	if c.m == nil {
		c.m = &sync.Mutex{}
	}
	c.m.Lock()
	defer c.m.Unlock()
	if c.FrozenBlockPartitionProducer == nil {
		c.FrozenBlockPartitionProducer = NewFrozenBlockPartitionProducer()
	}
	return c.FrozenBlockPartitionProducer
}

func (c *Config) AppendFrozenBlockPartition(id string, title string, content string, order int) {
	producer := c.GetOrCreateFrozenBlockPartitionProducer()
	if producer == nil {
		return
	}
	producer.AppendNewPartition(id, title, content, order)
}

func WithFrozenBlockPartitionProducer(producer *FrozenBlockPartitionProducer) ConfigOption {
	return func(c *Config) error {
		if c == nil || producer == nil {
			return nil
		}
		if c.m == nil {
			c.m = &sync.Mutex{}
		}
		c.m.Lock()
		c.FrozenBlockPartitionProducer = producer
		c.m.Unlock()
		return nil
	}
}

func WithFrozenBlockPartitions(partitions ...FrozenBlockPartition) ConfigOption {
	return func(c *Config) error {
		if c == nil || len(partitions) == 0 {
			return nil
		}
		producer := c.GetOrCreateFrozenBlockPartitionProducer()
		if producer == nil {
			return nil
		}
		producer.AppendPartitions(partitions...)
		return nil
	}
}

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

func FrozenBlockPartitionsFromConfig(config AICallerConfigIf) []FrozenBlockPartition {
	if config == nil {
		return nil
	}
	cfg, ok := config.(*Config)
	if !ok || cfg == nil {
		return nil
	}
	producer := cfg.FrozenBlockPartitionProducer
	if producer == nil {
		return nil
	}
	return producer.ProducePartitions()
}

const ( // stable partition orders for prefix cache , users can also define their own partitions with custom orders
	PersistentMemoryOrder = 90

	PlanFactsFrozenPartitionOrder    = 100
	PlanDocumentFrozenPartitionOrder = 110
)
