package aid

import (
	"bytes"
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"io"
	"sync"
	"text/template"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/linktable"
	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type memoryTimeline struct {
	memory *Memory
	config *Config

	ai               AICaller
	maxTimelineLimit int
	fullMemoryCount  int
	Timestamp        []int64
	tsToToolResult   *omap.OrderedMap[int64, *aitool.ToolResult]
	idToToolResult   *omap.OrderedMap[int64, *aitool.ToolResult]

	summary  *omap.OrderedMap[int64, *linktable.LinkTable[string]]
	reducers *omap.OrderedMap[int64, *linktable.LinkTable[string]]
}

func (m *memoryTimeline) BindConfig(config *Config) {
	m.config = config
	m.memory = config.memory
}

func newMemoryTimeline(clearCount int, ai AICaller) *memoryTimeline {
	return &memoryTimeline{
		ai:               ai,
		fullMemoryCount:  clearCount,
		maxTimelineLimit: 3 * clearCount,
		Timestamp:        []int64{},
		tsToToolResult:   omap.NewOrderedMap(map[int64]*aitool.ToolResult{}),
		idToToolResult:   omap.NewOrderedMap(map[int64]*aitool.ToolResult{}),

		summary:  omap.NewOrderedMap(map[int64]*linktable.LinkTable[string]{}),
		reducers: omap.NewOrderedMap(map[int64]*linktable.LinkTable[string]{}),
	}
}

func (m *memoryTimeline) PushToolResult(toolResult *aitool.ToolResult) {
	ts := time.Now().UnixMilli()
	if m.tsToToolResult.Have(ts) {
		time.Sleep(time.Millisecond * 100)
		ts = time.Now().UnixMilli()
	}
	m.tsToToolResult.Set(ts, toolResult)
	m.idToToolResult.Set(toolResult.GetID(), toolResult)
	m.Timestamp = append(m.Timestamp, ts)
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
		log.Errorf("call ai failed: %v", err)
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
		log.Errorf("call ai failed: %v", err)
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
	if pers != "" {
		if lt, ok := m.summary.Get(result.GetID()); ok {
			lt.Push(pers)
		} else {
			m.summary.Set(result.GetID(), linktable.NewUnlimitedStringLinkTable(pers))
		}
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
	k, _, _ := m.idToToolResult.Last()
	if k > 0 {
		return m.DumpBefore(k)
	}
	return "no timeline"
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
			if ok {
				buf.WriteString(fmt.Sprintf("├─[%s] id: %v memory: %v\n", timeStr, value.GetID(), val.Value()))
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

	buf.WriteString("no timeline\n")
	return buf.String()
}
