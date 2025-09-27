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

type Timeline struct {
	extraMetaInfo func() string // extra meta info for timeline, like runtime id, etc.
	config        AICallerConfigIf
	ai            AICaller

	idToTs           *omap.OrderedMap[int64, int64]
	tsToTimelineItem *omap.OrderedMap[int64, *TimelineItem]
	idToTimelineItem *omap.OrderedMap[int64, *TimelineItem]
	summary          *omap.OrderedMap[int64, *linktable.LinkTable[*TimelineItem]]
	reducers         *omap.OrderedMap[int64, *linktable.LinkTable[string]]

	// this limit is used to limit the number of timeline items.
	maxTimelineLimit int64 // total timeline item count
	fullMemoryCount  int64 // full memory timeline item count

	// this limit is used to limit the timeline dump string size.
	perDumpContentLimit   int64
	totalDumpContentLimit int64
}

func (m *Timeline) GetIdToTimelineItem() *omap.OrderedMap[int64, *TimelineItem] {
	return m.idToTimelineItem
}

func (m *Timeline) GetTimelineItemIDs() []int64 {
	return m.idToTimelineItem.Keys()
}

func (m *Timeline) ClearRuntimeConfig() {
	m.ai = nil
	m.config = nil
}

func (m *Timeline) SetAICaller(ai AICaller) {
	if ai == nil {
		log.Error("set ai caller is nil")
		return
	}
	m.ai = ai
}

func (m *Timeline) GetAICaller() AICaller {
	if m.ai == nil {
		return nil
	}
	return m.ai
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
			v.Push(&TimelineItem{
				createdAt: v.Value().createdAt,
				deleted:   true,
				value:     v.Value().value,
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
	m.SetTimelineLimit(config.GetTimelineRecordLimit())
	m.SetTimelineContentLimit(config.GetTimelineContentSizeLimit())
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
		tsToTimelineItem: omap.NewOrderedMap(map[int64]*TimelineItem{}),
		idToTimelineItem: omap.NewOrderedMap(map[int64]*TimelineItem{}),
		idToTs:           omap.NewOrderedMap(map[int64]int64{}),
		summary:          omap.NewOrderedMap(map[int64]*linktable.LinkTable[*TimelineItem]{}),
		reducers:         omap.NewOrderedMap(map[int64]*linktable.LinkTable[string]{}),
	}
}

func (m *Timeline) ExtraMetaInfo() string {
	if m.extraMetaInfo == nil {
		return ""
	}
	return m.extraMetaInfo()
}

func (m *Timeline) SetTimelineLimit(clearCount int64) {
	m.fullMemoryCount = clearCount
	m.maxTimelineLimit = 3 * clearCount
}

func (m *Timeline) SetTimelineContentLimit(contentSize int64) {
	m.totalDumpContentLimit = contentSize
}

func (m *Timeline) setAICaller(ai AICaller) {
	m.ai = ai
}

func (m *Timeline) PushToolResult(toolResult *aitool.ToolResult) {
	now := time.Now()
	ts := now.UnixMilli()
	if m.tsToTimelineItem.Have(ts) {
		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		ts = now.UnixMilli()
	}
	id := toolResult.GetID()
	if id <= 0 {
		log.Warnf("push tool result to timeline but id is invalid, id: %v", id)
		return
	}
	if m.idToTs.Have(id) {
		log.Warnf("push tool result to timeline but id already exist, id: %v", id)
	}
	m.idToTs.Set(id, ts)

	item := &TimelineItem{
		createdAt: now,
		value:     toolResult,
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
	now := time.Now()
	ts := now.UnixMilli()
	if m.tsToTimelineItem.Have(ts) {
		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		ts = now.UnixMilli()
	}
	m.idToTs.Set(id, ts)

	item := &TimelineItem{
		createdAt: now,
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

	// 当 timeline 达到 100 个 items 时，触发批量压缩
	const batchCompressThreshold = 100
	if total >= batchCompressThreshold {
		halfCount := total / 2
		log.Infof("start to batch compress memory timeline, total: %v, compress first half: %v items", total, halfCount)
		m.batchCompress(int(halfCount))
		return
	}

	// 保留原有的 reducer 逻辑，用于处理超过 maxTimelineLimit 的情况
	if m.maxTimelineLimit > 0 && total > m.maxTimelineLimit {
		endIdx := total - m.maxTimelineLimit - 1
		rawValue, ok := m.idToTimelineItem.GetByIndex(int(endIdx))
		if ok {
			val := rawValue.value
			log.Infof("start to reducer from id: %v, total: %v, limit: %v, delta: %v", val.GetID(), total, m.maxTimelineLimit, total-m.maxTimelineLimit)
			m.reducer(val.GetID())
		}
	}
}

func (m *Timeline) batchCompress(compressCount int) {
	if compressCount <= 0 || m.ai == nil {
		return
	}

	// 获取前 compressCount 个 items 进行压缩
	var itemsToCompress []*TimelineItem
	var idsToRemove []int64

	count := 0
	m.idToTimelineItem.ForEach(func(id int64, item *TimelineItem) bool {
		if count >= compressCount {
			return false
		}
		itemsToCompress = append(itemsToCompress, item)
		idsToRemove = append(idsToRemove, id)
		count++
		return true
	})

	if len(itemsToCompress) == 0 {
		return
	}

	// 生成压缩提示
	prompt := m.renderBatchCompressPrompt(itemsToCompress)
	if prompt == "" {
		return
	}

	// 调用 AI 进行批量压缩
	response, err := m.ai.CallAI(NewAIRequest(prompt))
	if err != nil {
		log.Errorf("batch compress call ai failed: %v", err)
		return
	}

	var r io.Reader
	if m.config == nil {
		r = response.GetUnboundStreamReader(false)
	} else {
		r = response.GetOutputStreamReader("batch-compress", true, m.config.GetEmitter())
	}

	action, err := ExtractActionFromStream(r, "timeline-reducer")
	if err != nil {
		log.Errorf("extract timeline batch compress action failed: %v", err)
		return
	}

	compressedMemory := action.GetString("reducer_memory")
	if compressedMemory == "" {
		log.Warnf("batch compress got empty compressed memory")
		return
	}

	// 存储压缩结果
	lastCompressedId := idsToRemove[len(idsToRemove)-1]
	if lt, ok := m.reducers.Get(lastCompressedId); ok {
		lt.Push(compressedMemory)
	} else {
		m.reducers.Set(lastCompressedId, linktable.NewUnlimitedStringLinkTable(compressedMemory))
	}
	log.Infof("batch compressed %d items into reducer at id: %v", len(itemsToCompress), lastCompressedId)

	// 删除被压缩的 items
	for _, id := range idsToRemove {
		m.idToTimelineItem.Delete(id)
		if ts, ok := m.idToTs.Get(id); ok {
			m.tsToTimelineItem.Delete(ts)
			m.idToTs.Delete(id)
		}
	}
}

func (m *Timeline) dumpSizeCheck() {
	// Since we removed shrink mechanism and use batch compression instead,
	// this function now only handles content size by triggering batch compression if needed
	total := int64(m.idToTimelineItem.Len())

	// If content is too large and we have many items, trigger batch compression
	if m.totalDumpContentLimit > 0 && int64(len(m.Dump())) > m.totalDumpContentLimit && total >= 50 {
		log.Infof("dump size check: content too large, triggering batch compression")
		halfCount := total / 2
		m.batchCompress(int(halfCount))
	}
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

func (m *Timeline) shrink(currentItem *TimelineItem) {
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

//go:embed prompts/timeline/batch_compress.txt
var timelineBatchCompress string

func (m *Timeline) renderBatchCompressPrompt(items []*TimelineItem) string {
	if len(items) == 0 {
		return ""
	}

	ins, err := template.New("timeline-batch-compress").Parse(timelineBatchCompress)
	if err != nil {
		log.Errorf("BUG: batch compress prompt template failed: %v", err)
		return ""
	}

	var buf bytes.Buffer
	var nonce = utils.RandStringBytes(6)

	// 构建要压缩的 items 字符串
	var itemsStr strings.Builder
	for i, item := range items {
		if i > 0 {
			itemsStr.WriteString("\n")
		}
		itemsStr.WriteString(fmt.Sprintf("[%d] %s", i+1, item.String()))
	}

	err = ins.Execute(&buf, map[string]any{
		"ExtraMetaInfo":   m.ExtraMetaInfo(),
		"ItemsToCompress": itemsStr.String(),
		"ItemCount":       len(items),
		"NONCE":           nonce,
	})
	if err != nil {
		log.Errorf("BUG: batch compress prompt execution failed: %v", err)
		return ""
	}
	return buf.String()
}

//go:embed prompts/timeline/shrink_tool_result.txt
var timelineSummary string

func (m *Timeline) renderSummaryPrompt(result *TimelineItem) string {
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

func (m *Timeline) DumpBefore(beforeId int64) string {
	buf := bytes.NewBuffer(nil)
	initOnce := sync.Once{}
	count := 0

	shrinkStartId, _, _ := m.summary.Last()
	reduceredStartId, _, _ := m.reducers.Last()

	// If we have reducers, show them first
	if reduceredStartId > 0 {
		val, ok := m.reducers.Get(reduceredStartId)
		if ok {
			initOnce.Do(func() {
				buf.WriteString("timeline:\n")
			})
			buf.WriteString(fmt.Sprint("  ...\n"))
			// Use a fixed timestamp for reducer display
			reducerTimeStr := time.Now().Format(utils.DefaultTimeFormat3)
			buf.WriteString(fmt.Sprintf("--[%s] id: %v reducer-memory: %v\n", reducerTimeStr, reduceredStartId, val.Value()))
		}
	}

	m.idToTimelineItem.ForEach(func(id int64, item *TimelineItem) bool {
		initOnce.Do(func() {
			buf.WriteString("timeline:\n")
		})

		if item.GetID() > beforeId {
			return true
		}

		ts, ok := m.idToTs.Get(item.GetID())
		if !ok {
			log.Warnf("BUG: timeline id %v not found", item.GetID())
		}
		t := time.Unix(0, ts*int64(time.Millisecond))
		timeStr := t.Format(utils.DefaultTimeFormat3)

		if shrinkStartId > 0 && item.GetID() <= shrinkStartId {
			val, ok := m.summary.Get(shrinkStartId)
			if ok && !val.Value().deleted {
				//buf.WriteString(fmt.Sprintf("├─[%s] id: %v memory: %v\n", timeStr, item.GetID(), val.Value().GetShrinkResult()))
				buf.WriteString(fmt.Sprintf("--[%s] id: %v memory: %v\n", timeStr, item.GetID(), val.Value().GetShrinkResult()))
			}
			return true
		}

		if item.deleted {
			return true
		}

		//buf.WriteString(fmt.Sprintf("├─[%s]\n", timeStr))
		buf.WriteString(fmt.Sprintf("--[%s]\n", timeStr))
		raw := item.String()
		for _, line := range utils.ParseStringToRawLines(raw) {
			//buf.WriteString(fmt.Sprintf("│    %s\n", line))
			buf.WriteString(fmt.Sprintf("     %s\n", line))
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

func (m *Timeline) PushText(id int64, fmtText string, items ...any) {
	now := time.Now()
	ts := now.UnixMilli()
	if m.tsToTimelineItem.Have(ts) {
		time.Sleep(time.Millisecond * 10)
		now = time.Now()
		ts = now.UnixMilli()
	}
	m.idToTs.Set(id, ts)

	var result string
	if len(items) > 0 {
		result = fmt.Sprintf(fmtText, items...)
	} else {
		result = fmtText
	}

	item := &TimelineItem{
		createdAt: now,
		value: &TextTimelineItem{
			ID:   id,
			Text: result,
		},
	}

	if m.perDumpContentLimit > 0 && int64(len(fmtText)) > m.perDumpContentLimit {
		m.shrink(item)
	}

	m.tsToTimelineItem.Set(ts, item)
	m.idToTimelineItem.Set(id, item)
	m.timelineLengthCheck()
	m.dumpSizeCheck()
}

// TimelineEntry 时间线条目
type TimelineItemOutput struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"` // "input", "thought", "action", "observation", "result"
	Content   string    `json:"content"`
}

func (m *TimelineItemOutput) String() string {
	return fmt.Sprintf("[%v][%s] %s", m.Timestamp, m.Type, m.Content)
}

func (m *Timeline) GetTimelineOutput() []*TimelineItemOutput {
	l := m.idToTimelineItem.Len()
	if l == 0 {
		return nil
	}
	return m.ToTimelineItemOutputLastN(l)
}

func (m *Timeline) ToTimelineItemOutputLastN(n int) []*TimelineItemOutput {
	l := m.tsToTimelineItem.Len()
	if l == 0 {
		return nil
	}

	var result []*TimelineItemOutput
	start := l - n
	if start < 0 {
		start = 0
	}

	for i := start; i < l; i++ {
		item, ok := m.tsToTimelineItem.GetByIndex(i)
		if !ok {
			continue
		}
		result = append(result, item.ToTimelineItemOutput())
	}

	return result
}
