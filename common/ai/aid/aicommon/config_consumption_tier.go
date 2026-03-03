package aicommon

import (
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type ConsumptionStats struct {
	InputConsumption  int64 `json:"input_consumption"`
	OutputConsumption int64 `json:"output_consumption"`
}

func newConsumptionStats() *ConsumptionStats {
	return &ConsumptionStats{}
}

func (s *ConsumptionStats) Add(inputDelta, outputDelta int64) {
	if s == nil {
		return
	}
	if inputDelta != 0 {
		atomic.AddInt64(&s.InputConsumption, inputDelta)
	}
	if outputDelta != 0 {
		atomic.AddInt64(&s.OutputConsumption, outputDelta)
	}
}

func (s *ConsumptionStats) Snapshot() map[string]int64 {
	if s == nil {
		return map[string]int64{
			"input_consumption":  0,
			"output_consumption": 0,
		}
	}
	return map[string]int64{
		"input_consumption":  atomic.LoadInt64(&s.InputConsumption),
		"output_consumption": atomic.LoadInt64(&s.OutputConsumption),
	}
}

type ConfigConsumptionState struct {
	InputConsumption    *int64
	OutputConsumption   *int64
	ConsumptionUUID     string
	TierConsumptionStat *omap.OrderedMap[consts.ModelTier, *ConsumptionStats]
	m                   *sync.Mutex
}

func NewConfigConsumptionState() *ConfigConsumptionState {
	return (&ConfigConsumptionState{
		InputConsumption:    new(int64),
		OutputConsumption:   new(int64),
		TierConsumptionStat: omap.NewOrderedMap(map[consts.ModelTier]*ConsumptionStats{}),
		m:                   &sync.Mutex{},
	}).ensure()
}

func (s *ConfigConsumptionState) ensure() *ConfigConsumptionState {
	if s == nil {
		return nil
	}
	if s.m == nil {
		s.m = &sync.Mutex{}
	}
	if s.InputConsumption == nil {
		s.InputConsumption = new(int64)
	}
	if s.OutputConsumption == nil {
		s.OutputConsumption = new(int64)
	}
	if s.TierConsumptionStat == nil {
		s.TierConsumptionStat = omap.NewOrderedMap(map[consts.ModelTier]*ConsumptionStats{})
	}
	return s
}

func (s *ConfigConsumptionState) SetConsumptionPointers(input, output *int64) {
	if s == nil {
		return
	}
	s.ensure()
	if input == nil {
		input = new(int64)
	}
	if output == nil {
		output = new(int64)
	}
	s.m.Lock()
	s.InputConsumption = input
	s.OutputConsumption = output
	s.m.Unlock()
}

func (s *ConfigConsumptionState) GetConsumptionPointers() (*int64, *int64) {
	if s == nil {
		return nil, nil
	}
	s.ensure()
	s.m.Lock()
	defer s.m.Unlock()
	return s.InputConsumption, s.OutputConsumption
}

func (s *ConfigConsumptionState) SetConsumptionUUID(uuid string) {
	if s == nil {
		return
	}
	s.ensure()
	s.m.Lock()
	s.ConsumptionUUID = uuid
	s.m.Unlock()
}

func (s *ConfigConsumptionState) GetConsumptionUUID() string {
	if s == nil {
		return ""
	}
	s.ensure()
	s.m.Lock()
	defer s.m.Unlock()
	return s.ConsumptionUUID
}

func (s *ConfigConsumptionState) SetTierConsumptionStats(stats *omap.OrderedMap[consts.ModelTier, *ConsumptionStats]) {
	if s == nil || stats == nil {
		return
	}
	s.ensure()
	s.m.Lock()
	s.TierConsumptionStat = stats
	s.m.Unlock()
}

func (s *ConfigConsumptionState) GetTierConsumptionStats() *omap.OrderedMap[consts.ModelTier, *ConsumptionStats] {
	if s == nil {
		return nil
	}
	s.ensure()
	s.m.Lock()
	defer s.m.Unlock()
	return s.TierConsumptionStat
}

func normalizeConsumptionTier(tier consts.ModelTier) consts.ModelTier {
	if tier == "" {
		return consts.TierIntelligent
	}
	return tier
}

func (c *Config) ensureConsumptionState() *ConfigConsumptionState {
	if c == nil {
		return nil
	}
	if c.InitStatus == nil {
		c.InitStatus = NewConfigInitStatus()
	}
	return c.InitStatus.GetOrCreateConsumptionState()
}

func (c *Config) SetConsumptionUUID(uuid string) {
	state := c.ensureConsumptionState()
	if state != nil {
		state.SetConsumptionUUID(uuid)
	}
}

func (c *Config) GetConsumptionUUID() string {
	state := c.ensureConsumptionState()
	if state == nil {
		return ""
	}
	return state.GetConsumptionUUID()
}

func (c *Config) ensureTierConsumptionStats() *omap.OrderedMap[consts.ModelTier, *ConsumptionStats] {
	state := c.ensureConsumptionState()
	if state == nil {
		return nil
	}
	return state.GetTierConsumptionStats()
}

func (c *Config) GetOrCreateTierConsumptionStats(tier consts.ModelTier) *ConsumptionStats {
	statsByTier := c.ensureTierConsumptionStats()
	if statsByTier == nil {
		return nil
	}
	normalizedTier := normalizeConsumptionTier(tier)
	return statsByTier.GetOrSet(normalizedTier, newConsumptionStats())
}

func (c *Config) AddTierConsumption(tier consts.ModelTier, inputDelta, outputDelta int64) {
	if inputDelta == 0 && outputDelta == 0 {
		return
	}
	stats := c.GetOrCreateTierConsumptionStats(tier)
	if stats == nil {
		return
	}
	stats.Add(inputDelta, outputDelta)
}

func (c *Config) GetTierConsumptionSnapshot() map[string]map[string]int64 {
	statsByTier := c.ensureTierConsumptionStats()
	result := make(map[string]map[string]int64)
	if statsByTier == nil {
		return result
	}
	statsByTier.ForEach(func(tier consts.ModelTier, stats *ConsumptionStats) bool {
		result[string(tier)] = stats.Snapshot()
		return true
	})
	return result
}
