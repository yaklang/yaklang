package aicommon

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
)

type KnowledgeCollection struct {
	mux              sync.Mutex
	allKnowledgeList []EnhanceKnowledge
	uselessKnowledge map[string]struct{}
}

func NewKnowledgeCollection() *KnowledgeCollection {
	return &KnowledgeCollection{
		mux:              sync.Mutex{},
		allKnowledgeList: make([]EnhanceKnowledge, 0),
	}
}

func (kc *KnowledgeCollection) Append(knowledge EnhanceKnowledge) {
	kc.mux.Lock()
	defer kc.mux.Unlock()
	kc.allKnowledgeList = append(kc.allKnowledgeList, knowledge)
}

func (kc *KnowledgeCollection) GetKnowledgeList() []EnhanceKnowledge {
	kc.mux.Lock()
	defer kc.mux.Unlock()
	var result = kc.allKnowledgeList
	if len(kc.uselessKnowledge) > 0 {
		result = lo.Filter(kc.allKnowledgeList, func(ek EnhanceKnowledge, index int) bool {
			if kc.uselessKnowledge != nil {
				if _, isUseless := kc.uselessKnowledge[ek.GetUUID()]; isUseless {
					return false
				}
			}
			return true
		})
	}
	return utils.RRFRankWithDefaultK[EnhanceKnowledge](result) // will copy new slice
}

func (kc *KnowledgeCollection) SetUseless(uuid string) {
	kc.mux.Lock()
	defer kc.mux.Unlock()
	if kc.uselessKnowledge == nil {
		kc.uselessKnowledge = make(map[string]struct{})
	}
	kc.uselessKnowledge[uuid] = struct{}{}
}

func (kc *KnowledgeCollection) UnsetUseless(uuid string) {
	kc.mux.Lock()
	defer kc.mux.Unlock()
	if kc.uselessKnowledge == nil {
		return
	}
	delete(kc.uselessKnowledge, uuid)
}

type EnhanceKnowledgeManager struct {
	emitter             *Emitter
	knowledgeGetter     func(ctx context.Context, emitter *Emitter, collections []string, query string) (<-chan EnhanceKnowledge, error)
	mux                 sync.Mutex
	knowledgeMap        map[string]EnhanceKnowledge
	taskToKnowledgeUUID map[string]*KnowledgeCollection
}

func (m *EnhanceKnowledgeManager) SetEmitter(emitter *Emitter) {
	if m == nil {
		return
	}
	m.emitter = emitter
}

func (m *EnhanceKnowledgeManager) FetchKnowledge(ctx context.Context, query string) (<-chan EnhanceKnowledge, error) {
	return m.FetchKnowledgeWithCollections(ctx, []string{}, query)
}

func (m *EnhanceKnowledgeManager) FetchKnowledgeWithCollections(ctx context.Context, collections []string, query string) (<-chan EnhanceKnowledge, error) {
	//todo 支持多种来源的知识方式合并 rag ｜ web search

	result := chanx.NewUnlimitedChan[EnhanceKnowledge](ctx, 10)
	midResult, err := m.knowledgeGetter(ctx, m.emitter, collections, query)
	if err != nil {
		return nil, err
	}
	go func() {
		defer result.Close()
		for knowledge := range midResult {
			result.SafeFeed(knowledge)
		}
	}()

	return result.OutputChannel(), nil
}

func (m *EnhanceKnowledgeManager) AppendKnowledge(taskID string, knowledge EnhanceKnowledge) {
	m.mux.Lock()
	defer m.mux.Unlock()
	if _, exist := m.knowledgeMap[knowledge.GetUUID()]; !exist {
		m.knowledgeMap[knowledge.GetUUID()] = knowledge
	}
	collection, ok := m.taskToKnowledgeUUID[taskID]
	if !ok {
		collection = NewKnowledgeCollection()
	}
	collection.Append(knowledge)
	m.taskToKnowledgeUUID[taskID] = collection
}

func (m *EnhanceKnowledgeManager) GetKnowledgeByTaskID(taskID string) []EnhanceKnowledge {
	m.mux.Lock()
	defer m.mux.Unlock()
	collection, ok := m.taskToKnowledgeUUID[taskID]
	if !ok {
		return nil
	}

	return collection.GetKnowledgeList()
}

func (m *EnhanceKnowledgeManager) SetKnowledgeUseless(taskID, uuid string) {
	m.mux.Lock()
	defer m.mux.Unlock()
	collection, ok := m.taskToKnowledgeUUID[taskID]
	if !ok {
		return
	}
	collection.SetUseless(uuid)
}

func (m *EnhanceKnowledgeManager) UnsetKnowledge(taskID, uuid string) {
	m.mux.Lock()
	defer m.mux.Unlock()
	collection, ok := m.taskToKnowledgeUUID[taskID]
	if !ok {
		return
	}
	collection.UnsetUseless(uuid)
}

func (m *EnhanceKnowledgeManager) DumpTaskAboutKnowledge(taskID string) string {
	return m.DumpTaskAboutKnowledgeWithTop(taskID, 0)
}

// aggregatedKnowledge is used for grouping knowledge entries by knowledge title
type aggregatedKnowledge struct {
	KnowledgeTitle   string   // knowledge title for grouping
	Source           string   // knowledge source
	HitQueries       []string // all search queries that hit this knowledge
	KnowledgeDetails string   // knowledge details (name, display name, type, description, keywords)
	SearchType       string   // search type
	SearchTarget     string   // search target
}

func (m *EnhanceKnowledgeManager) DumpTaskAboutKnowledgeWithTop(taskID string, top int) string {
	knowledges := m.GetKnowledgeByTaskID(taskID)
	if len(knowledges) == 0 {
		return ""
	}

	// deduplicate by UUID
	seen := make(map[string]bool)
	var uniqueKnowledges []EnhanceKnowledge
	for _, ek := range knowledges {
		uuid := ek.GetUUID()
		if uuid != "" && seen[uuid] {
			continue
		}
		if uuid != "" {
			seen[uuid] = true
		}
		uniqueKnowledges = append(uniqueKnowledges, ek)
	}
	knowledges = uniqueKnowledges

	// classify: knowledge entries vs direct search results
	var knowledgeEntries []EnhanceKnowledge
	var directSearchResults []EnhanceKnowledge
	for _, ek := range knowledges {
		if ek.GetKnowledgeEntryUUID() != "" {
			knowledgeEntries = append(knowledgeEntries, ek)
		} else {
			directSearchResults = append(directSearchResults, ek)
		}
	}

	// limit total count
	if top > 0 {
		totalKnowledge := len(knowledgeEntries)
		totalDirect := len(directSearchResults)
		if totalKnowledge+totalDirect > top {
			// prioritize knowledge entries
			if totalKnowledge >= top {
				knowledgeEntries = knowledgeEntries[:top]
				directSearchResults = nil
			} else {
				remaining := top - totalKnowledge
				if len(directSearchResults) > remaining {
					directSearchResults = directSearchResults[:remaining]
				}
			}
		}
	}

	var sb strings.Builder

	// output aggregated knowledge entries
	if len(knowledgeEntries) > 0 {
		sb.WriteString("=== 关联知识 ===\n\n")
		m.dumpAggregatedKnowledge(&sb, knowledgeEntries)
	}

	// output aggregated direct search results
	if len(directSearchResults) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("=== 搜索结果 ===\n\n")
		m.dumpAggregatedKnowledge(&sb, directSearchResults)
	}

	return sb.String()
}

// dumpAggregatedKnowledge aggregates knowledge by knowledge title and outputs
func (m *EnhanceKnowledgeManager) dumpAggregatedKnowledge(sb *strings.Builder, knowledges []EnhanceKnowledge) {
	// aggregate by knowledge title (or title if knowledge title is empty)
	aggregated := make(map[string]*aggregatedKnowledge)
	var orderedKeys []string

	for _, ek := range knowledges {
		// use knowledge title as group key, fallback to title
		groupKey := ek.GetKnowledgeTitle()
		if groupKey == "" {
			groupKey = ek.GetTitle()
		}
		if groupKey == "" {
			groupKey = "[untitled]"
		}

		if existing, ok := aggregated[groupKey]; ok {
			// aggregate hit queries (the search questions that matched)
			hitQuery := ek.GetTitle()
			if hitQuery != "" && hitQuery != groupKey {
				// avoid duplicate hit queries
				found := false
				for _, q := range existing.HitQueries {
					if q == hitQuery {
						found = true
						break
					}
				}
				if !found {
					existing.HitQueries = append(existing.HitQueries, hitQuery)
				}
			}
			// update knowledge details if not set yet
			if existing.KnowledgeDetails == "" {
				details := ek.GetKnowledgeDetails()
				if details == "" {
					// fallback to content if no knowledge details
					details = ek.GetContent()
				}
				if details != "" {
					existing.KnowledgeDetails = details
				}
			}
		} else {
			agg := &aggregatedKnowledge{
				KnowledgeTitle: groupKey,
				Source:         ek.GetSource(),
				SearchType:     ek.GetSearchType(),
				SearchTarget:   ek.GetSearchTarget(),
			}
			// add hit query if different from group key
			hitQuery := ek.GetTitle()
			if hitQuery != "" && hitQuery != groupKey {
				agg.HitQueries = append(agg.HitQueries, hitQuery)
			}
			// get knowledge details (the real content: name, type, description, keywords)
			// fallback to content if no knowledge details available
			details := ek.GetKnowledgeDetails()
			if details == "" {
				details = ek.GetContent()
			}
			agg.KnowledgeDetails = details
			aggregated[groupKey] = agg
			orderedKeys = append(orderedKeys, groupKey)
		}
	}

	// output aggregated knowledge
	for idx, key := range orderedKeys {
		agg := aggregated[key]
		m.dumpSingleAggregatedKnowledge(sb, idx+1, agg)
	}
}

// dumpSingleAggregatedKnowledge outputs a single aggregated knowledge entry
func (m *EnhanceKnowledgeManager) dumpSingleAggregatedKnowledge(sb *strings.Builder, index int, agg *aggregatedKnowledge) {
	// format: {index}. [{source}/{knowledge_title}] (hit: query1, query2, ...)
	source := agg.Source
	if source == "" {
		source = "unknown"
	}

	// title line with hit queries inline (compact format)
	sb.WriteString(fmt.Sprintf("%d. [%s/%s]", index, source, agg.KnowledgeTitle))
	if len(agg.HitQueries) > 0 {
		// limit hit queries display to avoid too long line
		displayQueries := agg.HitQueries
		if len(displayQueries) > 3 {
			displayQueries = append(displayQueries[:3], fmt.Sprintf("...+%d", len(agg.HitQueries)-3))
		}
		sb.WriteString(fmt.Sprintf(" (hit: %s)", strings.Join(displayQueries, "; ")))
	}
	sb.WriteString("\n")

	// output knowledge details (the real content: name, type, description, keywords)
	if agg.KnowledgeDetails != "" {
		lines := strings.Split(agg.KnowledgeDetails, "\n")
		for _, line := range lines {
			if line != "" {
				sb.WriteString(fmt.Sprintf("   %s\n", line))
			}
		}
	}

	sb.WriteString("\n")
}

// dumpSingleKnowledge outputs a single knowledge entry (legacy method, kept for compatibility)
func (m *EnhanceKnowledgeManager) dumpSingleKnowledge(sb *strings.Builder, ek EnhanceKnowledge) {
	// title line
	title := ek.GetTitle()
	if title == "" {
		title = "[untitled]"
	}

	// build identifiers
	var identifiers []string
	if searchType := ek.GetSearchType(); searchType != "" {
		identifiers = append(identifiers, fmt.Sprintf("type:%s", searchType))
	}
	if searchTarget := ek.GetSearchTarget(); searchTarget != "" {
		identifiers = append(identifiers, fmt.Sprintf("target:%s", searchTarget))
	}

	if len(identifiers) > 0 {
		sb.WriteString(fmt.Sprintf("[%s]", strings.Join(identifiers, " | ")))
	}
	sb.WriteString(fmt.Sprintf(" %s\n", title))

	// knowledge title (if linked to knowledge)
	if knowledgeTitle := ek.GetKnowledgeTitle(); knowledgeTitle != "" {
		sb.WriteString(fmt.Sprintf("  -> Knowledge: %s\n", knowledgeTitle))
	}

	// content
	content := ek.GetContent()
	if content != "" {
		sb.WriteString(fmt.Sprintf("%s\n", content))
	}

	// source info
	sb.WriteString(fmt.Sprintf("(source: %s)\n\n", ek.GetSource()))
}

func NewEnhanceKnowledgeManager(knowledgeGetter func(ctx context.Context, emitter *Emitter, query string) (<-chan EnhanceKnowledge, error)) *EnhanceKnowledgeManager {
	return &EnhanceKnowledgeManager{
		knowledgeGetter: func(ctx context.Context, emitter *Emitter, collections []string, query string) (<-chan EnhanceKnowledge, error) {
			return knowledgeGetter(ctx, emitter, query)
		},
		knowledgeMap:        make(map[string]EnhanceKnowledge),
		taskToKnowledgeUUID: make(map[string]*KnowledgeCollection),
		mux:                 sync.Mutex{},
	}
}

func NewEnhanceKnowledgeManagerWithCollectionLimitGetter(knowledgeGetter func(ctx context.Context, emitter *Emitter, collections []string, query string) (<-chan EnhanceKnowledge, error)) *EnhanceKnowledgeManager {
	return &EnhanceKnowledgeManager{
		knowledgeGetter:     knowledgeGetter,
		knowledgeMap:        make(map[string]EnhanceKnowledge),
		taskToKnowledgeUUID: make(map[string]*KnowledgeCollection),
		mux:                 sync.Mutex{},
	}
}

type EnhanceKnowledge interface {
	GetContent() string
	GetSource() string
	GetScore() float64
	GetType() string
	GetTitle() string
	GetScoreMethod() string
	GetUUID() string
	GetSearchTarget() string       // 搜索目标（如插件名、工具名等）
	GetSearchType() string         // 搜索类型（如 AI工具、Yak插件 等）
	GetKnowledgeTitle() string     // 关联知识的标题
	GetKnowledgeEntryUUID() string // 关联知识条目的 UUID，用于动态查询详情
	GetKnowledgeDetails() string   // 获取知识详情（名称、显示名称、类型、功能描述、关键词等）
}

type BasicEnhanceKnowledge struct {
	Content string  // 内容
	Source  string  // 来源
	Score   float64 // 相关性评分，0~1之间]
	UUID    string
}

type LazyEnhanceKnowledge struct {
	BasicEnhanceKnowledge
	ContentLoader func() string
}

func (e *LazyEnhanceKnowledge) GetContent() string {
	if e == nil {
		return ""
	}

	if e.ContentLoader != nil {
		return e.ContentLoader()
	}

	return e.Content
}

func NewBasicEnhanceKnowledge(content, source string, score float64) *BasicEnhanceKnowledge {
	return &BasicEnhanceKnowledge{
		Content: content,
		Source:  source,
		Score:   score,
	}
}

func (e *BasicEnhanceKnowledge) GetContent() string {
	if e == nil {
		return ""
	}
	return e.Content
}

func (e *BasicEnhanceKnowledge) GetSource() string {
	if e == nil {
		return ""
	}
	return e.Source
}

func (e *BasicEnhanceKnowledge) GetScore() float64 {
	if e == nil {
		return 0
	}
	return e.Score
}

func (e *BasicEnhanceKnowledge) GetType() string {
	return ""
}

func (e *BasicEnhanceKnowledge) GetTitle() string {
	return ""
}

func (e *BasicEnhanceKnowledge) GetScoreMethod() string {
	return ""
}

func (e *BasicEnhanceKnowledge) GetUUID() string {
	if e == nil {
		return ""
	}
	if e.UUID == "" {
		e.UUID = uuid.NewString()
	}
	return e.UUID
}

func (e *BasicEnhanceKnowledge) GetSearchTarget() string {
	return ""
}

func (e *BasicEnhanceKnowledge) GetSearchType() string {
	return ""
}

func (e *BasicEnhanceKnowledge) GetKnowledgeTitle() string {
	return ""
}

func (e *BasicEnhanceKnowledge) GetKnowledgeEntryUUID() string {
	return ""
}

func (e *BasicEnhanceKnowledge) GetKnowledgeDetails() string {
	return ""
}
