package aid

import (
	"bytes"
	_ "embed"
	"fmt"
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

type summary struct {
	// summary of the tool call result
	summaryContent string
	meaningless    bool
}

type memoryTimeline struct {
	memory *Memory
	config *Config

	ai               AICaller
	maxTimelineLimit int
	fullMemoryCount  int
	idToTs           *omap.OrderedMap[int64, int64]
	tsToToolResult   *omap.OrderedMap[int64, *aitool.ToolResult]
	idToToolResult   *omap.OrderedMap[int64, *aitool.ToolResult]
	summary          *omap.OrderedMap[int64, *linktable.LinkTable[*summary]]
	reducers         *omap.OrderedMap[int64, *linktable.LinkTable[string]]
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
		if ret, ok := m.idToToolResult.Get(id); ok {
			tl.idToToolResult.Set(id, ret)
		}
		if ret, ok := m.tsToToolResult.Get(ts); ok {
			tl.tsToToolResult.Set(ts, ret)
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
	m.setTimelineLimit(config.timeLineLimit)
	if utils.IsNil(m.ai) {
		m.setAICaller(config)
	}
}

func newMemoryTimeline(clearCount int, ai AICaller) *memoryTimeline {
	return &memoryTimeline{
		ai:               ai,
		fullMemoryCount:  clearCount,
		maxTimelineLimit: 3 * clearCount,
		tsToToolResult:   omap.NewOrderedMap(map[int64]*aitool.ToolResult{}),
		idToToolResult:   omap.NewOrderedMap(map[int64]*aitool.ToolResult{}),
		idToTs:           omap.NewOrderedMap(map[int64]int64{}),
		summary:          omap.NewOrderedMap(map[int64]*linktable.LinkTable[*summary]{}),
		reducers:         omap.NewOrderedMap(map[int64]*linktable.LinkTable[string]{}),
	}
}

func (m *memoryTimeline) setTimelineLimit(clearCount int) {
	m.fullMemoryCount = clearCount
	m.maxTimelineLimit = 3 * clearCount
}

func (m *memoryTimeline) setAICaller(ai AICaller) {
	m.ai = ai
}

func (m *memoryTimeline) PushToolResult(toolResult *aitool.ToolResult) {
	ts := time.Now().UnixMilli()
	if m.tsToToolResult.Have(ts) {
		time.Sleep(time.Millisecond * 10)
		ts = time.Now().UnixMilli()
	}
	m.idToTs.Set(toolResult.GetID(), ts)
	m.tsToToolResult.Set(ts, toolResult)
	m.idToToolResult.Set(toolResult.GetID(), toolResult)
	total := m.idToToolResult.Len()
	summaryCount := m.summary.Len()
	if total-summaryCount > m.fullMemoryCount {
		shrinkTargetIndex := total - m.fullMemoryCount - 1
		id := m.idToToolResult.Index(shrinkTargetIndex)
		for _, v := range id.Values() {
			log.Infof("start to shrink memory timeline id: %v, total: %v, summary: %v, size: %v", v.GetID(), total, summaryCount, m.fullMemoryCount)
			m.shrink(v)
		}
	}

	if m.maxTimelineLimit > 0 && total-m.maxTimelineLimit > 0 {
		endIdx := total - m.maxTimelineLimit - 1
		val, ok := m.idToToolResult.GetByIndex(endIdx)
		if ok {
			log.Infof("start to reducer from id: %v, total: %v, limit: %v, delta: %v", val.GetID(), total, m.maxTimelineLimit, total-m.maxTimelineLimit)
			m.reducer(val.GetID())
		}
	}
}

func (m *memoryTimeline) reducer(beforeId int64) {
	if beforeId <= 0 {
		return
	}
	pmt := m.renderReducerPrompt(beforeId)
	if utils.IsNil(m.ai) {
		return
	}
	response, err := m.ai.callAI(NewAIRequest(pmt))
	if err != nil {
		log.Errorf("reducer call ai failed: %v", err)
		return
	}
	var r io.Reader
	if m.config == nil {
		r = response.GetUnboundStreamReader(false)
	} else {
		r = response.GetOutputStreamReader("memory-reducer", true, m.config)
	}
	output, err := io.ReadAll(r)
	if err != nil {
		log.Errorf("read ai output failed: %v", err)
		return
	}
	action, err := ExtractAction(string(output), "timeline-reducer")
	if err != nil {
		log.Errorf("extract timeline action failed: %v", err)
		return
	}
	pers := action.GetString("reducer_memory")
	if pers != "" {
		if lt, ok := m.reducers.Get(beforeId); ok {
			lt.Push(pers)
		} else {
			m.reducers.Set(beforeId, linktable.NewUnlimitedStringLinkTable(pers))
		}
	}
}

func (m *memoryTimeline) shrink(result *aitool.ToolResult) {
	if m.ai == nil {
		log.Error("ai is nil, memory cannot emit memory shrink")
		return
	}

	response, err := m.ai.callAI(NewAIRequest(m.renderSummaryPrompt(result)))
	if err != nil {
		log.Errorf("shrink call ai failed: %v", err)
		return
	}
	var r io.Reader
	if m.config == nil {
		r = response.GetUnboundStreamReader(false)
	} else {
		r = response.GetOutputStreamReader("memory-timeline", true, m.config)
	}
	output, err := io.ReadAll(r)
	if err != nil {
		log.Errorf("read ai output failed: %v", err)
		return
	}
	action, err := ExtractAction(string(output), "timeline-shrink")
	if err != nil {
		log.Errorf("extract timeline action failed: %v", err)
		return
	}
	pers := action.GetString("persistent")
	if pers == "" {
		s, ok := m.summary.Get(result.GetID())
		if ok {
			pers = s.Value().summaryContent
		}
	}

	s := &summary{
		summaryContent: pers,
		meaningless:    action.GetBool("should_drop"),
	}
	if lt, ok := m.summary.Get(result.GetID()); ok {
		lt.Push(s)
	} else {
		m.summary.Set(result.GetID(), linktable.NewUnlimitedLinkTable(s))
	}
}

//go:embed prompts/timeline/reducer_memory.txt
var timelineReducer string

func (m *memoryTimeline) renderReducerPrompt(beforeId int64) string {
	input := m.DumpBefore(beforeId)
	ins, err := template.New("timeline-reducer").Parse(timelineReducer)
	if err != nil {
		log.Warnf("BUG: dump summary prompt failed: %v", err)
		return ""
	}
	var buf bytes.Buffer
	err = ins.Execute(&buf, map[string]any{
		"Memory": m.memory,
		"Input":  input,
	})
	if err != nil {
		log.Errorf("BUG: dump summary prompt failed: %v", err)
		return ""
	}
	return buf.String()
}

//go:embed prompts/timeline/shrink_tool_result.txt
var timelineSummary string

func (m *memoryTimeline) renderSummaryPrompt(result *aitool.ToolResult) string {
	ins, err := template.New("timeline-tool-result").Parse(timelineSummary)
	if err != nil {
		log.Warnf("BUG: dump summary prompt failed: %v", err)
		return ""
	}
	var buf bytes.Buffer
	err = ins.Execute(&buf, map[string]any{
		"Memory": m.memory,
		"Input":  result.String(),
	})
	if err != nil {
		log.Errorf("BUG: dump summary prompt failed: %v", err)
		return ""
	}
	return buf.String()
}

func (m *memoryTimeline) Dump() string {
	k, _, ok := m.idToToolResult.Last()
	if ok {
		return m.DumpBefore(k)
	}
	return "no timeline generated in Dump"
}

func (m *memoryTimeline) DumpBefore(id int64) string {
	buf := bytes.NewBuffer(nil)
	initOnce := sync.Once{}
	reducerOnce := sync.Once{}
	count := 0

	shrinkStartId, _, _ := m.summary.Last()
	reduceredStartId, _, _ := m.reducers.Last()
	m.tsToToolResult.ForEach(func(key int64, value *aitool.ToolResult) bool {
		initOnce.Do(func() {
			buf.WriteString("timeline:\n")
		})
		if value.GetID() > id {
			return true
		}

		t := time.Unix(0, key*int64(time.Millisecond))
		timeStr := t.Format(utils.DefaultTimeFormat3)

		if reduceredStartId > 0 {
			if value.GetID() == reduceredStartId {
				val, ok := m.reducers.Get(reduceredStartId)
				if ok {
					reducerOnce.Do(func() {
						buf.WriteString(fmt.Sprintf("├─...\n"))
						buf.WriteString(fmt.Sprintf("├─[%s] id: %v reducer-memory: %v\n", timeStr, value.GetID(), val.Value()))
					})
					return true
				}
			} else if value.GetID() < reduceredStartId {
				return true
			}
		}

		if shrinkStartId > 0 && value.GetID() <= shrinkStartId {
			val, ok := m.summary.Get(shrinkStartId)
			if ok && !val.Value().meaningless {
				buf.WriteString(fmt.Sprintf("├─[%s] id: %v memory: %v\n", timeStr, value.GetID(), val.Value().summaryContent))
			}
			return true
		}
		buf.WriteString(fmt.Sprintf("├─[%s]\n", timeStr))
		raw := value.String()
		for _, line := range utils.ParseStringToLines(raw) {
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
	if m.idToToolResult.Len() == 0 {
		return ""
	}

	var result = m.idToToolResult.Values()
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
