package aicommon

import (
	"bytes"
	"cmp"
	"context"
	_ "embed"
	"fmt"
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
	"github.com/yaklang/yaklang/common/ai/ytoken"
)

type Timeline struct {
	extraMetaInfo func() string // extra meta info for timeline, like runtime id, etc.
	config        AICallerConfigIf
	ai            AICaller

	idToTs           *omap.OrderedMap[int64, int64]
	tsToTimelineItem *omap.OrderedMap[int64, *TimelineItem]
	idToTimelineItem *omap.OrderedMap[int64, *TimelineItem]
	reducers         *omap.OrderedMap[int64, *linktable.LinkTable[string]]
	// reducerTs 与 reducers 一一对应，记录 reducer 的稳定时间戳（毫秒，UnixMilli）
	// 用于 DumpBefore / GroupByMinutes 渲染 reducer 行的时间字段，避免使用 time.Now() 破坏缓存稳定性
	// 关键词: reducerTs, reducer 稳定时间戳, 缓存稳定
	reducerTs   *omap.OrderedMap[int64, int64]
	archiveRefs *omap.OrderedMap[int64, *TimelineArchiveRef]

	// this limit is used to limit the timeline dump content size (in tokens).
	perDumpContentLimit   int64
	totalDumpContentLimit int64

	compressing *utils.Once
}

func (m *Timeline) OrderInsertId(id int64, item *TimelineItem) {
	m.idToTimelineItem.OrderInsert(id, item, cmp.Less[int64])
}

func (m *Timeline) OrderInsertTs(ts int64, item *TimelineItem) {
	m.tsToTimelineItem.OrderInsert(ts, item, cmp.Less[int64])
}

// MaxTimelineSaveSize is the maximum size (1.5MB storage limit) for timeline data when saving to database
const MaxTimelineSaveSize = 1536 * 1024

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

// GetMaxID 返回当前 timeline 中最大的条目 ID。
// 如果 timeline 为空则返回 0。
func (m *Timeline) GetMaxID() int64 {
	ids := m.idToTimelineItem.Keys()
	var maxID int64
	for _, id := range ids {
		if id > maxID {
			maxID = id
		}
	}
	return maxID
}

// TruncateAfter 软删除所有 ID 严格大于 checkpointID 的 timeline 条目。
// 用于在串行 sub-agent 结束后恢复 timeline 状态，实现 agent 间上下文隔离。
func (m *Timeline) TruncateAfter(checkpointID int64) {
	for _, id := range m.idToTimelineItem.Keys() {
		if id > checkpointID {
			m.SoftDelete(id)
		}
	}
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
		reducers:              m.reducers.Copy(),
		reducerTs:             m.reducerTs.Copy(),
		archiveRefs:           m.archiveRefs.Copy(),
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
	}
}

// CreateSubTimeline 用入参 ids 限定活跃 item 集合，构造一个新的 sub-timeline
// 关键词: CreateSubTimeline, 子 timeline 构造, reducer 继承
//
// reducer 继承语义:
//
//	reducers 代表的是主 timeline 的历史压缩快照，其 key 是 reducerKeyID（已被批量压缩
//	并从活跃区移除的旧 item id），与入参 ids（活跃 item id 子集）属于不同命名空间，
//	用 ids 去 m.reducers.Get(id) 必然 miss——这是历史死代码。
//	为保持 sub-timeline 与主 timeline 历史视图一致（任何派生 sub.Dump() 都应包含完整压缩
//	记忆），这里始终全量复制 reducers 与 reducerTs，与 ids 解耦。
//	历史 bug: DumpBefore / TimelineWithout / CurrentTaskTimeline 全部经此路径，旧实现使
//	它们在 prompt 中丢失全部 reducer block，破坏 LLM 记忆连续性，源头此处修复。
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
			tl.OrderInsertId(id, ret)
		}
		if ret, ok := m.tsToTimelineItem.Get(ts); ok {
			tl.OrderInsertTs(ts, ret)
		}
	}

	// 全量继承 reducer 历史快照（与 ids 解耦）
	if m.reducers != nil && m.reducers.Len() > 0 {
		m.reducers.ForEach(func(reducerKeyID int64, lt *linktable.LinkTable[string]) bool {
			if lt == nil {
				return true
			}
			tl.reducers.Set(reducerKeyID, lt)
			if m.reducerTs != nil {
				if ts, ok := m.reducerTs.Get(reducerKeyID); ok {
					tl.reducerTs.Set(reducerKeyID, ts)
				}
			}
			return true
		})
	}

	return tl
}

func (m *Timeline) SoftBindConfig(config AICallerConfigIf, aiCaller AICaller) {
	if config != nil {
		m.config = config
		m.SetTimelineContentLimit(config.GetTimelineContentSizeLimit())
	}
	if utils.IsNil(m.ai) && !utils.IsNil(aiCaller) {
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
		reducers:         omap.NewOrderedMap(map[int64]*linktable.LinkTable[string]{}),
		reducerTs:        omap.NewOrderedMap(map[int64]int64{}),
		archiveRefs:      omap.NewOrderedMap(map[int64]*TimelineArchiveRef{}),
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
	m.OrderInsertId(id, item)
	m.OrderInsertTs(ts, item)
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

// 关键词: timeline_batch_compress 已迁出
// 以下批量压缩相关代码已迁移至 timeline_batch_compress.go：
//   - estimateItemContentTokens
//   - findCompressSplitByRecentKeepTokens
//   - compressForSizeLimit
//   - batchCompressOldestWithRecent
//   - renderBatchCompressPrompt / buildRecentKeptString / buildItemsToCompressString
//   - MaxBatchCompressPromptSize / MaxBatchCompressRecentSize / timelineBatchCompress (embed)
// timeline.go 仅保留: calculateActualContentSize / dumpSizeCheck / emergencyCompress / createEmergencySummary

func (m *Timeline) calculateActualContentSize() int64 {
	buf := bytes.NewBuffer(nil)
	initOnce := sync.Once{}
	count := 0

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
		return int64(ytoken.CalcTokenCount(buf.String()))
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
	var removedIDs []int64
	var removedItems []*TimelineItem
	var emergencySummaries []string
	var lastRemovedID int64
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
		removedIDs = append(removedIDs, oldestID)
		removedItems = append(removedItems, item)
		if briefSummary != "" {
			emergencySummaries = append(emergencySummaries, briefSummary)
		}
		lastRemovedID = oldestID

		// 在删除 idToTs 之前先取出 ts，写入 reducerTs 用于稳定渲染
		// 关键词: emergencyCompress, reducerTs, 稳定时间戳
		var origTs int64
		if ts, ok := m.idToTs.Get(oldestID); ok {
			origTs = ts
			m.tsToTimelineItem.Delete(ts)
			m.idToTs.Delete(oldestID)
		}
		m.idToTimelineItem.Delete(oldestID)

		// Store the emergency summary in reducers
		if briefSummary != "" {
			if lt, ok := m.reducers.Get(oldestID); ok {
				lt.Push(briefSummary)
			} else {
				m.reducers.Set(oldestID, linktable.NewUnlimitedStringLinkTable(briefSummary))
			}
			// 同步写入 reducerTs（关键词: emergencyCompress, reducer 稳定时间戳）
			if origTs > 0 {
				m.reducerTs.Set(oldestID, origTs)
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
	if len(removedIDs) > 0 {
		m.attachArchiveRef(lastRemovedID, m.archiveForgottenBatch(
			TimelineArchiveReasonEmergencyCompress,
			lastRemovedID,
			removedIDs,
			removedItems,
			strings.Join(emergencySummaries, "\n"),
		))
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

// TimelineDumpDefaultIntervalMinutes 是 Dump / String / DumpBefore 默认使用的分桶分钟数
// 关键词: TimelineDumpDefaultIntervalMinutes, Dump 默认 interval
const TimelineDumpDefaultIntervalMinutes = 3

// TimelineDumpDefaultAITagName 是 Dump / String / DumpBefore 默认使用的 aitag tag 名
// 关键词: TimelineDumpDefaultAITagName, Dump aitag tag
const TimelineDumpDefaultAITagName = "TIMELINE"

// Dump 输出 timeline 的 aitag-wrapped 渲染串。
//
// 等价于:
//
//	GroupByMinutes(TimelineDumpDefaultIntervalMinutes).
//	    GetAllRenderable().
//	    RenderWithFrozenBoundary(
//	        TimelineDumpDefaultAITagName,
//	        TimelineFrozenBoundaryTagName,
//	        TimelineFrozenBoundaryNonce,
//	    )
//
// 仅包含 reducer block + interval block，不包含 archive block（archive 暂时不展示在 Dump 中）。
//
// 输出在含混合 frozen+open 的场景下会自动加上
// <|AI_CACHE_FROZEN_semi-dynamic|>...<|AI_CACHE_FROZEN_END_semi-dynamic|>
// 边界标签把已冻结前缀包起来, 让下游 aicache hijacker 能用简单字符串
// IndexOf 精准定位到 frozen 与 open 的边界, 实现 §7.7.7 双 cc 命中所需
// 的 user1 (frozen prefix) / user2 (open tail) 切分。
//
// 全 frozen / 全 open 场景下不加边界, 保持与原 Render 字节一致, 退化路径
// 让 hijacker 走 2 段拼接 + aibalance 单 cc 兜底。
//
// 关键词: Timeline.Dump, GroupByMinutes 别名, aitag 包裹, 前缀缓存,
//        AI_CACHE_FROZEN 边界, hijacker 切割锚点, §7.7.7
func (m *Timeline) Dump() string {
	if m == nil {
		return ""
	}
	return m.GroupByMinutes(TimelineDumpDefaultIntervalMinutes).
		GetAllRenderable().
		RenderWithFrozenBoundary(
			TimelineDumpDefaultAITagName,
			TimelineFrozenBoundaryTagName,
			TimelineFrozenBoundaryNonce,
		)
}

// String 是 Dump 的别名，为了兼容 fmt.Stringer 接口
func (m *Timeline) String() string {
	return m.Dump()
}

// DumpBefore 输出 ID <= beforeId 的部分 timeline，结构与 Dump 一致
// 通过 CreateSubTimeline 限定上界，再走 Dump 公共路径，避免修改 GroupByMinutes 签名
// 关键词: Timeline.DumpBefore, 子 timeline 上界, GroupByMinutes 复用, reducer 继承
//
// 注意: reducer 由 CreateSubTimeline 在源头全量继承，这里无需重复迁移。
func (m *Timeline) DumpBefore(beforeId int64) string {
	if m == nil {
		return ""
	}
	if m.idToTimelineItem == nil {
		return ""
	}

	// 收集 ID <= beforeId 的活跃条目 ID
	var ids []int64
	m.idToTimelineItem.ForEach(func(id int64, _ *TimelineItem) bool {
		if id <= beforeId {
			ids = append(ids, id)
		}
		return true
	})
	if len(ids) == 0 {
		// 无活跃 item 时与 Dump() 在 idToTimelineItem 空时的行为对齐：返回空。
		// GroupByMinutes 在活跃 item 为空时即便有 reducer 也不会渲染 reducer block，
		// 这里不做特例化，以保持 Dump / DumpBefore 的语义对称。
		return ""
	}

	sub := m.CreateSubTimeline(ids...)
	if sub == nil {
		return ""
	}
	return sub.Dump()
}

func (m *Timeline) attachArchiveRef(reducerKeyID int64, ref *TimelineArchiveRef) {
	if reducerKeyID <= 0 || ref == nil {
		return
	}
	if m.archiveRefs == nil {
		m.archiveRefs = omap.NewOrderedMap(map[int64]*TimelineArchiveRef{})
	}
	m.archiveRefs.Set(reducerKeyID, ref)
}

func (m *Timeline) archiveForgottenBatch(reason TimelineArchiveReason, reducerKeyID int64, ids []int64, items []*TimelineItem, summary string) *TimelineArchiveRef {
	store := m.timelineArchiveStore()
	if store == nil || len(ids) == 0 || len(items) == 0 {
		return nil
	}

	startID := ids[0]
	endID := ids[len(ids)-1]
	refID := utils.CalcSha256(
		fmt.Sprintf("%s", reason),
		strconv.FormatInt(reducerKeyID, 10),
		strconv.FormatInt(startID, 10),
		strconv.FormatInt(endID, 10),
		strings.TrimSpace(summary),
	)

	batch := &TimelineArchiveBatch{
		ArchiveID:           "timeline-archive-" + refID[:16],
		PersistentSessionID: m.timelinePersistentSessionID(),
		Reason:              reason,
		Summary:             strings.TrimSpace(summary),
		MergedContent:       strings.TrimSpace(timelineArchiveMergedContent(items)),
		SourceChunks:        timelineArchiveSourceChunks(items),
		ReducerKeyID:        reducerKeyID,
		SourceStartID:       startID,
		SourceEndID:         endID,
		ItemCount:           len(ids),
		RepresentativeSnips: timelineArchiveRepresentativeSnippets(items, 3),
		Tags: []string{
			"timeline_midterm",
			fmt.Sprintf("timeline_range_%d_%d", startID, endID),
			fmt.Sprintf("timeline_reason_%s", reason),
		},
	}

	if len(items) > 0 {
		batch.SourceStartAt = items[0].createdAt
		batch.SourceEndAt = items[len(items)-1].createdAt
	}

	ref, err := store.ArchiveCompressedBatch(context.Background(), batch)
	if err != nil {
		log.Warnf("archive forgotten timeline batch failed: %v", err)
		return nil
	}
	return ref
}

func timelineArchiveRepresentativeSnippets(items []*TimelineItem, limit int) []string {
	if limit <= 0 {
		limit = 3
	}
	result := make([]string, 0, limit)
	for _, item := range items {
		if item == nil {
			continue
		}
		snippet := strings.TrimSpace(utils.ShrinkString(item.String(), 240))
		if snippet == "" {
			continue
		}
		result = append(result, snippet)
		if len(result) >= limit {
			break
		}
	}
	return result
}

func timelineArchiveMergedContent(items []*TimelineItem) string {
	if len(items) == 0 {
		return ""
	}

	var buf strings.Builder
	for _, item := range items {
		if item == nil || item.deleted {
			continue
		}

		if !item.createdAt.IsZero() {
			buf.WriteString("[")
			buf.WriteString(item.createdAt.Format(time.RFC3339))
			buf.WriteString("] ")
		}
		buf.WriteString("id=")
		buf.WriteString(strconv.FormatInt(item.GetID(), 10))
		buf.WriteString("\n")

		raw := strings.TrimSpace(item.String())
		if raw != "" {
			for _, line := range utils.ParseStringToRawLines(raw) {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				buf.WriteString("- ")
				buf.WriteString(line)
				buf.WriteString("\n")
			}
		}
		buf.WriteString("\n")
	}

	return strings.TrimSpace(buf.String())
}

func timelineArchiveSourceChunks(items []*TimelineItem) []string {
	if len(items) == 0 {
		return nil
	}

	chunks := make([]string, 0, len(items))
	for _, item := range items {
		if item == nil || item.deleted {
			continue
		}

		var buf strings.Builder
		if !item.createdAt.IsZero() {
			buf.WriteString("[")
			buf.WriteString(item.createdAt.Format(time.RFC3339))
			buf.WriteString("] ")
		}
		buf.WriteString("id=")
		buf.WriteString(strconv.FormatInt(item.GetID(), 10))
		buf.WriteString("\n")

		raw := strings.TrimSpace(item.String())
		if raw != "" {
			for _, line := range utils.ParseStringToRawLines(raw) {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				buf.WriteString("- ")
				buf.WriteString(line)
				buf.WriteString("\n")
			}
		}

		chunk := strings.TrimSpace(buf.String())
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
	}
	return chunks
}

func (m *Timeline) timelineArchiveStore() TimelineArchiveStore {
	if m == nil || m.config == nil {
		return nil
	}
	if provider, ok := m.config.(interface{ GetTimelineArchiveStore() TimelineArchiveStore }); ok {
		return provider.GetTimelineArchiveStore()
	}
	return nil
}

func (m *Timeline) timelinePersistentSessionID() string {
	if m == nil || m.config == nil {
		return ""
	}
	if provider, ok := m.config.(interface{ GetPersistentSessionID() string }); ok {
		return provider.GetPersistentSessionID()
	}
	return ""
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
	newReducers := omap.NewOrderedMap(map[int64]*linktable.LinkTable[string]{})
	// 关键词: ReassignIDs, reducerTs 重映射
	newReducerTs := omap.NewOrderedMap(map[int64]int64{})

	// Track old ID to new ID mapping for reducers
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

		// Update reducers if exists for this old ID
		if reducerLt, ok := m.reducers.Get(oldID); ok {
			newReducers.Set(newID, reducerLt)
			// 同步重映射 reducerTs
			if origTs, tsOk := m.reducerTs.Get(oldID); tsOk {
				newReducerTs.Set(newID, origTs)
			}
		}
	}

	// 注意：保持 ReassignIDs 原有语义——只重映射 idToTimelineItem 中存在的 ID 对应的 reducers，
	// 不为孤立 reducer key（idToTimelineItem 里已不存在）单独再分配新 ID
	_ = oldToNewID

	// Replace old mappings with new ones
	m.idToTs = newIdToTs
	m.idToTimelineItem = newIdToTimelineItem
	m.reducers = newReducers
	m.reducerTs = newReducerTs

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
