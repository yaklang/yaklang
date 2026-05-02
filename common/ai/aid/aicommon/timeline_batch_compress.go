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
	"github.com/yaklang/yaklang/common/utils/linktable"

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
	raw := item.String()
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
	total := m.idToTimelineItem.Len()
	if total <= 1 {
		return 0
	}

	// 关键词: idToTimelineItem.Keys, id 升序, 反向遍历
	ids := m.idToTimelineItem.Keys()
	if len(ids) != total {
		// 防御性: Keys() 应与 Len() 一致
		return 0
	}

	if keepTokens <= 0 {
		// 至少保留最新 1 个 item
		return total - 1
	}

	var acc int64
	// 从最新端（数组尾部）向前累加 token
	for i := len(ids) - 1; i >= 0; i-- {
		id := ids[i]
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
//	keepTokens = currentSize / 4，按 token 反向累加从最新端向旧端切分，
//	[0, splitIdx) 进入 toCompress 一并压成 1 条 reducer，
//	[splitIdx, end] 进入 recentKeep 不动，并作为"现在 agent 在做什么"的 prompt 上下文一并喂给 AI。
//
// 关键词: compressForSizeLimit, recent keep token 切分, batch compress 触发
func (m *Timeline) compressForSizeLimit() {
	if m.ai == nil || m.totalDumpContentLimit <= 0 {
		return
	}

	total := m.idToTimelineItem.Len()
	if total <= 1 {
		return // 不能压缩到少于1个项目
	}

	// 计算当前活跃区 token 数（不含 reducer），与触发同口径
	currentSize := m.calculateActualContentSize()
	if currentSize <= m.totalDumpContentLimit {
		return
	}

	// 关键词: compressForSizeLimit, keepTokens, currentSize/4
	// 目标：保留最新约 1/4 token 的 item 不动，其余压缩
	keepTokens := currentSize / 4
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
	ids := m.idToTimelineItem.Keys()
	if len(ids) != total {
		log.Warnf("compressForSizeLimit: ids length %d != total %d, skip", len(ids), total)
		return
	}
	var toCompress []*TimelineItem
	var recentKeep []*TimelineItem
	for i, id := range ids {
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

	// 生成压缩提示（双段：RECENT_KEEP + ITEMS_TO_COMPRESS）
	nonceStr := utils.RandStringBytes(4)
	prompt := m.renderBatchCompressPrompt(toCompress, recentKeep, nonceStr)
	if prompt == "" {
		// If prompt is empty, fall back to emergency compress
		log.Warnf("batch compress: prompt is empty, falling back to emergency compress")
		m.emergencyCompress(MaxTimelineSaveSize)
		return
	}

	// 调用 AI 进行批量压缩
	var action *Action
	var cumulativeSummary string
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
					boundEmitter.EmitDefaultStreamEvent(
						"memory-timeline",
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
	// 关键词: batchCompressOldestWithRecent, reducerTs, 缓存稳定
	// 在删除 idToTs 之前先获取最末压缩 item 的原始 ts，写入 reducerTs
	// 这样 DumpBefore / GroupByMinutes 渲染 reducer 行时能拿到稳定时间，避免 time.Now() 漂移
	var lastCompressedTs int64
	if ts, ok := m.idToTs.Get(lastCompressedId); ok {
		lastCompressedTs = ts
	}
	if lt, ok := m.reducers.Get(lastCompressedId); ok {
		lt.Push(compressedMemory)
	} else {
		m.reducers.Set(lastCompressedId, linktable.NewUnlimitedStringLinkTable(compressedMemory))
	}
	if lastCompressedTs > 0 {
		m.reducerTs.Set(lastCompressedId, lastCompressedTs)
	}
	m.attachArchiveRef(lastCompressedId, m.archiveForgottenBatch(
		TimelineArchiveReasonBatchCompress,
		lastCompressedId,
		idsToRemove,
		toCompress,
		compressedMemory,
	))
	log.Infof("batch compressed %d items into reducer at id: %v", len(toCompress), lastCompressedId)

	// 删除被压缩的 items
	for _, id := range idsToRemove {
		m.idToTimelineItem.Delete(id)
		if ts, ok := m.idToTs.Get(id); ok {
			m.tsToTimelineItem.Delete(ts)
			m.idToTs.Delete(id)
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
func (m *Timeline) renderBatchCompressPrompt(toCompress []*TimelineItem, recentKeep []*TimelineItem, nonceStr string) string {
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
	recentStr, recentCount, recentTruncated := buildRecentKeptString(recentKeep, MaxBatchCompressRecentSize)

	// 2) 剩余预算给 ITEMS_TO_COMPRESS（保留 1KB 给模板/指引/JSON schema）
	const templateOverheadReserve = 1024
	remainingBudget := MaxBatchCompressPromptSize - len(recentStr) - templateOverheadReserve
	if remainingBudget < 1024 {
		// 极端情况：recent 占满了，强行至少给 toCompress 留 1KB
		remainingBudget = 1024
	}

	itemsStr, actualItemCount, itemsTruncated := buildItemsToCompressString(toCompress, remainingBudget)

	if actualItemCount == 0 {
		log.Warnf("batch compress: no items could fit within size limit, using truncated first item")
		firstItem := toCompress[0].String()
		if len(firstItem) > remainingBudget-100 {
			firstItem = firstItem[:remainingBudget-100] + "... [truncated]"
		}
		itemsStr = fmt.Sprintf("[1] %s", firstItem)
		actualItemCount = 1
		itemsTruncated = true
	}

	if recentTruncated {
		log.Warnf("batch compress: RECENT_KEEP truncated to %d bytes (kept %d items)", len(recentStr), recentCount)
	}
	if itemsTruncated {
		log.Warnf("batch compress: ITEMS_TO_COMPRESS truncated (budget=%d)", remainingBudget)
	}

	err = ins.Execute(&buf, map[string]any{
		"ExtraMetaInfo":   m.ExtraMetaInfo(),
		"RecentKept":      recentStr,
		"RecentKeptCount": recentCount,
		"HasRecentKept":   recentCount > 0,
		"ItemsToCompress": itemsStr,
		"ItemCount":       actualItemCount,
		"NONCE":           nonce,
	})
	if err != nil {
		log.Errorf("BUG: batch compress prompt execution failed: %v", err)
		return ""
	}
	return buf.String()
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
