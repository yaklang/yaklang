package aicommon

// 关键词: timeline_batch_compress, batch compress, RECENT_KEEP, ITEMS_TO_COMPRESS
//
// 本文件聚合 Timeline 的"基于 AI 的批量压缩"全流程：
//   - 触发判断 (compressForSizeLimit)
//   - 切点计算 (estimateItemContentTokens / findCompressSplitByRecentKeepTokens)
//   - prompt 渲染 (renderBatchCompressPrompt + buildRecentKeptString + buildItemsToCompressString)
//   - 实际压缩 (batchCompressOldestWithRecent)
//
// 注意:
//   - 与 batch_compress 强相关但**不**属于本文件的代码:
//       calculateActualContentSize / dumpSizeCheck / emergencyCompress / createEmergencySummary
//     它们是基础度量与非 AI 兜底压缩，仍位于 timeline.go。

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"strings"
	"text/template"
	"time"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/ai/ytoken"
)

// estimateItemContentTokens 按 calculateActualContentSize 一致的 wrap 格式估算单个 item 的 token 数
// 用于 batchCompress 切点：从最新端反向累加 token 找到保留区起点
// 注意：BPE token 化在多 item 拼接时不严格线性可加，本函数为近似估算（误差可接受）
// 关键词: estimateItemContentTokens, batchCompress 切点 token 估算
func (m *Timeline) estimateItemContentTokens(id int64, item *TimelineItem) int64 {
	if item == nil || item.deleted {
		return 0
	}
	var buf bytes.Buffer
	ts, _ := m.idToTs.Get(id)
	t := time.Unix(0, ts*int64(time.Millisecond))
	timeStr := t.Format(utils.DefaultTimeFormat3)

	buf.WriteString(fmt.Sprintf("--[%s]\n", timeStr))
	raw := selectShrunkContent(item)
	for _, line := range utils.ParseStringToRawLines(raw) {
		buf.WriteString(fmt.Sprintf("     %s\n", line))
	}
	return int64(ytoken.CalcTokenCount(buf.String()))
}

// findCompressSplitByRecentKeepTokens 找到 active 区按 token 大小划分的切点：
//
//	从最新端向旧端反向累加 token，累加首次 >= keepTokens 时停下，
//	返回 keepStartIdx：[0, keepStartIdx) 是 toCompress（最旧的，需要压缩），
//	[keepStartIdx, len) 是 recentKeep（最新的，保留不动）。
//
// 边界:
//   - 0 或 1 个活跃 item: 返回 0（不压缩）
//   - keepTokens <= 0:   至少保留最新 1 个 item
//   - 全部 item 都被纳入"最新保留区"才达到 keepTokens: 返回 0（不压缩，等价于全部都是最近）
//
// 关键词: findCompressSplitByRecentKeepTokens, batchCompress 切点, recent keep, token 维度
func (m *Timeline) findCompressSplitByRecentKeepTokens(keepTokens int64) int {
	if m == nil || m.idToTimelineItem == nil {
		return 0
	}
	activeIDs := m.getActiveTimelineItemIDs()
	total := len(activeIDs)
	if total <= 1 {
		return 0
	}

	if keepTokens <= 0 {
		// 至少保留最新 1 个 item
		return total - 1
	}

	var acc int64
	// 从最新端（数组尾部）向前累加 token
	for i := len(activeIDs) - 1; i >= 0; i-- {
		id := activeIDs[i]
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil {
			continue
		}
		acc += m.estimateItemContentTokens(id, item)
		if acc >= keepTokens {
			// 当前 i 即保留区起点；[0, i) 进入待压缩，[i, end] 留作最近保留
			return i
		}
	}
	// 全部 item 累加仍未达到 keepTokens => 全部都是"最近"，无需压缩
	return 0
}

// compressForSizeLimit 当活跃区 token 超过 totalDumpContentLimit 时，触发 batch compress：
//
//	keepTokens = currentSize / 6，按 token 反向累加从最新端向旧端切分，
//	[0, splitIdx) 进入 toCompress 一并压成 1 条 reducer，
//	[splitIdx, end] 进入 recentKeep 不动，并作为"现在 agent 在做什么"的 prompt 上下文一并喂给 AI。
//
// 关键词: compressForSizeLimit, recent keep token 切分, batch compress 触发
func (m *Timeline) compressForSizeLimit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.compressForSizeLimitLocked()
}

func (m *Timeline) compressForSizeLimitLocked() {
	// Control-plane mutations must be materialized before ordinary facts are sent
	// to a reducer. They are excluded from activeIDs and reducer prompts below.
	m.forcePromoteAllLocked()
	if m.ai == nil || m.totalDumpContentLimit <= 0 {
		return
	}

	activeIDs := m.getActiveTimelineItemIDs()
	total := len(activeIDs)
	if total <= 1 {
		return // 不能压缩到少于1个项目
	}

	// Caller already holds Timeline.mu. Do not call calculateActualContentSize(),
	// because it would try to RLock the same RWMutex and deadlock.
	currentSize := m.calculateActualContentSizeLocked()
	if currentSize <= m.totalDumpContentLimit {
		return
	}

	// 关键词: compressForSizeLimit, keepTokens, currentSize/6
	// 目标：保留最新约 1/6 token 的 item 不动，其余压缩
	keepTokens := currentSize / 6
	if keepTokens < 1 {
		keepTokens = 1
	}

	splitIdx := m.findCompressSplitByRecentKeepTokens(keepTokens)
	if splitIdx <= 0 {
		// 全部 item 累加 token 仍未达到 keepTokens，或活跃 item 太少；不压缩
		log.Infof("compress skipped: %d active items, keep all as recent (currentSize=%d, keepTokens=%d)",
			total, currentSize, keepTokens)
		return
	}

	// 防御性：splitIdx < 2 意味着只压缩了 1 条，价值很低，跳过本次（避免空炮）
	// 关键词: compressForSizeLimit, splitIdx 阈值, 避免无效压缩
	if splitIdx < 2 {
		log.Infof("compress skipped: only %d oldest item to compress (split=%d/%d), wait for more growth",
			splitIdx, splitIdx, total)
		return
	}

	// 按 id 升序收集 toCompress / recentKeep 切片
	var toCompress []*TimelineItem
	var recentKeep []*TimelineItem
	for i, id := range activeIDs {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil {
			continue
		}
		if i < splitIdx {
			toCompress = append(toCompress, item)
		} else {
			recentKeep = append(recentKeep, item)
		}
	}

	if len(toCompress) == 0 {
		return
	}

	log.Infof("content size %d > limit %d, compress oldest %d items, keep recent %d items (~%d tokens)",
		currentSize, m.totalDumpContentLimit, len(toCompress), len(recentKeep), keepTokens)

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
			m.batchCompressOldestWithRecent(toCompress, recentKeep)
		}, func() {
			log.Info("batch compress is already running, skip this compress request")
		})
	}()
}

// batchCompressOldestWithRecent 把活跃区按 splitIdx 切出的"最旧 toCompress"压缩成 1 条 reducer，
// 同时把"最新 recentKeep"作为 prompt 中的 RECENT_KEEP 参考段一并喂给 AI（不修改、不删除），
// 让 AI 基于"现在 agent 在做什么"判断 toCompress 中哪些细节有价值需保留。
//
// 关键词: batchCompressOldestWithRecent, RECENT_KEEP context, batch compress 双段
func (m *Timeline) batchCompressOldestWithRecent(toCompress []*TimelineItem, recentKeep []*TimelineItem) {
	if len(toCompress) == 0 {
		return
	}

	// If AI is nil, use emergency compress instead
	if m.ai == nil {
		log.Warnf("batch compress: AI is nil, using emergency compress")
		m.emergencyCompress(MaxTimelineSaveSize)
		return
	}

	total := int64(len(m.getActiveTimelineItemIDs()))
	if total <= 1 {
		return
	}

	// Check if current timeline is already too large for AI processing
	// If so, do emergency compress first to bring it to a manageable size
	tlstr, err := MarshalTimeline(m)
	if err == nil && len(tlstr) > MaxTimelineSaveSize*2 {
		log.Warnf("batch compress: timeline too large (%d), performing emergency compress first", len(tlstr))
		m.emergencyCompress(MaxTimelineSaveSize)
		// emergencyCompress 已经动了活跃区，本次切片 (toCompress / recentKeep) 已失效，
		// 直接返回，等下一次 push 触发 dumpSizeCheck 再切
		log.Warnf("batch compress: aborting current cycle after emergency compress, will retry next cycle")
		return
	}

	// 收集要从活跃区删除的 id 列表（顺序与 toCompress 对齐，确保 lastCompressedId 是最末一个）
	var idsToRemove []int64
	for _, item := range toCompress {
		if item == nil {
			continue
		}
		idsToRemove = append(idsToRemove, item.GetID())
	}

	if len(idsToRemove) == 0 {
		return
	}

	log.Infof("batch compress: compressing %d oldest items, keeping %d recent items as context",
		len(toCompress), len(recentKeep))

	// 计算 token 预算
	inputTokenEstimate := int64(0)
	for _, item := range toCompress {
		if item != nil {
			inputTokenEstimate += m.estimateItemContentTokens(item.GetID(), item)
		}
	}

	// OutputTokenBudget: 压缩后 head 的目标 token 上限
	// 目标是 head + recentKeep <= totalDumpContentLimit
	// headBudget = totalDumpContentLimit - keepTokens - headOverhead(渲染 header 约 50 token)
	const headRenderOverhead = 50
	keepTokens := m.calculateActualContentSizeLocked() / 6
	outputTokenBudget := m.totalDumpContentLimit - keepTokens - headRenderOverhead
	if outputTokenBudget < 200 {
		outputTokenBudget = 200
	}

	// 旧 head 文本：不送 AI 二次压缩，最终直接前置拼接
	oldHeadText := ""
	if m.compressedHead != nil {
		oldHeadText = strings.TrimSpace(m.compressedHead.Text)
	}

	// 生成压缩提示（双段：RECENT_KEEP + ITEMS_TO_COMPRESS + token 预算）
	nonceStr := utils.RandStringBytes(4)
	prompt := m.renderBatchCompressPrompt(m.compressedHead, toCompress, recentKeep, nonceStr, inputTokenEstimate, outputTokenBudget)
	if prompt == "" {
		// If prompt is empty, fall back to emergency compress
		log.Warnf("batch compress: prompt is empty, falling back to emergency compress")
		m.emergencyCompress(MaxTimelineSaveSize)
		return
	}

	// 调用 AI 进行批量压缩
	var action *Action
	err = CallAITransaction(m.config, prompt, m.ai.CallSpeedPriorityAI, func(response *AIResponse) error {
		var boundEmitter *Emitter
		if m.config != nil {
			boundEmitter = response.BindEmitter(m.config.GetEmitter())
		}
		var r io.Reader
		if m.config == nil {
			r = response.GetUnboundStreamReader(false)
		} else {
			r = response.GetOutputStreamReader("batch-compress", true, m.config.GetEmitter())
		}

		// 为每个结构化字段注册 stream handler，实时 emit 到 UI
		fieldHandlers := []string{
			"key_findings", "active_config", "completed_work",
			"failed_and_resolved", "discarded", "user_directives",
		}
		streamHandlers := make([]ActionMakerOption, 0, len(fieldHandlers)+2)
		streamHandlers = append(streamHandlers,
			WithActionNonce(nonceStr),
		)
		for _, fieldName := range fieldHandlers {
			fn := fieldName // capture
			streamHandlers = append(streamHandlers, WithActionFieldStreamHandler(
				[]string{fn},
				func(key string, reader io.Reader) {
					if boundEmitter == nil {
						io.Copy(io.Discard, reader)
						return
					}
					boundEmitter.EmitDefaultSystemStreamEvent(
						"memory-timeline",
						utils.JSONStringReader(reader),
						response.GetTaskIndex(),
						func() {
							log.Infof("memory-timeline field [%s] streamed", fn)
						},
					)
				},
			))
		}

		var extractErr error
		action, extractErr = ExtractActionFromStream(
			m.config.GetContext(),
			r, "timeline-reducer",
			streamHandlers...,
		)
		if extractErr != nil {
			log.Errorf("extract timeline batch compress action failed: %v", extractErr)
			return utils.Errorf("extract timeline reducer action failed: %v", extractErr)
		}
		return nil
	}, WithAIRequest_CallerLabel("timeline-batch-compress"))
	if err != nil {
		log.Warnf("batch compress call ai failed: %v", err)
		return
	}

	// 解析结构化字段，拼成分段文本
	compressedMemory := buildStructuredCompressedMemory(action)
	if compressedMemory == "" {
		// 兜底：如果结构化字段全空，尝试旧格式 reducer_memory
		compressedMemory = action.GetString("reducer_memory")
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

	// post-check: 如果 AI 输出超标，规则截断低优先级字段
	compressedMemory = enforceOutputTokenBudget(compressedMemory, outputTokenBudget)

	// 旧 head 前置拼接（不二次压缩）
	finalText := compressedMemory
	if oldHeadText != "" {
		finalText = oldHeadText + "\n\n" + compressedMemory
	}

	// 如果拼接后 head 超预算，触发 head-only 精简
	if int64(MeasureTokens(finalText)) > outputTokenBudget {
		finalText = m.refineCompressedHeadLocked(finalText, outputTokenBudget, nonceStr)
	}

	// 存储压缩结果（单有效压缩段）
	lastCompressedId := idsToRemove[len(idsToRemove)-1]
	var lastCompressedTs int64
	if ts, ok := m.idToTs.Get(lastCompressedId); ok {
		lastCompressedTs = ts
	}
	m.updateCompressedHead(&TimelineCompressedHead{
		Text:             strings.TrimSpace(finalText),
		CoveredEndItemID: lastCompressedId,
		CoveredEndAtMs:   lastCompressedTs,
	})
	m.attachArchiveRef(lastCompressedId, m.archiveForgottenBatch(
		TimelineArchiveReasonBatchCompress,
		lastCompressedId,
		idsToRemove,
		toCompress,
		finalText,
	))
	log.Infof("batch compressed %d items into reducer at id: %v", len(toCompress), lastCompressedId)

	// 标记被压缩的 items 为非活跃
	for _, id := range idsToRemove {
		if item, ok := m.idToTimelineItem.Get(id); ok && item != nil {
			item.deleted = true
		}
	}
}

// MaxBatchCompressPromptSize is the maximum size (in bytes) for batch compress prompt
// This leaves room for the template overhead while keeping under the total token budget
const MaxBatchCompressPromptSize = 80 * 1024

// MaxBatchCompressRecentSize 是 batch compress prompt 中 RECENT_KEEP 段的字节预算上限
// 占总预算约 1/5，保证 ITEMS_TO_COMPRESS 仍是主体，且 RECENT_KEEP 提供足够的"现在"上下文
// 关键词: MaxBatchCompressRecentSize, recent keep prompt budget
const MaxBatchCompressRecentSize = 16 * 1024

//go:embed prompts/timeline/batch_compress.txt
var timelineBatchCompress string

// renderBatchCompressPrompt 渲染双段 batch compress prompt:
//
//	RECENT_KEEP   - 最新保留段，作为压缩参考"现在 agent 在做什么"，AI 不修改它
//	ITEMS_TO_COMPRESS - 待压缩的最旧段，AI 将其浓缩成 1 条 reducer
//
// 预算分配:
//
//	RECENT_KEEP   <= MaxBatchCompressRecentSize（先填，从最新向旧）
//	ITEMS_TO_COMPRESS <= MaxBatchCompressPromptSize - actualRecentSize（再填，按时间顺序最旧到次新）
//
// 关键词: renderBatchCompressPrompt, RECENT_KEEP, ITEMS_TO_COMPRESS, prompt 预算分配
func (m *Timeline) renderBatchCompressPrompt(currentHead *TimelineCompressedHead, toCompress []*TimelineItem, recentKeep []*TimelineItem, nonceStr string, inputTokenEstimate int64, outputTokenBudget int64) string {
	if len(toCompress) == 0 {
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

	// 1) 先构造 RECENT_KEEP 段（从最新向旧填，超限就在前面加 truncate notice）
	// 关键词: renderBatchCompressPrompt, RECENT_KEEP 截断, 从新向旧填充
	promptRecentKeep := projectTimelineItemsForPrompt(recentKeep)
	promptToCompress := projectTimelineItemsForPrompt(toCompress)
	recentStr, recentCount, recentTruncated := buildRecentKeptString(promptRecentKeep, MaxBatchCompressRecentSize)

	// 2) 剩余预算给 ITEMS_TO_COMPRESS（保留 1KB 给模板/指引/JSON schema）
	const templateOverheadReserve = 1024
	remainingBudget := MaxBatchCompressPromptSize - len(recentStr) - templateOverheadReserve
	if remainingBudget < 1024 {
		// 极端情况：recent 占满了，强行至少给 toCompress 留 1KB
		remainingBudget = 1024
	}

	itemsStr, actualItemCount, itemsTruncated := buildItemsToCompressString(promptToCompress, remainingBudget)

	if actualItemCount == 0 {
		if len(promptToCompress) == 0 {
			itemsStr = "[system bookkeeping omitted from prompt projection]"
			itemsTruncated = false
		} else {
			log.Warnf("batch compress: no items could fit within size limit, using truncated first item")
			firstItem := promptToCompress[0].String()
			if len(firstItem) > remainingBudget-100 {
				firstItem = firstItem[:remainingBudget-100] + "... [truncated]"
			}
			itemsStr = fmt.Sprintf("[1] %s", firstItem)
			actualItemCount = 1
			itemsTruncated = true
		}
	}

	if recentTruncated {
		log.Warnf("batch compress: RECENT_KEEP truncated to %d bytes (kept %d items)", len(recentStr), recentCount)
	}
	if itemsTruncated {
		log.Warnf("batch compress: ITEMS_TO_COMPRESS truncated (budget=%d)", remainingBudget)
	}

	err = ins.Execute(&buf, map[string]any{
		"ExtraMetaInfo":        m.ExtraMetaInfo(),
		"RecentKept":           recentStr,
		"RecentKeptCount":      recentCount,
		"HasRecentKept":        recentCount > 0,
		"ItemsToCompress":      itemsStr,
		"ItemCount":            actualItemCount,
		"HasCompressedHead":    currentHead != nil && strings.TrimSpace(currentHead.Text) != "",
		"CompressedHeadText":   compressedHeadText(currentHead),
		"CompressedHeadID":     compressedHeadCoveredID(currentHead),
		"CompressedHeadAtMs":   compressedHeadCoveredAtMs(currentHead),
		"CompressedHeadVer":    compressedHeadVersion(currentHead),
		"InputTokenEstimate":   inputTokenEstimate,
		"OutputTokenBudget":    outputTokenBudget,
		"NONCE":                nonce,
	})
	if err != nil {
		log.Errorf("BUG: batch compress prompt execution failed: %v", err)
		return ""
	}
	return buf.String()
}

func (m *Timeline) getActiveTimelineItemIDs() []int64 {
	if m == nil || m.idToTimelineItem == nil {
		return nil
	}
	ids := m.idToTimelineItem.Keys()
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted || isPromotableTimelineItem(item) {
			continue
		}
		out = append(out, id)
	}
	return out
}

func compressedHeadText(head *TimelineCompressedHead) string {
	if head == nil {
		return ""
	}
	return strings.TrimSpace(head.Text)
}

func compressedHeadCoveredID(head *TimelineCompressedHead) int64 {
	if head == nil {
		return 0
	}
	return head.CoveredEndItemID
}

func compressedHeadCoveredAtMs(head *TimelineCompressedHead) int64 {
	if head == nil {
		return 0
	}
	return head.CoveredEndAtMs
}

func compressedHeadVersion(head *TimelineCompressedHead) int64 {
	if head == nil {
		return 0
	}
	return head.Version
}

// buildRecentKeptString 从最新向旧填充 recentKeep 段，受 budget 字节上限约束
// 输出按时间顺序（最旧 → 最新）排版，前缀若有截断则加 truncate notice
// 关键词: buildRecentKeptString, recent keep 截断
func buildRecentKeptString(recentKeep []*TimelineItem, budget int) (string, int, bool) {
	if len(recentKeep) == 0 || budget <= 0 {
		return "", 0, false
	}

	// 从最新（末尾）向旧（开头）反向选取，保持总字节 <= budget
	type framed struct {
		idx  int
		text string
	}
	picked := make([]framed, 0, len(recentKeep))
	used := 0
	truncated := false
	for i := len(recentKeep) - 1; i >= 0; i-- {
		item := recentKeep[i]
		if item == nil {
			continue
		}
		// 与 ITEMS_TO_COMPRESS 同样格式: "[seq] <item.String()>"
		text := fmt.Sprintf("[%d] %s", i+1, item.String())
		// 含换行符
		need := used + len(text)
		if len(picked) > 0 {
			need++
		}
		if need > budget {
			truncated = true
			break
		}
		picked = append(picked, framed{idx: i, text: text})
		used = need
	}

	if len(picked) == 0 {
		return "", 0, len(recentKeep) > 0
	}

	// picked 当前是"最新→旧"，输出时反转为"旧→最新"
	for l, r := 0, len(picked)-1; l < r; l, r = l+1, r-1 {
		picked[l], picked[r] = picked[r], picked[l]
	}

	var buf strings.Builder
	if truncated {
		buf.WriteString(fmt.Sprintf("... [%d earlier recent items truncated due to size budget] ...\n", len(recentKeep)-len(picked)))
	}
	for i, f := range picked {
		if i > 0 {
			buf.WriteString("\n")
		}
		buf.WriteString(f.text)
	}
	return buf.String(), len(picked), truncated
}

// buildItemsToCompressString 按时间顺序（最旧 → 次新）填充 toCompress 段，受 budget 字节上限约束
// 关键词: buildItemsToCompressString, items to compress 截断
func buildItemsToCompressString(items []*TimelineItem, budget int) (string, int, bool) {
	if len(items) == 0 || budget <= 0 {
		return "", 0, false
	}
	var buf strings.Builder
	totalSize := 0
	actualItemCount := 0
	truncated := false
	for i, item := range items {
		if item == nil {
			continue
		}
		itemContent := fmt.Sprintf("[%d] %s", i+1, item.String())
		need := totalSize + len(itemContent)
		if i > 0 {
			need++
		}
		if need > budget {
			truncated = true
			truncateNotice := fmt.Sprintf("\n... [%d more items truncated due to size limit] ...", len(items)-i)
			if totalSize+len(truncateNotice) <= budget {
				buf.WriteString(truncateNotice)
			}
			break
		}
		if i > 0 {
			buf.WriteString("\n")
			totalSize++
		}
		buf.WriteString(itemContent)
		totalSize += len(itemContent)
		actualItemCount++
	}
	return buf.String(), actualItemCount, truncated
}

// buildStructuredCompressedMemory 从 AI action 中提取结构化字段，
// 拼成有固定段落标记的分段文本，存入 compressedHead.Text。
//
// 字段优先级: key_findings > active_config > user_directives > completed_work > failed_and_resolved > discarded
// 关键词: buildStructuredCompressedMemory, 结构化压缩输出, 分段文本
func buildStructuredCompressedMemory(action *Action) string {
	if action == nil {
		return ""
	}

	var sections []struct {
		title string
		body  string
	}

	// key_findings (string array)
	findings := action.GetStringSlice("key_findings")
	if len(findings) > 0 {
		var buf strings.Builder
		for _, f := range findings {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			buf.WriteString("- ")
			buf.WriteString(f)
			buf.WriteString("\n")
		}
		if buf.Len() > 0 {
			sections = append(sections, struct {
				title string
				body  string
			}{"Key Findings", strings.TrimRight(buf.String(), "\n")})
		}
	}

	// active_config
	if s := strings.TrimSpace(action.GetString("active_config")); s != "" {
		sections = append(sections, struct {
			title string
			body  string
		}{"Active Config", s})
	}

	// user_directives
	if s := strings.TrimSpace(action.GetString("user_directives")); s != "" {
		sections = append(sections, struct {
			title string
			body  string
		}{"User Directives", s})
	}

	// completed_work
	if s := strings.TrimSpace(action.GetString("completed_work")); s != "" {
		sections = append(sections, struct {
			title string
			body  string
		}{"Completed Work", s})
	}

	// failed_and_resolved
	if s := strings.TrimSpace(action.GetString("failed_and_resolved")); s != "" {
		sections = append(sections, struct {
			title string
			body  string
		}{"Failed & Resolved", s})
	}

	// discarded
	if s := strings.TrimSpace(action.GetString("discarded")); s != "" {
		sections = append(sections, struct {
			title string
			body  string
		}{"Discarded", s})
	}

	if len(sections) == 0 {
		return ""
	}

	var buf strings.Builder
	for i, sec := range sections {
		if i > 0 {
			buf.WriteString("\n\n")
		}
		buf.WriteString("## ")
		buf.WriteString(sec.title)
		buf.WriteString("\n")
		buf.WriteString(sec.body)
	}
	return buf.String()
}

// enforceOutputTokenBudget 当 AI 输出超过 token 预算时，按优先级截断低价值字段。
// 截断顺序: discarded -> failed_and_resolved -> completed_work -> user_directives -> active_config -> key_findings
// key_findings 永远不截断。
// 关键词: enforceOutputTokenBudget, post-check, 规则截断
func enforceOutputTokenBudget(text string, budget int64) string {
	if budget <= 0 {
		return text
	}
	current := int64(MeasureTokens(text))
	if current <= budget {
		return text
	}

	// 按 ## 标题分节
	sections := splitCompressedHeadSections(text)
	if len(sections) == 0 {
		// 无法分节，整体按 token 截断
		return ShrinkByTokens(text, int(budget))
	}

	// 低优先级字段按顺序截断/删除
	dropOrder := []string{"Discarded", "Failed & Resolved", "Completed Work", "User Directives", "Active Config"}
	for _, name := range dropOrder {
		if current <= budget {
			break
		}
		for i, sec := range sections {
			if sec.title == name {
				sectionTokens := int64(MeasureTokens(sec.body + "## " + sec.title + "\n"))
				// 先尝试截断到一半，如果还是太大就整个删除
				if sectionTokens > 50 {
					half := sectionTokens / 2
					sections[i].body = ShrinkByTokens(sec.body, int(half))
					current = int64(MeasureTokens(joinCompressedHeadSections(sections)))
				}
				if current > budget {
					sections[i].body = ""
					sections[i].title = "" // mark for removal
					current = int64(MeasureTokens(joinCompressedHeadSections(sections)))
				}
				break
			}
		}
	}

	return joinCompressedHeadSections(sections)
}

type compressedHeadSection struct {
	title string
	body  string
}

func splitCompressedHeadSections(text string) []compressedHeadSection {
	lines := strings.Split(text, "\n")
	var sections []compressedHeadSection
	var current *compressedHeadSection
	var bodyLines []string

	flush := func() {
		if current != nil {
			current.body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
			sections = append(sections, *current)
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			flush()
			current = &compressedHeadSection{title: strings.TrimPrefix(trimmed, "## ")}
			bodyLines = nil
		} else if current != nil {
			bodyLines = append(bodyLines, line)
		}
	}
	flush()
	return sections
}

func joinCompressedHeadSections(sections []compressedHeadSection) string {
	var buf strings.Builder
	first := true
	for _, sec := range sections {
		if sec.title == "" && sec.body == "" {
			continue
		}
		if !first {
			buf.WriteString("\n\n")
		}
		first = false
		buf.WriteString("## ")
		buf.WriteString(sec.title)
		buf.WriteString("\n")
		buf.WriteString(sec.body)
	}
	return buf.String()
}

// refineCompressedHeadLocked 当 head 累积超过预算时，调用 AI 对 head 自身做精简。
// 只精简 completed_work/discarded/failed_and_resolved，保留 key_findings/active_config/user_directives。
// 关键词: refineCompressedHead, head-only 精简, head 累积控制
func (m *Timeline) refineCompressedHeadLocked(headText string, budget int64, nonceStr string) string {
	if m.ai == nil || headText == "" || budget <= 0 {
		return headText
	}

	// 如果 AI 不可用或精简 prompt 构建失败，用规则截断兜底
	if MeasureTokens(headText) <= int(budget) {
		return headText
	}

	// 构建 head 精简 prompt
	refinePrompt := buildRefineHeadPrompt(headText, budget, nonceStr)
	if refinePrompt == "" {
		return enforceOutputTokenBudget(headText, budget)
	}

	var action *Action
	err := CallAITransaction(m.config, refinePrompt, m.ai.CallSpeedPriorityAI, func(response *AIResponse) error {
		var r io.Reader
		if m.config == nil {
			r = response.GetUnboundStreamReader(false)
		} else {
			r = response.GetOutputStreamReader("head-refine", true, m.config.GetEmitter())
		}

		fieldHandlers := []string{
			"key_findings", "active_config", "completed_work",
			"failed_and_resolved", "discarded", "user_directives",
		}
		streamHandlers := make([]ActionMakerOption, 0, len(fieldHandlers)+1)
		streamHandlers = append(streamHandlers, WithActionNonce(nonceStr))
		for _, fieldName := range fieldHandlers {
			streamHandlers = append(streamHandlers, WithActionFieldStreamHandler(
				[]string{fieldName},
				func(key string, reader io.Reader) {
					io.Copy(io.Discard, reader)
				},
			))
		}

		var extractErr error
		action, extractErr = ExtractActionFromStream(
			m.config.GetContext(),
			r, "timeline-reducer",
			streamHandlers...,
		)
		if extractErr != nil {
			return utils.Errorf("extract head refine action failed: %v", extractErr)
		}
		return nil
	}, WithAIRequest_CallerLabel("timeline-head-refine"))

	if err != nil || action == nil {
		log.Warnf("head refine AI call failed: %v, falling back to rule-based truncation", err)
		return enforceOutputTokenBudget(headText, budget)
	}

	refined := buildStructuredCompressedMemory(action)
	if refined == "" {
		return enforceOutputTokenBudget(headText, budget)
	}

	// 确保 refined 不超过预算
	if int64(MeasureTokens(refined)) > budget {
		refined = enforceOutputTokenBudget(refined, budget)
	}
	return refined
}

// buildRefineHeadPrompt 构建 head-only 精简 prompt
func buildRefineHeadPrompt(headText string, budget int64, nonceStr string) string {
	const refineTemplate = `# 角色与核心目标

你是一个 **AI 记忆精简模块**。当前的任务是对一段已压缩的历史摘要进行**精简**，使其 token 数不超过 {{ .OutputTokenBudget }}。

# 需要精简的压缩段
<|HEAD_TO_REFINE_{{ .NONCE }}|>
{{ .HeadText }}
<|HEAD_TO_REFINE_END_{{ .NONCE }}|>

## 精简规则

1. **key_findings 和 active_config 不得丢失或删减**：这些是最高价值信息，必须原样保留。
2. **user_directives 不得丢失**：用户指令必须原样保留。
3. **可以精简 completed_work**：合并相似条目，去除冗余描述，但保留工作阶段和关键结论。
4. **可以精简或删除 failed_and_resolved**：如果内容过长，保留最重要的转折性结论，删减细节。
5. **可以删除 discarded**：如果需要进一步缩减，优先删除此字段。
6. 保持原有的 6 字段结构化输出格式。

## 输出 token 预算
总输出 ≤ {{ .OutputTokenBudget }} tokens

# 输出格式：JSON

输出与原始压缩段相同的结构化 JSON 格式：

` + "```schema" + `
{
  "type": "object",
  "required": ["@action", "key_findings"],
  "properties": {
    "@action": { "const": "timeline-reducer" },
    "key_findings": { "type": "array", "items": { "type": "string" }, "minItems": 1 },
    "active_config": { "type": "string" },
    "completed_work": { "type": "string" },
    "failed_and_resolved": { "type": "string" },
    "discarded": { "type": "string" },
    "user_directives": { "type": "string" }
  }
}
` + "```" + `
`

	ins, err := template.New("timeline-head-refine").Parse(refineTemplate)
	if err != nil {
		log.Errorf("BUG: head refine prompt template failed: %v", err)
		return ""
	}

	nonce := nonceStr
	if nonce == "" {
		nonce = utils.RandStringBytes(6)
	}

	var buf bytes.Buffer
	err = ins.Execute(&buf, map[string]any{
		"HeadText":         headText,
		"OutputTokenBudget": budget,
		"NONCE":            nonce,
	})
	if err != nil {
		log.Errorf("BUG: head refine prompt execution failed: %v", err)
		return ""
	}
	return buf.String()
}
