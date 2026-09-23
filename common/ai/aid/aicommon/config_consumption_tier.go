package aicommon

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type ConsumptionStats struct {
	InputConsumption  int64 `json:"input_consumption"`
	OutputConsumption int64 `json:"output_consumption"`
	CacheHitToken     int64 `json:"cache_hit_token"`
}

// ModelConsumptionIdentity identifies the effective model configuration used by
// one completed AI call. It intentionally contains no provider credentials or
// endpoint fields.
type ModelConsumptionIdentity struct {
	ProviderType  string `json:"provider_type,omitempty"`
	ModelName     string `json:"model_name,omitempty"`
	ThinkingLevel string `json:"thinking_level,omitempty"`
}

type ModelConsumptionStats struct {
	ModelConsumptionIdentity
	ConsumptionStats
}

type ModelConsumptionSnapshot struct {
	ModelConsumptionIdentity
	InputConsumption  int64 `json:"input_consumption"`
	OutputConsumption int64 `json:"output_consumption"`
	CacheHitToken     int64 `json:"cache_hit_token"`
}

func normalizeModelConsumptionIdentity(identity ModelConsumptionIdentity) ModelConsumptionIdentity {
	identity.ProviderType = strings.TrimSpace(identity.ProviderType)
	identity.ModelName = strings.TrimSpace(identity.ModelName)
	identity.ThinkingLevel = strings.ToLower(strings.TrimSpace(identity.ThinkingLevel))
	if identity.ThinkingLevel == "" {
		identity.ThinkingLevel = "auto"
	}
	return identity
}

func modelConsumptionIdentityKey(identity ModelConsumptionIdentity) string {
	return identity.ProviderType + "\x00" + identity.ModelName + "\x00" + identity.ThinkingLevel
}

func (s *ModelConsumptionStats) Snapshot() ModelConsumptionSnapshot {
	if s == nil {
		return ModelConsumptionSnapshot{}
	}
	return ModelConsumptionSnapshot{
		ModelConsumptionIdentity: s.ModelConsumptionIdentity,
		InputConsumption:         atomic.LoadInt64(&s.InputConsumption),
		OutputConsumption:        atomic.LoadInt64(&s.OutputConsumption),
		CacheHitToken:            atomic.LoadInt64(&s.CacheHitToken),
	}
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

func (s *ConsumptionStats) AddCacheHit(cacheHitDelta int64) {
	if s == nil {
		return
	}
	if cacheHitDelta != 0 {
		atomic.AddInt64(&s.CacheHitToken, cacheHitDelta)
	}
}

func (s *ConsumptionStats) Snapshot() map[string]int64 {
	if s == nil {
		return map[string]int64{
			"input_consumption":  0,
			"output_consumption": 0,
			"cache_hit_token":    0,
		}
	}
	return map[string]int64{
		"input_consumption":  atomic.LoadInt64(&s.InputConsumption),
		"output_consumption": atomic.LoadInt64(&s.OutputConsumption),
		"cache_hit_token":    atomic.LoadInt64(&s.CacheHitToken),
	}
}

type ConfigConsumptionState struct {
	InputConsumption           *int64
	OutputConsumption          *int64
	CacheHitToken              *int64
	ConsumptionUUID            string
	TierConsumptionStat        *omap.OrderedMap[consts.ModelTier, *ConsumptionStats]
	TierModelConsumptionStat   map[consts.ModelTier]map[string]*ModelConsumptionStats
	EffectiveSingleModelMode   bool
	effectiveModelModeResolved bool
	m                          *sync.Mutex
}

func NewConfigConsumptionState() *ConfigConsumptionState {
	return (&ConfigConsumptionState{
		InputConsumption:         new(int64),
		OutputConsumption:        new(int64),
		CacheHitToken:            new(int64),
		TierConsumptionStat:      omap.NewOrderedMap(map[consts.ModelTier]*ConsumptionStats{}),
		TierModelConsumptionStat: make(map[consts.ModelTier]map[string]*ModelConsumptionStats),
		m:                        &sync.Mutex{},
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
	if s.CacheHitToken == nil {
		s.CacheHitToken = new(int64)
	}
	if s.TierConsumptionStat == nil {
		s.TierConsumptionStat = omap.NewOrderedMap(map[consts.ModelTier]*ConsumptionStats{})
	}
	if s.TierModelConsumptionStat == nil {
		s.TierModelConsumptionStat = make(map[consts.ModelTier]map[string]*ModelConsumptionStats)
	}
	return s
}

// InitializeEffectiveSingleModelMode records the mode resolved by the root
// Config. ConfigInitStatus is shared by child Configs, so the first resolved
// value owns the session-level consumption state.
func (s *ConfigConsumptionState) InitializeEffectiveSingleModelMode(enabled bool) {
	if s == nil {
		return
	}
	s.ensure()
	s.m.Lock()
	defer s.m.Unlock()
	if s.effectiveModelModeResolved {
		return
	}
	s.EffectiveSingleModelMode = enabled
	s.effectiveModelModeResolved = true
}

func (s *ConfigConsumptionState) GetEffectiveSingleModelMode() bool {
	if s == nil {
		return false
	}
	s.ensure()
	s.m.Lock()
	defer s.m.Unlock()
	return s.EffectiveSingleModelMode
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

func (s *ConfigConsumptionState) GetCacheHitTokenPointer() *int64 {
	if s == nil {
		return nil
	}
	s.ensure()
	s.m.Lock()
	defer s.m.Unlock()
	return s.CacheHitToken
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

func (s *ConfigConsumptionState) AddTierInputConsumption(tier consts.ModelTier, inputDelta int64) {
	if s == nil {
		return
	}
	s.ensure()
	normalizedTier := normalizeConsumptionTier(tier)
	s.m.Lock()
	stats, _ := s.TierConsumptionStat.Get(normalizedTier)
	if stats == nil {
		stats = newConsumptionStats()
		s.TierConsumptionStat.Set(normalizedTier, stats)
	}
	s.m.Unlock()
	stats.Add(inputDelta, 0)
	atomic.AddInt64(s.InputConsumption, inputDelta)
}

func (s *ConfigConsumptionState) AddTierOutputConsumption(tier consts.ModelTier, outputDelta int64) {
	if s == nil {
		return
	}
	s.ensure()
	normalizedTier := normalizeConsumptionTier(tier)
	s.m.Lock()
	stats, _ := s.TierConsumptionStat.Get(normalizedTier)
	if stats == nil {
		stats = newConsumptionStats()
		s.TierConsumptionStat.Set(normalizedTier, stats)
	}
	s.m.Unlock()
	stats.Add(0, outputDelta)
	atomic.AddInt64(s.OutputConsumption, outputDelta)
}

func (s *ConfigConsumptionState) AddTierConsumption(tier consts.ModelTier, inputDelta, outputDelta int64) {
	if s == nil || (inputDelta == 0 && outputDelta == 0) {
		return
	}
	s.AddTierOutputConsumption(tier, outputDelta)
	s.AddTierInputConsumption(tier, inputDelta)
}

func (s *ConfigConsumptionState) AddTierCacheHitToken(tier consts.ModelTier, cacheHitDelta int64) {
	if s == nil || cacheHitDelta == 0 {
		return
	}
	s.ensure()
	normalizedTier := normalizeConsumptionTier(tier)
	s.m.Lock()
	stats, _ := s.TierConsumptionStat.Get(normalizedTier)
	if stats == nil {
		stats = newConsumptionStats()
		s.TierConsumptionStat.Set(normalizedTier, stats)
	}
	s.m.Unlock()
	stats.AddCacheHit(cacheHitDelta)
	atomic.AddInt64(s.CacheHitToken, cacheHitDelta)
}

// AddTierModelConsumption records one completed call in two dimensions: the
// logical tier total and the concrete model bucket within that tier.
func (s *ConfigConsumptionState) AddTierModelConsumption(
	tier consts.ModelTier,
	identity ModelConsumptionIdentity,
	inputDelta, outputDelta, cacheHitDelta int64,
) {
	if s == nil {
		return
	}
	s.AddTierConsumption(tier, inputDelta, outputDelta)
	s.AddTierCacheHitToken(tier, cacheHitDelta)
	s.addModelConsumption(tier, identity, inputDelta, outputDelta, cacheHitDelta)
}

// addModelConsumption only maintains the model breakdown. Tier and global
// totals are owned by AddTierConsumption/AddTierCacheHitToken.
func (s *ConfigConsumptionState) addModelConsumption(
	tier consts.ModelTier,
	identity ModelConsumptionIdentity,
	inputDelta, outputDelta, cacheHitDelta int64,
) {
	identity = normalizeModelConsumptionIdentity(identity)
	if identity.ProviderType == "" && identity.ModelName == "" {
		return
	}
	s.ensure()
	tier = normalizeConsumptionTier(tier)

	s.m.Lock()
	models := s.TierModelConsumptionStat[tier]
	if models == nil {
		models = make(map[string]*ModelConsumptionStats)
		s.TierModelConsumptionStat[tier] = models
	}
	key := modelConsumptionIdentityKey(identity)
	stats := models[key]
	if stats == nil {
		stats = &ModelConsumptionStats{ModelConsumptionIdentity: identity}
		models[key] = stats
	}
	s.m.Unlock()

	stats.Add(inputDelta, outputDelta)
	stats.AddCacheHit(cacheHitDelta)
}

func (s *ConfigConsumptionState) GetTierModelConsumptionSnapshot() map[string][]ModelConsumptionSnapshot {
	result := make(map[string][]ModelConsumptionSnapshot)
	if s == nil {
		return result
	}
	s.ensure()
	s.m.Lock()
	defer s.m.Unlock()
	for tier, models := range s.TierModelConsumptionStat {
		items := make([]ModelConsumptionSnapshot, 0, len(models))
		for _, stats := range models {
			items = append(items, stats.Snapshot())
		}
		sort.Slice(items, func(i, j int) bool {
			// Input consumption excludes cached tokens; sort by all input tokens.
			inputI := items[i].InputConsumption + items[i].CacheHitToken
			inputJ := items[j].InputConsumption + items[j].CacheHitToken
			if inputI != inputJ {
				return inputI > inputJ
			}
			if items[i].ProviderType != items[j].ProviderType {
				return items[i].ProviderType < items[j].ProviderType
			}
			if items[i].ModelName != items[j].ModelName {
				return items[i].ModelName < items[j].ModelName
			}
			return items[i].ThinkingLevel < items[j].ThinkingLevel
		})
		result[string(tier)] = items
	}
	return result
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

func (c *Config) AddTierConsumption(tier consts.ModelTier, inputDelta, outputDelta int64) {
	state := c.ensureConsumptionState()
	state.AddTierConsumption(tier, inputDelta, outputDelta)
}

func (c *Config) InputConsumptionCallback(tier consts.ModelTier, current int) {
	state := c.ensureConsumptionState()
	if state == nil {
		return
	}
	state.AddTierInputConsumption(tier, int64(current))
}

func (c *Config) OutputConsumptionCallback(tier consts.ModelTier, current int) {
	state := c.ensureConsumptionState()
	if state == nil {
		return
	}
	state.AddTierOutputConsumption(tier, int64(current))
}

func (c *Config) AddTierCacheHitToken(tier consts.ModelTier, cacheHitDelta int64) {
	if cacheHitDelta == 0 {
		return
	}
	state := c.ensureConsumptionState()
	if state == nil {
		return
	}
	state.AddTierCacheHitToken(tier, cacheHitDelta)
}

func (c *Config) AddTierModelConsumption(
	tier consts.ModelTier,
	identity ModelConsumptionIdentity,
	inputDelta, outputDelta, cacheHitDelta int64,
) {
	state := c.ensureConsumptionState()
	if state == nil {
		return
	}
	state.AddTierModelConsumption(tier, identity, inputDelta, outputDelta, cacheHitDelta)
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

func (c *Config) GetTierModelConsumptionSnapshot() map[string][]ModelConsumptionSnapshot {
	state := c.ensureConsumptionState()
	if state == nil {
		return map[string][]ModelConsumptionSnapshot{}
	}
	return state.GetTierModelConsumptionSnapshot()
}

func (c *Config) BuildConsumptionPayload() map[string]any {
	state := c.ensureConsumptionState()
	effectiveSingleModelMode := false
	if state != nil {
		effectiveSingleModelMode = state.GetEffectiveSingleModelMode()
	}
	return map[string]any{
		"input_consumption":           c.GetInputConsumption(),
		"output_consumption":          c.GetOutputConsumption(),
		"cache_hit_token":             c.GetCacheHitToken(),
		"consumption_uuid":            c.GetConsumptionUUID(),
		"effective_single_model_mode": effectiveSingleModelMode,
		"tier_consumption":            c.GetTierConsumptionSnapshot(),
		"tier_model_consumption":      c.GetTierModelConsumptionSnapshot(),
	}
}
