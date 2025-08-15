package aid

import (
	"bytes"
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"io"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/linktable"
	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type timelineItem struct {
	deleted bool

	value TimelineItemValue // *aitool.ToolResult
}

func (item *timelineItem) GetShrinkResult() string {
	return item.value.GetShrinkResult()
}

func (item *timelineItem) GetShrinkSimilarResult() string {
	return item.value.GetShrinkSimilarResult()
}

func (item *timelineItem) String() string {
	return item.value.String()
}

func (item *timelineItem) SetShrinkResult(pers string) {
	item.value.SetShrinkResult(pers)
}

func (item *timelineItem) GetID() int64 {
	if item.value == nil {
		return 0
	}
	return item.value.GetID()
}

var _ TimelineItemValue = (*timelineItem)(nil)

type memoryTimeline struct {
	memory *Memory
	config *Config

	ai aicommon.AICaller

	idToTs           *omap.OrderedMap[int64, int64]
	tsToTimelineItem *omap.OrderedMap[int64, *timelineItem]
	idToTimelineItem *omap.OrderedMap[int64, *timelineItem]
	summary          *omap.OrderedMap[int64, *linktable.LinkTable[*timelineItem]]
	reducers         *omap.OrderedMap[int64, *linktable.LinkTable[string]]

	// this limit is used to limit the number of timeline items.
	maxTimelineLimit int // total timeline item count
	fullMemoryCount  int // full memory timeline item count

	// this limit is used to limit the timeline dump string size.
	perDumpContentLimit   int
	totalDumpContentLimit int
}

func (m *memoryTimeline) CopyReducibleTimelineWithMemory(mem *Memory) *memoryTimeline {
	tl := &memoryTimeline{
		memory:                mem,
		config:                m.config,
		idToTs:                m.idToTs.Copy(),
		tsToTimelineItem:      m.tsToTimelineItem.Copy(),
		idToTimelineItem:      m.idToTimelineItem.Copy(),
		summary:               m.summary.Copy(),
		reducers:              m.reducers.Copy(),
		maxTimelineLimit:      m.maxTimelineLimit,
		fullMemoryCount:       m.fullMemoryCount,
		perDumpContentLimit:   m.perDumpContentLimit,
		totalDumpContentLimit: m.totalDumpContentLimit,
	}
	return tl
}

func (m *memoryTimeline) SoftDelete(id ...int64) {
	for _, i := range id {
		if v, ok := m.idToTimelineItem.Get(i); ok {
			v.deleted = true
		}
		if v, ok := m.summary.Get(i); ok {
			v.Push(&timelineItem{
				value:   v.Value().value,
				deleted: true,
			})
		}
	}
}

func (m *memoryTimeline) CreateSubTimeline(ids ...int64) *memoryTimeline {
	tl := newMemoryTimeline(m.fullMemoryCount, m.ai)
	if m.memory != nil {
		tl.memory = m.memory
	}
	if m.config != nil {
		tl.config = m.config
	}
	if len(ids) == 0 {
		return nil
	}
	tl.ai = m.ai
	for _, id := range ids {
		ts, ok := m.idToTs.Get(id)
		if !ok {
			continue
		}
		tl.idToTs.Set(id, ts)
		if ret, ok := m.idToTimelineItem.Get(id); ok {
			tl.idToTimelineItem.Set(id, ret)
		}
		if ret, ok := m.tsToTimelineItem.Get(ts); ok {
			tl.tsToTimelineItem.Set(ts, ret)
		}
		if ret, ok := m.summary.Get(id); ok {
			tl.summary.Set(id, ret)
		}
		if ret, ok := m.reducers.Get(id); ok {
			tl.reducers.Set(id, ret)
		}
	}
	return tl
}

func (m *memoryTimeline) BindConfig(config *Config) {
	m.config = config
	m.memory = config.memory
	if m.memory == nil {
		m.memory = GetDefaultMemory()
	}
	m.setTimelineLimit(config.timelineRecordLimit)
	m.setTimelineContentLimit(config.timelineContentSizeLimit)
	if utils.IsNil(m.ai) {
		m.setAICaller(config)
	}
}

func newMemoryTimeline(clearCount int, ai aicommon.AICaller) *memoryTimeline {
	return &memoryTimeline{
		ai:               ai,
		fullMemoryCount:  clearCount,
		maxTimelineLimit: 3 * clearCount,
		tsToTimelineItem: omap.NewOrderedMap(map[int64]*timelineItem{}),
		idToTimelineItem: omap.NewOrderedMap(map[int64]*timelineItem{}),
		idToTs:           omap.NewOrderedMap(map[int64]int64{}),
		summary:          omap.NewOrderedMap(map[int64]*linktable.LinkTable[*timelineItem]{}),
		reducers:         omap.NewOrderedMap(map[int64]*linktable.LinkTable[string]{}),
	}
}

func (m *memoryTimeline) setTimelineLimit(clearCount int) {
	m.fullMemoryCount = clearCount
	m.maxTimelineLimit = 3 * clearCount
}

func (m *memoryTimeline) setTimelineContentLimit(contentSize int) {
	m.totalDumpContentLimit = contentSize
}

func (m *memoryTimeline) setAICaller(ai aicommon.AICaller) {
	m.ai = ai
}

func (m *memoryTimeline) PushToolResult(toolResult *aitool.ToolResult) {
	ts := time.Now().UnixMilli()
	if m.tsToTimelineItem.Have(ts) {
		time.Sleep(time.Millisecond * 10)
		ts = time.Now().UnixMilli()
	}
	m.idToTs.Set(toolResult.GetID(), ts)

	item := &timelineItem{
		value: toolResult,
	}

	// if item dump string > perDumpContentLimit should shrink this item
	if m.perDumpContentLimit > 0 && len(item.String()) > m.perDumpContentLimit {
		m.shrink(item)
	}

	m.tsToTimelineItem.Set(ts, item)
	m.idToTimelineItem.Set(toolResult.GetID(), item)
	m.timelineLengthCheck()
	m.dumpSizeCheck()
}

func (m *memoryTimeline) PushUserInteraction(stage UserInteractionStage, id int64, systemPrompt string, userExtraPrompt string) {
	ts := time.Now().UnixMilli()
	if m.tsToTimelineItem.Have(ts) {
		time.Sleep(time.Millisecond * 10)
		ts = time.Now().UnixMilli()
	}
	m.idToTs.Set(id, ts)

	item := &timelineItem{
		value: &UserInteraction{
			ID:              id,
			SystemPrompt:    systemPrompt,
			UserExtraPrompt: userExtraPrompt,
			Stage:           stage,
		},
	}

	if m.perDumpContentLimit > 0 && len(item.String()) > m.perDumpContentLimit {
		m.shrink(item)
	}

	m.tsToTimelineItem.Set(ts, item)
	m.idToTimelineItem.Set(id, item)
	m.timelineLengthCheck()
	m.dumpSizeCheck()
}

func (m *memoryTimeline) timelineLengthCheck() {
	total := m.idToTimelineItem.Len()
	summaryCount := m.summary.Len()
	if total-summaryCount > m.fullMemoryCount {
		shrinkTargetIndex := total - m.fullMemoryCount - 1
		id := m.idToTimelineItem.Index(shrinkTargetIndex)
		for _, v := range id.Values() {
			log.Infof("start to shrink memory timeline id: %v, total: %v, summary: %v, size: %v", v.value.GetID(), total, summaryCount, m.fullMemoryCount)
			m.shrink(v)
		}
	}

	if m.maxTimelineLimit > 0 && total-m.maxTimelineLimit > 0 {
		endIdx := total - m.maxTimelineLimit - 1
		rawValue, ok := m.idToTimelineItem.GetByIndex(endIdx)
		if ok {
			val := rawValue.value
			log.Infof("start to reducer from id: %v, total: %v, limit: %v, delta: %v", val.GetID(), total, m.maxTimelineLimit, total-m.maxTimelineLimit)
			m.reducer(val.GetID())
		}
	}
}

func (m *memoryTimeline) dumpSizeCheck() {
	if m.ai == nil {
		log.Error("ai is nil, memory cannot emit memory shrink")
		return
	}

	if m.totalDumpContentLimit <= 0 || len(m.Dump()) <= m.totalDumpContentLimit {
		return
	}
	totalLastID, _, _ := m.idToTimelineItem.Last()
	summaryLastID, _, _ := m.summary.Last()

	// check everyone timeline item was shrunk
	if totalLastID > summaryLastID {
		m.idToTimelineItem.ForEach(func(k int64, v *timelineItem) bool {
			if k > summaryLastID {
				log.Infof("start to shrink memory timeline id: %v", v.value.GetID())
				m.shrink(v)
				return false
			}
			return true
		})
	} else {
		reducerID := int64(0)
		if m.reducers.Len() > 0 { // has reducer, reducer index should be current reducer next
			reducerID, _, _ = m.reducers.Last()
		}
		m.idToTimelineItem.ForEach(func(k int64, v *timelineItem) bool {
			if k > reducerID {
				log.Infof("start to shrink memory timeline id: %v", v.value.GetID())
				m.reducer(k)
				return false
			}
			return true
		})
	}
	m.dumpSizeCheck() // recursion check
}

func (m *memoryTimeline) reducer(beforeId int64) {
	if beforeId <= 0 {
		return
	}
	pmt := m.renderReducerPrompt(beforeId)
	if utils.IsNil(m.ai) {
		return
	}

	if m.config == nil {
		CallAITransactionWithoutConfig(pmt, m.ai.CallAI, func(response *aicommon.AIResponse) error {
			action, err := aicommon.ExtractActionFromStream(response.GetUnboundStreamReader(false), "timeline-reducer")
			if err != nil {
				log.Errorf("extract timeline action failed: %v", err)
				return utils.Errorf("extract timeline-reducer failed: %v", err)
			}
			pers := action.GetString("reducer_memory")
			if pers != "" {
				if lt, ok := m.reducers.Get(beforeId); ok {
					lt.Push(pers)
				} else {
					m.reducers.Set(beforeId, linktable.NewUnlimitedStringLinkTable(pers))
				}
			}
			return nil
		})
	} else {
		m.config.callAiTransaction(pmt, m.ai.CallAI, func(response *aicommon.AIResponse) error {
			action, err := aicommon.ExtractActionFromStream(
				response.GetOutputStreamReader("memory-reducer", true, m.config.GetEmitter()),
				"timeline-reducer",
			)
			if err != nil {
				return utils.Errorf("extract timeline action failed: %v", err)
			}
			pers := action.GetString("reducer_memory")
			if pers != "" {
				if lt, ok := m.reducers.Get(beforeId); ok {
					lt.Push(pers)
				} else {
					m.reducers.Set(beforeId, linktable.NewUnlimitedStringLinkTable(pers))
				}
			}
			return nil
		})
	}
}

func (m *memoryTimeline) shrink(currentItem *timelineItem) {
	if m.ai == nil {
		log.Error("ai is nil, memory cannot emit memory shrink")
		return
	}

	response, err := m.ai.CallAI(aicommon.NewAIRequest(m.renderSummaryPrompt(currentItem)))
	if err != nil {
		log.Errorf("shrink call ai failed: %v", err)
		return
	}
	var r io.Reader
	if m.config == nil {
		r = response.GetUnboundStreamReader(false)
	} else {
		r = response.GetOutputStreamReader("memory-timeline", true, m.config.GetEmitter())
	}
	output, err := io.ReadAll(r)
	if err != nil {
		log.Errorf("read ai output failed: %v", err)
		return
	}
	action, err := aicommon.ExtractAction(string(output), "timeline-shrink")
	if err != nil {
		log.Errorf("extract timeline action failed: %v", err)
		return
	}
	pers := action.GetString("persistent")
	if pers == "" {
		s, ok := m.summary.Get(currentItem.GetID())
		if ok {
			pers = s.Value().GetShrinkResult()
			if pers == "" {
				pers = s.Value().GetShrinkSimilarResult()
			}
		}
	}
	newItem := *currentItem //  copy struct
	newItem.deleted = action.GetBool("should_drop", currentItem.deleted)
	//newItem.ShrinkResult = pers
	newItem.SetShrinkResult(pers)
	if lt, ok := m.summary.Get(currentItem.GetID()); ok {
		lt.Push(&newItem)
	} else {
		m.summary.Set(currentItem.GetID(), linktable.NewUnlimitedLinkTable(&newItem))
	}
}

//go:embed prompts/timeline/reducer_memory.txt
var timelineReducer string

func (m *memoryTimeline) renderReducerPrompt(beforeId int64) string {
	input := m.DumpBefore(beforeId)
	ins, err := template.New("timeline-reducer").Parse(timelineReducer)
	if err != nil {
		log.Errorf("BUG: dump summary prompt failed: %v", err)
		return ""
	}
	var buf bytes.Buffer
	var nonce = utils.RandStringBytes(6)
	err = ins.Execute(&buf, map[string]any{
		"Memory": m.memory,
		"Input":  input,
		`NONCE`:  nonce,
	})
	if err != nil {
		log.Errorf("BUG: dump summary prompt failed: %v", err)
		return ""
	}
	return buf.String()
}

//go:embed prompts/timeline/shrink_tool_result.txt
var timelineSummary string

func (m *memoryTimeline) renderSummaryPrompt(result *timelineItem) string {
	ins, err := template.New("timeline-tool-result").Parse(timelineSummary)
	if err != nil {
		log.Warnf("BUG: dump summary prompt failed: %v", err)
		return ""
	}
	var buf bytes.Buffer
	var nonce = strings.ToLower(utils.RandStringBytes(6))
	err = ins.Execute(&buf, map[string]any{
		"Memory": m.memory,
		"Input":  result.String(),
		"NONCE":  nonce,
	})
	if err != nil {
		log.Errorf("BUG: dump summary prompt failed: %v", err)
		return ""
	}
	return buf.String()
}

func (m *memoryTimeline) Dump() string {
	k, _, ok := m.idToTimelineItem.Last()
	if ok {
		return m.DumpBefore(k)
	}
	return ""
}

func (m *memoryTimeline) DumpBefore(id int64) string {
	buf := bytes.NewBuffer(nil)
	initOnce := sync.Once{}
	reducerOnce := sync.Once{}
	count := 0

	shrinkStartId, _, _ := m.summary.Last()
	reduceredStartId, _, _ := m.reducers.Last()
	m.tsToTimelineItem.ForEach(func(key int64, item *timelineItem) bool {
		initOnce.Do(func() {
			buf.WriteString("timeline:\n")
		})

		if item.GetID() > id {
			return true
		}

		t := time.Unix(0, key*int64(time.Millisecond))
		timeStr := t.Format(utils.DefaultTimeFormat3)

		if reduceredStartId > 0 {
			if item.GetID() == reduceredStartId {
				val, ok := m.reducers.Get(reduceredStartId)
				if ok {
					reducerOnce.Do(func() {
						buf.WriteString(fmt.Sprintf("├─...\n"))
						buf.WriteString(fmt.Sprintf("├─[%s] id: %v reducer-memory: %v\n", timeStr, item.GetID(), val.Value()))
					})
					return true
				}
			} else if item.GetID() < reduceredStartId {
				return true
			}
		}

		if shrinkStartId > 0 && item.GetID() <= shrinkStartId {
			val, ok := m.summary.Get(shrinkStartId)
			if ok && !val.Value().deleted {
				buf.WriteString(fmt.Sprintf("├─[%s] id: %v memory: %v\n", timeStr, item.GetID(), val.Value().GetShrinkResult()))
			}
			return true
		}

		if item.deleted {
			return true
		}

		buf.WriteString(fmt.Sprintf("├─[%s]\n", timeStr))
		raw := item.String()
		for _, line := range utils.ParseStringToRawLines(raw) {
			buf.WriteString(fmt.Sprintf("│    %s\n", line))
		}
		count++
		return true
	})
	if count > 0 {
		return buf.String()
	}

	buf.WriteString("no timeline generated in DumpBefore\n")
	return buf.String()
}

func (m *memoryTimeline) PromptForToolCallResultsForLastN(n int) string {
	if m.idToTimelineItem.Len() == 0 {
		return ""
	}

	var result = m.idToTimelineItem.Values()
	if len(result) > n {
		result = result[len(result)-n:]
	}
	templateData := map[string]interface{}{
		"ToolCallResults": result,
	}
	temp, err := template.New("tool-result-history").Parse(__prompt_ToolResultHistoryPromptTemplate)
	if err != nil {
		log.Errorf("error parsing tool result history template: %v", err)
		return ""
	}
	var promptBuilder strings.Builder
	err = temp.Execute(&promptBuilder, templateData)
	if err != nil {
		log.Errorf("error executing tool result history template: %v", err)
		return ""
	}
	return promptBuilder.String()
}
