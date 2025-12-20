package aicommon

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

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

	// this limit is used to limit the timeline dump string size.
	perDumpContentLimit   int64
	totalDumpContentLimit int64

	compressing *utils.Once
}

// MaxTimelineSaveSize is the maximum size (100KB) for timeline data when saving to database
const MaxTimelineSaveSize = 100 * 1024

func (m *Timeline) Save(db *gorm.DB, persistentId string) {
	if utils.IsNil(m) {
		log.Warnf("try to save nil timeline for persistentId: %v", persistentId)
		return
	}

	// Check and emergency compress if timeline is too large before saving
	tlstr, err := MarshalTimeline(m)
	if err != nil {
		log.Warnf("save(/marshal) timeline failed: %v", err)
		return
	}

	// If timeline is too large, perform emergency compression
	if len(tlstr) > MaxTimelineSaveSize {
		log.Warnf("timeline size %d exceeds max save size %d, performing emergency compression before save", len(tlstr), MaxTimelineSaveSize)
		m.emergencyCompress(MaxTimelineSaveSize)

		// Re-marshal after emergency compression
		tlstr, err = MarshalTimeline(m)
		if err != nil {
			log.Warnf("save(/marshal) timeline after emergency compress failed: %v", err)
			return
		}

		// If still too large after emergency compression, truncate and log warning
		if len(tlstr) > MaxTimelineSaveSize {
			log.Warnf("timeline still too large (%d) after emergency compression, will save truncated version", len(tlstr))
		}
	}

	result := strconv.Quote(tlstr)
	if err := yakit.UpdateAIAgentRuntimeTimelineWithPersistentId(db, persistentId, result); err != nil {
		log.Errorf("ReAct: save timeline to db failed: %v", err)
		return
	}
}

func (m *Timeline) Valid() bool {
	if m == nil {
		return false
	}
	if m.config == nil {
		return false
	}
	if m.ai == nil {
		return false
	}
	return true
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
		perDumpContentLimit:   m.perDumpContentLimit,
		totalDumpContentLimit: m.totalDumpContentLimit,
		compressing:           utils.NewOnce(),
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
	tl := NewTimeline(m.ai, m.extraMetaInfo)
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
	m.SetTimelineContentLimit(config.GetTimelineContentSizeLimit())
	if utils.IsNil(m.ai) {
		m.setAICaller(aiCaller)
	}
}

func NewTimeline(ai AICaller, extraMetaInfo func() string) *Timeline {
	return &Timeline{
		extraMetaInfo:    extraMetaInfo,
		ai:               ai,
		tsToTimelineItem: omap.NewOrderedMap(map[int64]*TimelineItem{}),
		idToTimelineItem: omap.NewOrderedMap(map[int64]*TimelineItem{}),
		idToTs:           omap.NewOrderedMap(map[int64]int64{}),
		summary:          omap.NewOrderedMap(map[int64]*linktable.LinkTable[*TimelineItem]{}),
		reducers:         omap.NewOrderedMap(map[int64]*linktable.LinkTable[string]{}),
		compressing:      utils.NewOnce(),
	}
}

func (m *Timeline) ExtraMetaInfo() string {
	if m.extraMetaInfo == nil {
		return ""
	}
	return m.extraMetaInfo()
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

	m.pushTimelineItem(ts, toolResult.GetID(), item)
}

func (m *Timeline) pushTimelineItem(ts int64, id int64, item *TimelineItem) {
	m.tsToTimelineItem.Set(ts, item)
	m.idToTimelineItem.Set(id, item)
	m.dumpSizeCheck()

	// Emit timeline item asynchronously to avoid blocking when EventHandler
	// writes to an unbuffered channel that hasn't been consumed yet
	if m.config != nil && m.config.GetEmitter() != nil {
		go m.config.GetEmitter().EmitTimelineItem(item)
	}
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

	m.pushTimelineItem(ts, id, item)
}

// findCompressCountForTargetSize 使用二分法找到需要压缩的项目数量，使得剩余项目数约为 targetSize
func (m *Timeline) findCompressCountForTargetSize(targetSize int) int {
	total := int64(m.idToTimelineItem.Len())
	if total <= int64(targetSize) {
		return 0 // 已经达到或小于目标大小，不需要压缩
	}

	// 使用二分法找到合适的压缩数量
	left, right := 0, int(total-1)

	for left < right {
		mid := (left + right) / 2
		remainingSize := int(total) - mid

		if remainingSize <= targetSize {
			// 剩余大小小于等于目标大小，压缩太多了，需要减少压缩数量
			right = mid
		} else {
			// 剩余大小大于目标大小，需要增加压缩数量
			left = mid + 1
		}
	}

	compressCount := left
	if compressCount < 0 {
		compressCount = 0
	}
	if compressCount > int(total)-1 {
		compressCount = int(total) - 1
	}

	return compressCount
}

func (m *Timeline) batchCompressByTargetSize(targetSize int) {
	if targetSize <= 0 {
		return
	}

	// If AI is nil, use emergency compress instead
	if m.ai == nil {
		log.Warnf("batch compress: AI is nil, using emergency compress")
		m.emergencyCompress(MaxTimelineSaveSize)
		return
	}

	total := int64(m.idToTimelineItem.Len())
	if total <= 1 {
		return
	}

	// Check if current timeline is already too large for AI processing
	// If so, do emergency compress first to bring it to a manageable size
	tlstr, err := MarshalTimeline(m)
	if err == nil && len(tlstr) > MaxTimelineSaveSize*2 {
		log.Warnf("batch compress: timeline too large (%d), performing emergency compress first", len(tlstr))
		m.emergencyCompress(MaxTimelineSaveSize)
		// Recalculate total after emergency compress
		total = int64(m.idToTimelineItem.Len())
		if total <= 1 {
			return
		}
	}

	// 使用二分法找到需要压缩的项目数量，使得压缩后大小约为 targetSize
	compressCount := m.findCompressCountForTargetSize(targetSize)
	if compressCount <= 0 {
		return
	}

	log.Infof("batch compress: found compress count %d for target size %d", compressCount, targetSize)

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
	nonceStr := utils.RandStringBytes(4)
	prompt := m.renderBatchCompressPrompt(itemsToCompress, nonceStr)
	if prompt == "" {
		// If prompt is empty, fall back to emergency compress
		log.Warnf("batch compress: prompt is empty, falling back to emergency compress")
		m.emergencyCompress(MaxTimelineSaveSize)
		return
	}

	// 调用 AI 进行批量压缩
	var action *Action
	var cumulativeSummary string
	err = CallAITransaction(m.config, prompt, m.ai.CallAI, func(response *AIResponse) error {
		var callErr error
		response, callErr = m.ai.CallAI(NewAIRequest(prompt))
		if callErr != nil {
			log.Errorf("batch compress call ai failed: %v", callErr)
			return utils.Errorf("context-shrink call ai failed: %v", callErr)
		}

		var r io.Reader
		if m.config == nil {
			r = response.GetUnboundStreamReader(false)
		} else {
			r = response.GetOutputStreamReader("batch-compress", true, m.config.GetEmitter())
		}

		var extractErr error
		action, extractErr = ExtractActionFromStream(
			m.config.GetContext(),
			r, "timeline-reducer",
			WithActionTagToKey("REDUCER_MEMORY", "reducer_memory"),
			WithActionNonce(nonceStr),
			WithActionFieldStreamHandler(
				[]string{"reducer_memory"},
				func(key string, reader io.Reader) {
					var out bytes.Buffer
					reducerMem := io.TeeReader(utils.JSONStringReader(reader), &out)
					m.config.GetEmitter().EmitSystemStreamEvent(
						"memory-timeline",
						time.Now(),
						reducerMem,
						response.GetTaskIndex(),
						func() {
							log.Infof("memory-timeline shrink result: %v", out.String())
						},
					)
				}),
		)
		if extractErr != nil {
			log.Errorf("extract timeline batch compress action failed: %v", extractErr)
			return utils.Errorf("extract timeline reducer_memory action failed: %v", extractErr)
		}
		result := action.GetString("reducer_memory")
		if result == "" && cumulativeSummary == "" {
			log.Warn("batch compress got empty reducer memory in json field")
		}
		return nil
	})
	if err != nil {
		log.Warnf("batch compress call ai failed: %v", err)
		return
	}

	compressedMemory := action.GetString("reducer_memory")
	if compressedMemory == "" {
		compressedMemory = cumulativeSummary
	} else {
		compressedMemory += "\n" + cumulativeSummary
	}
	if compressedMemory == "" {
		log.Warn("================================================================")
		log.Warn("================================================================")
		log.Warn("batch compress got empty compressed memory, action dumpped: ")
		fmt.Println(action.GetParams())
		log.Warn("================================================================")
		log.Warn("================================================================")
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

func (m *Timeline) calculateActualContentSize() int64 {
	buf := bytes.NewBuffer(nil)
	initOnce := sync.Once{}
	count := 0

	shrinkStartId, _, _ := m.summary.Last()

	m.idToTimelineItem.ForEach(func(id int64, item *TimelineItem) bool {
		initOnce.Do(func() {
			buf.WriteString("timeline:\n")
		})

		ts, ok := m.idToTs.Get(item.GetID())
		if !ok {
			log.Warnf("BUG: timeline id %v not found", item.GetID())
		}
		t := time.Unix(0, ts*int64(time.Millisecond))
		timeStr := t.Format(utils.DefaultTimeFormat3)

		if shrinkStartId > 0 && item.GetID() <= shrinkStartId {
			val, ok := m.summary.Get(shrinkStartId)
			if ok && !val.Value().deleted {
				buf.WriteString(fmt.Sprintf("--[%s] id: %v memory: %v\n", timeStr, item.GetID(), val.Value().GetShrinkResult()))
			}
			return true
		}

		if item.deleted {
			return true
		}

		buf.WriteString(fmt.Sprintf("--[%s]\n", timeStr))
		raw := item.String()
		for _, line := range utils.ParseStringToRawLines(raw) {
			buf.WriteString(fmt.Sprintf("     %s\n", line))
		}
		count++
		return true
	})
	if count > 0 {
		return int64(len(buf.String()))
	}
	return 0
}

func (m *Timeline) dumpSizeCheck() {
	// 在 push 时检查内容大小，如果超过限制就压缩
	if m.totalDumpContentLimit <= 0 {
		return
	}

	// 获取当前内容大小（不包括reducer）
	contentSize := m.calculateActualContentSize()
	if contentSize <= m.totalDumpContentLimit {
		return // 内容大小正常
	}

	log.Infof("timeline content too large (%d > %d), triggering batch compression", contentSize, m.totalDumpContentLimit)

	// 压缩到合适的大小
	m.compressForSizeLimit()
}

// EmergencyCompress performs non-AI compression by removing oldest items
// This is the public API that can be called from outside
// Use this when timeline is too large and needs to be compressed without AI assistance
func (m *Timeline) EmergencyCompress() {
	m.emergencyCompress(MaxTimelineSaveSize)
}

// emergencyCompress performs non-AI compression by removing oldest items
// This is used when timeline is too large and needs to be compressed without AI assistance
func (m *Timeline) emergencyCompress(targetSize int) {
	if m == nil {
		return
	}

	// Calculate current size
	tlstr, err := MarshalTimeline(m)
	if err != nil {
		log.Errorf("emergency compress: failed to marshal timeline: %v", err)
		return
	}
	currentSize := len(tlstr)
	if currentSize <= targetSize {
		return // Already small enough
	}

	log.Warnf("emergency compress: current size %d, target size %d", currentSize, targetSize)

	// Get all item IDs ordered by timestamp (oldest first)
	var itemIDs []int64
	m.idToTimelineItem.ForEach(func(id int64, item *TimelineItem) bool {
		itemIDs = append(itemIDs, id)
		return true
	})

	if len(itemIDs) <= 1 {
		log.Warnf("emergency compress: only %d items left, cannot compress further", len(itemIDs))
		return
	}

	// Keep removing oldest items until we're under target size
	// We need to keep at least 1 item
	removedCount := 0
	for len(itemIDs) > 1 && currentSize > targetSize {
		// Remove the oldest item (first in the list)
		oldestID := itemIDs[0]
		itemIDs = itemIDs[1:]

		// Get the item for summary before removing
		item, ok := m.idToTimelineItem.Get(oldestID)
		if !ok {
			continue
		}

		// Create a brief summary of what was removed (without AI)
		briefSummary := m.createEmergencySummary(item, oldestID)

		// Remove from all maps
		if ts, ok := m.idToTs.Get(oldestID); ok {
			m.tsToTimelineItem.Delete(ts)
			m.idToTs.Delete(oldestID)
		}
		m.idToTimelineItem.Delete(oldestID)
		m.summary.Delete(oldestID)

		// Store the emergency summary in reducers
		if briefSummary != "" {
			if lt, ok := m.reducers.Get(oldestID); ok {
				lt.Push(briefSummary)
			} else {
				m.reducers.Set(oldestID, linktable.NewUnlimitedStringLinkTable(briefSummary))
			}
		}

		removedCount++

		// Recalculate size periodically (every 10 items for performance)
		if removedCount%10 == 0 {
			tlstr, err = MarshalTimeline(m)
			if err != nil {
				continue
			}
			currentSize = len(tlstr)
		}
	}

	// Final size check
	tlstr, _ = MarshalTimeline(m)
	log.Infof("emergency compress completed: removed %d items, final size: %d (target: %d)", removedCount, len(tlstr), targetSize)
}

// createEmergencySummary creates a brief summary of an item without AI assistance
func (m *Timeline) createEmergencySummary(item *TimelineItem, id int64) string {
	if item == nil {
		return ""
	}

	// Get timestamp
	ts, ok := m.idToTs.Get(id)
	if !ok {
		return ""
	}
	t := time.Unix(0, ts*int64(time.Millisecond))
	timeStr := t.Format(utils.DefaultTimeFormat3)

	// Create a very brief summary based on item type
	var summary string
	switch v := item.value.(type) {
	case *aitool.ToolResult:
		if v.Success {
			summary = fmt.Sprintf("[%s] tool:%s success", timeStr, v.Name)
		} else {
			summary = fmt.Sprintf("[%s] tool:%s failed", timeStr, v.Name)
		}
	case *UserInteraction:
		summary = fmt.Sprintf("[%s] user-interaction stage:%v", timeStr, v.Stage)
	case *TextTimelineItem:
		// Truncate text to 50 chars
		text := v.Text
		if len(text) > 50 {
			text = text[:47] + "..."
		}
		summary = fmt.Sprintf("[%s] text:%s", timeStr, text)
	default:
		summary = fmt.Sprintf("[%s] item removed (emergency compress)", timeStr)
	}

	return summary
}

func (m *Timeline) compressForSizeLimit() {
	if m.ai == nil || m.totalDumpContentLimit <= 0 {
		return
	}

	total := int64(m.idToTimelineItem.Len())
	if total <= 1 {
		return // 不能压缩到少于1个项目
	}

	// 计算当前内容大小（不包括reducer）
	currentSize := m.calculateActualContentSize()

	// 如果内容大小没有超过限制，不需要压缩
	if currentSize <= m.totalDumpContentLimit {
		return
	}

	// 当内容大小超过限制时，压缩到原来的一半大小
	targetSize := int(total / 2)
	if targetSize < 1 {
		targetSize = 1
	}

	log.Infof("content size %d > limit %d, compressing to half size: %d items",
		currentSize, m.totalDumpContentLimit, targetSize)

	if m.compressing.Done() {
		m.compressing.Reset()
	}

	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("batch compress panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		m.compressing.DoOr(func() {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("batch compress panic: %v", err)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()
			m.batchCompressByTargetSize(targetSize)
		}, func() {
			log.Info("batch compress is already running, skip this compress request")
		})
	}()
}

// MaxBatchCompressPromptSize is the maximum size (80KB) for batch compress prompt
// This leaves room for the template overhead while keeping under 100KB total
const MaxBatchCompressPromptSize = 80 * 1024

//go:embed prompts/timeline/batch_compress.txt
var timelineBatchCompress string

func (m *Timeline) renderBatchCompressPrompt(items []*TimelineItem, nonceStr string) string {
	if len(items) == 0 {
		return ""
	}

	ins, err := template.New("timeline-batch-compress").Parse(timelineBatchCompress)
	if err != nil {
		log.Errorf("BUG: batch compress prompt template failed: %v", err)
		return ""
	}

	var buf bytes.Buffer
	var nonce = nonceStr
	if nonce == "" {
		nonce = utils.RandStringBytes(6)
	}

	// 构建要压缩的 items 字符串，限制总大小
	var itemsStr strings.Builder
	totalSize := 0
	actualItemCount := 0

	for i, item := range items {
		itemContent := fmt.Sprintf("[%d] %s", i+1, item.String())

		// Check if adding this item would exceed the limit
		if totalSize+len(itemContent)+1 > MaxBatchCompressPromptSize {
			log.Warnf("batch compress: truncating items at %d/%d due to size limit (%d > %d)",
				i, len(items), totalSize+len(itemContent), MaxBatchCompressPromptSize)

			// Add a notice that items were truncated
			truncateNotice := fmt.Sprintf("\n... [%d more items truncated due to size limit] ...", len(items)-i)
			if totalSize+len(truncateNotice) < MaxBatchCompressPromptSize {
				itemsStr.WriteString(truncateNotice)
			}
			break
		}

		if i > 0 {
			itemsStr.WriteString("\n")
			totalSize++
		}
		itemsStr.WriteString(itemContent)
		totalSize += len(itemContent)
		actualItemCount++
	}

	if actualItemCount == 0 {
		log.Warnf("batch compress: no items could fit within size limit, using truncated first item")
		// Force include at least a truncated version of the first item
		firstItem := items[0].String()
		if len(firstItem) > MaxBatchCompressPromptSize-100 {
			firstItem = firstItem[:MaxBatchCompressPromptSize-100] + "... [truncated]"
		}
		itemsStr.WriteString(fmt.Sprintf("[1] %s", firstItem))
		actualItemCount = 1
	}

	err = ins.Execute(&buf, map[string]any{
		"ExtraMetaInfo":   m.ExtraMetaInfo(),
		"ItemsToCompress": itemsStr.String(),
		"ItemCount":       actualItemCount,
		"NONCE":           nonce,
	})
	if err != nil {
		log.Errorf("BUG: batch compress prompt execution failed: %v", err)
		return ""
	}
	return buf.String()
}

// MaxSummaryPromptTimelineSize is the maximum size (60KB) for timeline content in summary prompt
// This leaves room for Input, ExtraMetaInfo, and template overhead
const MaxSummaryPromptTimelineSize = 60 * 1024

// MaxSummaryPromptInputSize is the maximum size (30KB) for input content in summary prompt
const MaxSummaryPromptInputSize = 30 * 1024

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

	// Get timeline dump and truncate if too large
	timelineDump := m.Dump()
	if len(timelineDump) > MaxSummaryPromptTimelineSize {
		log.Warnf("summary prompt: timeline dump too large (%d > %d), truncating",
			len(timelineDump), MaxSummaryPromptTimelineSize)
		// Keep the end of timeline (more recent items are more important)
		timelineDump = "... [earlier timeline truncated due to size] ...\n" +
			timelineDump[len(timelineDump)-MaxSummaryPromptTimelineSize+50:]
	}

	// Get input and truncate if too large
	inputStr := result.String()
	if len(inputStr) > MaxSummaryPromptInputSize {
		log.Warnf("summary prompt: input too large (%d > %d), truncating",
			len(inputStr), MaxSummaryPromptInputSize)
		inputStr = inputStr[:MaxSummaryPromptInputSize-50] + "\n... [content truncated due to size] ..."
	}

	err = ins.Execute(&buf, map[string]any{
		"ExtraMetaInfo": m.ExtraMetaInfo(),
		"Timeline":      timelineDump,
		"Input":         inputStr,
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

func (m *Timeline) String() string {
	return m.Dump()
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

	var timelineItems = m.idToTimelineItem.Values()
	if len(timelineItems) > n {
		timelineItems = timelineItems[len(timelineItems)-n:]
	}

	// Extract ToolResult objects from TimelineItems
	var result []*aitool.ToolResult
	for _, item := range timelineItems {
		if toolResult, ok := item.value.(*aitool.ToolResult); ok {
			result = append(result, toolResult)
		}
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

	m.pushTimelineItem(ts, id, item)
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

// ReassignIDs reassigns sequential IDs to all timeline items starting from the given startID
// This is used when restoring from persistent session to avoid ID conflicts
// Returns the next available ID after reassignment
func (m *Timeline) ReassignIDs(idGenerator func() int64) int64 {
	if m == nil || idGenerator == nil {
		return 0
	}

	// Collect all items ordered by their original timestamp to maintain order
	type itemWithTs struct {
		ts   int64
		item *TimelineItem
	}
	var orderedItems []itemWithTs

	// Iterate through items in timestamp order
	m.tsToTimelineItem.ForEach(func(ts int64, item *TimelineItem) bool {
		orderedItems = append(orderedItems, itemWithTs{ts: ts, item: item})
		return true
	})

	if len(orderedItems) == 0 {
		return 0
	}

	// Create new mappings
	newIdToTs := omap.NewOrderedMap(map[int64]int64{})
	newIdToTimelineItem := omap.NewOrderedMap(map[int64]*TimelineItem{})
	newSummary := omap.NewOrderedMap(map[int64]*linktable.LinkTable[*TimelineItem]{})
	newReducers := omap.NewOrderedMap(map[int64]*linktable.LinkTable[string]{})

	// Track old ID to new ID mapping for summary and reducers
	oldToNewID := make(map[int64]int64)

	var lastID int64
	// Reassign IDs in order
	for _, itemWithTs := range orderedItems {
		item := itemWithTs.item
		ts := itemWithTs.ts
		oldID := item.GetID()
		newID := idGenerator()
		lastID = newID

		// Update the ID in the underlying value
		switch v := item.value.(type) {
		case *aitool.ToolResult:
			v.ID = newID
		case *UserInteraction:
			v.ID = newID
		case *TextTimelineItem:
			v.ID = newID
		default:
			log.Warnf("unknown timeline item value type: %T", v)
		}

		// Store mapping
		oldToNewID[oldID] = newID

		// Add to new mappings
		newIdToTs.Set(newID, ts)
		newIdToTimelineItem.Set(newID, item)

		// Update summary if exists for this old ID
		if summaryLt, ok := m.summary.Get(oldID); ok {
			newSummary.Set(newID, summaryLt)
		}

		// Update reducers if exists for this old ID
		if reducerLt, ok := m.reducers.Get(oldID); ok {
			newReducers.Set(newID, reducerLt)
		}
	}

	// Replace old mappings with new ones
	m.idToTs = newIdToTs
	m.idToTimelineItem = newIdToTimelineItem
	m.summary = newSummary
	m.reducers = newReducers

	log.Infof("reassigned IDs for %d timeline items, last ID: %d", len(orderedItems), lastID)
	return lastID
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
