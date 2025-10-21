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
	emitter         *Emitter
	knowledgeGetter func(ctx context.Context, emitter *Emitter, query string) (<-chan EnhanceKnowledge, error)

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
	//todo 支持多种来源的知识方式合并 rag ｜ web search

	result := chanx.NewUnlimitedChan[EnhanceKnowledge](ctx, 10)
	midResult, err := m.knowledgeGetter(ctx, m.emitter, query)
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

func (m *EnhanceKnowledgeManager) DumpTaskAboutKnowledgeWithTop(taskID string, top int) string {
	knowledges := m.GetKnowledgeByTaskID(taskID)
	var sb strings.Builder
	if top > 0 && len(knowledges) > top {
		knowledges = knowledges[:top]
	}
	for _, ek := range knowledges {
		sb.WriteString(fmt.Sprintf("-%s \n%s\n (Source: %s, Relevance: %.2f)\n", ek.GetTitle(), ek.GetContent(), ek.GetSource(), ek.GetScore()))
	}
	return sb.String()
}

func NewEnhanceKnowledgeManager(knowledgeGetter func(ctx context.Context, emitter *Emitter, query string) (<-chan EnhanceKnowledge, error)) *EnhanceKnowledgeManager {
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
