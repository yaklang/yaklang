package aicommon

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/linktable"
	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type TimelineItemValue interface {
	String() string
	GetID() int64
	GetShrinkResult() string
	GetShrinkSimilarResult() string
	SetShrinkResult(string)
}

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

type Timeline struct {
	extraMetaInfo func() string // extra meta info for timeline, like runtime id, etc.
	config        AICallerConfigIf
	ai            AICaller

	idToTs           *omap.OrderedMap[int64, int64]
	tsToTimelineItem *omap.OrderedMap[int64, *timelineItem]
	idToTimelineItem *omap.OrderedMap[int64, *timelineItem]
	summary          *omap.OrderedMap[int64, *linktable.LinkTable[*timelineItem]]
	reducers         *omap.OrderedMap[int64, *linktable.LinkTable[string]]

	// this limit is used to limit the number of timeline items.
	maxTimelineLimit int64 // total timeline item count
	fullMemoryCount  int64 // full memory timeline item count

	// this limit is used to limit the timeline dump string size.
	perDumpContentLimit   int64
	totalDumpContentLimit int64
}

func (m *Timeline) CopyReducibleTimelineWithMemory() *Timeline {
	tl := &Timeline{
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

func (m *Timeline) SoftDelete(id ...int64) {
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

func (m *Timeline) CreateSubTimeline(ids ...int64) *Timeline {
	tl := NewTimeline(m.fullMemoryCount, m.ai, m.extraMetaInfo)
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

func (m *Timeline) BindConfig(config AICallerConfigIf, aiCaller AICaller) {
	m.config = config
	m.setTimelineLimit(config.GetTimelineRecordLimit())
	m.setTimelineContentLimit(config.GetTimelineContentSizeLimit())
	if utils.IsNil(m.ai) {
		m.setAICaller(aiCaller)
	}
}

func NewTimeline(clearCount int64, ai AICaller, extraMetaInfo func() string) *Timeline {
	return &Timeline{
		extraMetaInfo:    extraMetaInfo,
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

func (m *Timeline) ExtraMetaInfo() string {
	if m.extraMetaInfo == nil {
		return ""
	}
	return m.extraMetaInfo()
}

func (m *Timeline) setTimelineLimit(clearCount int64) {
	m.fullMemoryCount = clearCount
	m.maxTimelineLimit = 3 * clearCount
}

func (m *Timeline) setTimelineContentLimit(contentSize int64) {
	m.totalDumpContentLimit = contentSize
}

func (m *Timeline) setAICaller(ai AICaller) {
	m.ai = ai
}

func (m *Timeline) PushToolResult(toolResult *aitool.ToolResult) {
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
	if m.perDumpContentLimit > 0 && int64(len(item.String())) > m.perDumpContentLimit {
		m.shrink(item)
	}

	m.tsToTimelineItem.Set(ts, item)
	m.idToTimelineItem.Set(toolResult.GetID(), item)
	m.timelineLengthCheck()
	m.dumpSizeCheck()
}

func (m *Timeline) PushUserInteraction(stage UserInteractionStage, id int64, systemPrompt string, userExtraPrompt string) {
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

	if m.perDumpContentLimit > 0 && int64(len(item.String())) > m.perDumpContentLimit {
		m.shrink(item)
	}

	m.tsToTimelineItem.Set(ts, item)
	m.idToTimelineItem.Set(id, item)
	m.timelineLengthCheck()
	m.dumpSizeCheck()
}

func (m *Timeline) timelineLengthCheck() {
	total := int64(m.idToTimelineItem.Len())
	summaryCount := int64(m.summary.Len())
	if total-summaryCount > m.fullMemoryCount {
		shrinkTargetIndex := total - m.fullMemoryCount - 1
		id := m.idToTimelineItem.Index(int(shrinkTargetIndex))
		for _, v := range id.Values() {
			log.Infof("start to shrink memory timeline id: %v, total: %v, summary: %v, size: %v", v.value.GetID(), total, summaryCount, m.fullMemoryCount)
			m.shrink(v)
		}
	}

	if m.maxTimelineLimit > 0 && total-m.maxTimelineLimit > 0 {
		endIdx := total - m.maxTimelineLimit - 1
		rawValue, ok := m.idToTimelineItem.GetByIndex(int(endIdx))
		if ok {
			val := rawValue.value
			log.Infof("start to reducer from id: %v, total: %v, limit: %v, delta: %v", val.GetID(), total, m.maxTimelineLimit, total-m.maxTimelineLimit)
			m.reducer(val.GetID())
		}
	}
}

func (m *Timeline) dumpSizeCheck() {
	if m.ai == nil {
		log.Error("ai is nil, memory cannot emit memory shrink")
		return
	}

	if m.totalDumpContentLimit <= 0 || int64(len(m.Dump())) <= m.totalDumpContentLimit {
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

func (m *Timeline) reducer(beforeId int64) {
	if beforeId <= 0 {
		return
	}
	pmt := m.renderReducerPrompt(beforeId)
	if utils.IsNil(m.ai) {
		return
	}

	if m.config == nil {
		err := CallAITransaction(nil, pmt, m.ai.CallAI, func(response *AIResponse) error {
			action, err := ExtractActionFromStream(response.GetUnboundStreamReader(false), "timeline-reducer")
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
		if err != nil {
			log.Errorf("call ai transaction failed in memory reducer: %v", err)
			return
		}
	} else {
		err := CallAITransaction(m.config, pmt, m.ai.CallAI, func(response *AIResponse) error {
			action, err := ExtractActionFromStream(
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
		if err != nil {
			log.Errorf("call ai transaction failed in memory reducer: %v", err)
			return
		}
	}
}

func (m *Timeline) shrink(currentItem *timelineItem) {
	if m.ai == nil {
		log.Error("ai is nil, memory cannot emit memory shrink")
		return
	}

	response, err := m.ai.CallAI(NewAIRequest(m.renderSummaryPrompt(currentItem)))
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
	action, err := ExtractActionFromStream(r, "timeline-shrink")
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

func (m *Timeline) renderReducerPrompt(beforeId int64) string {
	input := m.DumpBefore(beforeId)
	ins, err := template.New("timeline-reducer").Parse(timelineReducer)
	if err != nil {
		log.Errorf("BUG: dump summary prompt failed: %v", err)
		return ""
	}
	var buf bytes.Buffer
	var nonce = utils.RandStringBytes(6)
	err = ins.Execute(&buf, map[string]any{
		"Timeline":      m.Dump(),
		"ExtraMetaInfo": m.ExtraMetaInfo(),
		"Input":         input,
		`NONCE`:         nonce,
	})
	if err != nil {
		log.Errorf("BUG: dump summary prompt failed: %v", err)
		return ""
	}
	return buf.String()
}

//go:embed prompts/timeline/shrink_tool_result.txt
var timelineSummary string

func (m *Timeline) renderSummaryPrompt(result *timelineItem) string {
	ins, err := template.New("timeline-tool-result").Parse(timelineSummary)
	if err != nil {
		log.Warnf("BUG: dump summary prompt failed: %v", err)
		return ""
	}
	var buf bytes.Buffer
	var nonce = strings.ToLower(utils.RandStringBytes(6))
	err = ins.Execute(&buf, map[string]any{
		"ExtraMetaInfo": m.ExtraMetaInfo(),
		"Timeline":      m.Dump(),
		"Input":         result.String(),
		"NONCE":         nonce,
	})
	if err != nil {
		log.Errorf("BUG: dump summary prompt failed: %v", err)
		return ""
	}
	return buf.String()
}

func (m *Timeline) Dump() string {
	k, _, ok := m.idToTimelineItem.Last()
	if ok {
		return m.DumpBefore(k)
	}
	return ""
}

func (m *Timeline) DumpBefore(id int64) string {
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

//go:embed prompts/timeline/tool_result_history.txt
var toolResultHistory string

func (m *Timeline) PromptForToolCallResultsForLastN(n int) string {
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
	temp, err := template.New("tool-result-history").Parse(toolResultHistory)
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

var _ TimelineItemValue = (*UserInteraction)(nil)

type UserInteractionStage string

const (
	UserInteractionStage_BeforePlan UserInteractionStage = "before_plan"
	UserInteractionStage_Review     UserInteractionStage = "review"
	UserInteractionStage_FreeInput  UserInteractionStage = "free_input"
)

type UserInteraction struct {
	ID              int64                `json:"id"`
	SystemPrompt    string               `json:"prompt"`
	UserExtraPrompt string               `json:"extra_prompt"`
	Stage           UserInteractionStage `json:"stage"` // Stage
	ShrinkResult    string               `json:"shrink_result,omitempty"`
}

func (u *UserInteraction) String() string {
	if u.Stage == "" {
		u.Stage = UserInteractionStage_FreeInput
	}
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf(" <- [id:%v] when %v\n", u.ID, u.Stage))
	buf.WriteString("   system-question: " + u.SystemPrompt + "\n")
	buf.WriteString("       user-answer: " + u.UserExtraPrompt + "\n")
	return buf.String()
}

func (u *UserInteraction) GetID() int64 {
	return u.ID
}

func (u *UserInteraction) GetShrinkResult() string {
	return u.ShrinkResult
}

func (u *UserInteraction) GetShrinkSimilarResult() string {
	if u.ShrinkResult != "" {
		return u.ShrinkResult
	}
	return ""
}

func (u *UserInteraction) SetShrinkResult(s string) {
	u.ShrinkResult = s
}
