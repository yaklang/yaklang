package aicommon

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// 关键词: bucket_bench, 字节子桶调优, 离线重放, 缓存命中模拟
//
// 该文件提供 Timeline 字节子桶大小调优所需的纯函数 helper:
//   - 重放真实 aispace session 工具调用记录
//   - 构造合成场景 (short_query / dense_tools / single_huge / mixed)
//   - 重放序列下针对任意 bucketSizer 收集 frozen 段命中代价指标
//
// 这里没有 build tag, 让单测 (timeline_bucket_tuning_test.go) 与 bench
// 入口 (timeline_bucket_bench_test.go, 带 bucketbench tag) 都能复用。

// BucketBenchEvent 表示重放序列里的一条 timeline push 事件。
// 关键词: BucketBenchEvent, 重放事件
type BucketBenchEvent struct {
	ID      int64
	Ts      time.Time
	Name    string // 工具名 / event 名
	Success bool
	Content string // 进入 Timeline 的字符串内容 (作为 ToolResult.Data)
}

// BucketBenchScenario 命名后的事件序列, 便于报告里按名称引用。
// 关键词: BucketBenchScenario
type BucketBenchScenario struct {
	Name   string
	Events []BucketBenchEvent
}

// BucketBenchResult 单次实验产出的关键指标。
// 关键词: BucketBenchResult, flushCount, netCost
type BucketBenchResult struct {
	Scenario          string
	BudgetLabel       string
	NumEvents         int
	FlushCount        int   // frozen 段 hash 变化次数 ≈ cache_create 次数
	StableHitCount    int   // frozen 段未变次数 ≈ cache_hit 次数
	AvgFrozenBytes    int64 // 全程平均 frozen 段字节
	P95FrozenBytes    int64 // 95 分位
	MaxFrozenBytes    int64
	TotalFrozenSeen   int64 // 累计 frozen 字节 (含变化和稳定)
	EstCreateCost     int64 // sum(frozenBytesWhenChanged) * 1.25
	EstHitSavings     int64 // sum(frozenBytesWhenStable) * 0.6
	EstNetCost        int64 // EstCreateCost - EstHitSavings
	OpenBytesAtEnd    int64 // 末态 open 子桶字节
	FrozenSubBlockMax int   // 单次出现过的 frozen 子块最大数量
}

// BucketBenchOptions 控制单次实验的所有变量。
// 关键词: BucketBenchOptions
type BucketBenchOptions struct {
	// Budget 直接传给 GroupByMinutesAndBytes (固定桶大小路径)。
	//   -1 -> 禁用字节切分 (只按 3 分钟时间桶)
	//    0 -> 用 TimelineDumpDefaultBucketByteSize
	//   >0 -> 用该字节预算
	//
	// 当 Sizer != nil 时, Budget 被忽略, 走动态 sizer 路径。
	Budget int64

	// Sizer 提供动态桶大小决策。若非 nil, 实验跑动态算法对照。
	// 关键词: BucketBenchOptions.Sizer
	Sizer BucketSizer

	// IntervalMinutes 默认为 TimelineDumpDefaultIntervalMinutes (3)。
	IntervalMinutes int

	// HitDiscount 是 dashscope 实测的命中折扣系数。cached_tokens
	// 计费 ≈ 0.4 * input_token 单价, 节省 = 1 - 0.4 = 0.6。
	// 关键词: HitDiscount, dashscope 计费
	HitDiscount float64

	// CreateMultiplier 是 dashscope cache_creation_input_tokens 的
	// 计费倍率, 实测 1.25 (125%)。
	// 关键词: CreateMultiplier, dashscope 计费
	CreateMultiplier float64
}

const (
	defaultBucketBenchHitDiscount      = 0.6
	defaultBucketBenchCreateMultiplier = 1.25
)

// BucketSizer 是动态桶大小决策接口。
// 关键词: BucketSizer, 动态桶大小, 调优实验
//
// 实现必须是**幂等**的: 相同输入序列必须产生相同的 budget 决策, 不允许
// 引入 wall-clock time 或全局随机源 (调优实验依赖可重放性)。
type BucketSizer interface {
	NextBudget(ctx BucketSizerContext) int64
}

// BucketSizerContext 给 BucketSizer 提供当前桶状态。
// 关键词: BucketSizerContext, 桶上下文
type BucketSizerContext struct {
	IntervalMinutes      int
	BucketStart          time.Time
	BucketEnd            time.Time
	NextItemCreatedAt    time.Time // 即将打包的 item 的 createdAt
	CurrentBucketBytes   int       // 当前子桶里已累积的字节 (含 header)
	CurrentBucketItems   int       // 当前子桶里已累积的 entry 数
	RecentEntryMeanBytes int       // 最近 N 条 entry 的平均字节 (含 header 估算)
}

// BucketSizerFunc 让普通函数也能实现 BucketSizer 接口。
type BucketSizerFunc func(ctx BucketSizerContext) int64

// NextBudget 让 BucketSizerFunc 满足 BucketSizer 接口。
func (f BucketSizerFunc) NextBudget(ctx BucketSizerContext) int64 {
	return f(ctx)
}

// FixedBucketSizer 返回固定的桶大小, 等价于现有常量行为。
// 关键词: FixedBucketSizer, A_Fixed 策略
func FixedBucketSizer(budget int64) BucketSizer {
	return BucketSizerFunc(func(ctx BucketSizerContext) int64 {
		return budget
	})
}

// TimeRemainingBucketSizer 让桶大小随时间桶剩余时间线性衰减:
//
//	budget = max(minBudget, base * (remaining / total))
//
// 时间桶刚开始 -> budget=base; 接近结束 -> budget=minBudget。
// 这意味着越接近时间桶尾段越倾向于早切, 让 frozen 段更早进入。
//
// 关键词: TimeRemainingBucketSizer, B_TimeRemaining 策略
func TimeRemainingBucketSizer(base, minBudget int64) BucketSizer {
	if minBudget <= 0 {
		minBudget = 4 * 1024
	}
	if base <= minBudget {
		base = minBudget * 4
	}
	return BucketSizerFunc(func(ctx BucketSizerContext) int64 {
		total := ctx.BucketEnd.Sub(ctx.BucketStart)
		if total <= 0 {
			return base
		}
		remaining := ctx.BucketEnd.Sub(ctx.NextItemCreatedAt)
		if remaining <= 0 {
			return minBudget
		}
		ratio := float64(remaining) / float64(total)
		if ratio > 1 {
			ratio = 1
		}
		v := int64(float64(base) * ratio)
		if v < minBudget {
			v = minBudget
		}
		return v
	})
}

// EntryAdaptiveBucketSizer 让桶大小适应"最近 N 条 entry 的平均字节":
//
//	budget = clamp(meanRecentEntry * N, minBudget, maxBudget)
//
// 让一个桶大致能容纳 N 条平均尺寸的 entry。
// 对小 entry 紧凑, 对大 entry 自动撑大。
//
// 关键词: EntryAdaptiveBucketSizer, C_EntryAdaptive 策略
func EntryAdaptiveBucketSizer(targetEntries int, minBudget, maxBudget int64) BucketSizer {
	if targetEntries <= 0 {
		targetEntries = 8
	}
	if minBudget <= 0 {
		minBudget = 32 * 1024
	}
	if maxBudget <= 0 {
		maxBudget = 256 * 1024
	}
	return BucketSizerFunc(func(ctx BucketSizerContext) int64 {
		mean := ctx.RecentEntryMeanBytes
		if mean <= 0 {
			return minBudget
		}
		v := int64(mean) * int64(targetEntries)
		if v < minBudget {
			v = minBudget
		}
		if v > maxBudget {
			v = maxBudget
		}
		return v
	})
}

// TokenAwareBucketSizer 用字节字符估算 token 数, 目标 token 数 +-误差。
// 这里用保守估算: 1 token ≈ 3.5 byte (中英混合, 工具输出含 JSON / yaml)。
// 关键词: TokenAwareBucketSizer, D_TokenAware 策略
func TokenAwareBucketSizer(targetTokens int) BucketSizer {
	if targetTokens <= 0 {
		targetTokens = 5000
	}
	bytePerToken := 3.5
	budget := int64(float64(targetTokens) * bytePerToken)
	return BucketSizerFunc(func(ctx BucketSizerContext) int64 {
		return budget
	})
}

// DefaultBucketSizer 返回项目推荐的默认动态桶大小算法 (EntryAdaptive 8x, 32K-256K)。
//
// **调优实验结论** (见 TIMELINE_BUCKET_TUNING.md):
//   - 在 mixed 合成场景上, EntryAdaptive 净成本 -820K vs 64K 固定 -550K (优 49%)
//   - 在 real_redhaze 真实数据上, EntryAdaptive 净成本 -2.99M vs 64K 固定 -3.12M (差 4%)
//   - 在 dense_tools 密集工具场景上, EntryAdaptive 净成本 -216K vs 64K 固定 -215K (持平)
//   - 在 short_query / single_huge 上两者均接近最优
//
// 适合作为"主动缓存优化"调用方默认动态策略:
//
//	tl := aicommon.NewTimeline(ai, nil)
//	tl.SetTimelineBucketSizer(aicommon.DefaultBucketSizer())
//
// 不主动注册到 NewTimeline, 保持向后兼容: 老调用方仍走固定 64K 默认值,
// 想要动态自适应的调用方显式注册 sizer。
//
// 关键词: DefaultBucketSizer, EntryAdaptive 默认, 主动缓存推荐
func DefaultBucketSizer() BucketSizer {
	return EntryAdaptiveBucketSizer(8, 32*1024, 256*1024)
}

// LoadRealSessionEvents 从 yakit-projects/aispace/<session>/ 目录扫描 tool_calls
// 与 loop_default_action_calls 的 markdown 文件, 按 mtime 排序还原成 timeline
// push 事件。
//
// 解析逻辑: 文件名 N_<tool>.md 中的 N 用作 ID + 排序键, mtime 作为 ts。
// 文件正文截取首 4KB 作为 ToolResult.Data, 模拟 shrunk content (实际生产里
// 还会经 ShrinkTextBlockByTokens, 这里近似)。
//
// 关键词: LoadRealSessionEvents, 真实重放, aispace
func LoadRealSessionEvents(sessionDir string) ([]BucketBenchEvent, error) {
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		return nil, fmt.Errorf("read session dir: %w", err)
	}

	var events []BucketBenchEvent
	var nextID int64 = 1

	type fileMeta struct {
		path    string
		modTime time.Time
		taskTag string
		kind    string // "tool" / "action"
		seqHint int    // 文件名前缀数字, 若有
	}

	var files []fileMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		taskDir := filepath.Join(sessionDir, e.Name())
		for _, subdir := range []string{"tool_calls", "loop_default_action_calls"} {
			sub := filepath.Join(taskDir, subdir)
			items, err := os.ReadDir(sub)
			if err != nil {
				continue
			}
			for _, it := range items {
				if it.IsDir() || !strings.HasSuffix(it.Name(), ".md") {
					continue
				}
				p := filepath.Join(sub, it.Name())
				info, ierr := it.Info()
				if ierr != nil {
					continue
				}
				kind := "tool"
				if subdir == "loop_default_action_calls" {
					kind = "action"
				}
				seq := parseLeadingInt(it.Name())
				files = append(files, fileMeta{
					path:    p,
					modTime: info.ModTime(),
					taskTag: e.Name(),
					kind:    kind,
					seqHint: seq,
				})
			}
		}
	}

	sort.Slice(files, func(i, j int) bool {
		if !files[i].modTime.Equal(files[j].modTime) {
			return files[i].modTime.Before(files[j].modTime)
		}
		return files[i].seqHint < files[j].seqHint
	})

	for _, fm := range files {
		raw, err := os.ReadFile(fm.path)
		if err != nil {
			continue
		}
		content := string(raw)
		if len(content) > 4*1024 {
			content = content[:4*1024]
		}
		events = append(events, BucketBenchEvent{
			ID:      nextID,
			Ts:      fm.modTime,
			Name:    fmt.Sprintf("%s/%s", fm.kind, fm.taskTag),
			Success: true,
			Content: content,
		})
		nextID++
	}
	return events, nil
}

// parseLeadingInt 解析 "12_xxx.md" 这类文件名前缀的整数, 失败返回 0。
func parseLeadingInt(name string) int {
	end := 0
	for end < len(name) && name[end] >= '0' && name[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	v := 0
	for i := 0; i < end; i++ {
		v = v*10 + int(name[i]-'0')
	}
	return v
}

// BuildSyntheticScenario 根据名称生成确定性的合成场景, 保证可重放。
// 名称: short_query / dense_tools / single_huge / mixed
// 关键词: BuildSyntheticScenario, 合成场景
func BuildSyntheticScenario(name string, baseTs time.Time) BucketBenchScenario {
	r := rand.New(rand.NewSource(int64(hashStr(name))))
	switch name {
	case "short_query":
		// 1 分钟 30 条小 entry, 每条 ~500B
		evs := make([]BucketBenchEvent, 0, 30)
		for i := 0; i < 30; i++ {
			evs = append(evs, BucketBenchEvent{
				ID:      int64(i + 1),
				Ts:      baseTs.Add(time.Duration(i*2) * time.Second),
				Name:    fmt.Sprintf("short/%d", i+1),
				Success: true,
				Content: deterministicPayload(r, 500),
			})
		}
		return BucketBenchScenario{Name: name, Events: evs}

	case "dense_tools":
		// 3 分钟 20 条 2-8KB
		evs := make([]BucketBenchEvent, 0, 20)
		for i := 0; i < 20; i++ {
			size := 2*1024 + r.Intn(6*1024)
			evs = append(evs, BucketBenchEvent{
				ID:      int64(i + 1),
				Ts:      baseTs.Add(time.Duration(i*9) * time.Second),
				Name:    fmt.Sprintf("dense/%d", i+1),
				Success: true,
				Content: deterministicPayload(r, size),
			})
		}
		return BucketBenchScenario{Name: name, Events: evs}

	case "single_huge":
		// 周边 5 条 1KB + 中间 1 条 64KB
		evs := make([]BucketBenchEvent, 0, 6)
		for i := 0; i < 3; i++ {
			evs = append(evs, BucketBenchEvent{
				ID:      int64(i + 1),
				Ts:      baseTs.Add(time.Duration(i*5) * time.Second),
				Name:    fmt.Sprintf("warmup/%d", i+1),
				Success: true,
				Content: deterministicPayload(r, 1024),
			})
		}
		evs = append(evs, BucketBenchEvent{
			ID:      4,
			Ts:      baseTs.Add(20 * time.Second),
			Name:    "huge/dump",
			Success: true,
			Content: deterministicPayload(r, 64*1024),
		})
		for i := 0; i < 2; i++ {
			evs = append(evs, BucketBenchEvent{
				ID:      int64(5 + i),
				Ts:      baseTs.Add(time.Duration(30+i*7) * time.Second),
				Name:    fmt.Sprintf("cooldown/%d", i+1),
				Success: true,
				Content: deterministicPayload(r, 1024),
			})
		}
		return BucketBenchScenario{Name: name, Events: evs}

	case "mixed":
		// 3 个时间桶 (9 分钟) 内交错出现 small / medium / large
		evs := make([]BucketBenchEvent, 0, 36)
		patterns := []int{500, 2 * 1024, 8 * 1024, 1024, 4 * 1024, 12 * 1024, 800, 3 * 1024, 20 * 1024}
		for i := 0; i < 36; i++ {
			size := patterns[i%len(patterns)]
			evs = append(evs, BucketBenchEvent{
				ID:      int64(i + 1),
				Ts:      baseTs.Add(time.Duration(i*15) * time.Second),
				Name:    fmt.Sprintf("mixed/%d", i+1),
				Success: true,
				Content: deterministicPayload(r, size),
			})
		}
		return BucketBenchScenario{Name: name, Events: evs}
	}
	return BucketBenchScenario{Name: name}
}

// deterministicPayload 用提供的 random source 生成长度 n 的伪文本, 确保可重放。
//
// **重要**: 每 ~60 字节强制插入一个 '\n', 避免 ParseStringToRawLines 使用的
// bufio.Scanner 默认 64KB 单 token 上限静默丢弃超长行 (生产里工具输出基本都
// 是多行文本, 不会触发这个边界)。
//
// 关键词: deterministicPayload, 合成 payload, bufio scanner 64K 边界
func deterministicPayload(r *rand.Rand, n int) string {
	if n <= 0 {
		return ""
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyz ABCDEFGHIJKLMNOPQRSTUVWXYZ 0123456789"
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		// 强制让每 ~60 字节出现一个 \n, 避免 single huge entry 触发 bufio 64K 边界。
		if i > 0 && i%60 == 0 {
			buf[i] = '\n'
			continue
		}
		buf[i] = alphabet[r.Intn(len(alphabet))]
	}
	return string(buf)
}

func hashStr(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// ReplayAndMeasure 把 events 按顺序注入一个新 Timeline, 每注入一条都按 opts
// 计算当前 frozen 段 hash 与字节, 累计指标。
// 关键词: ReplayAndMeasure, 离线重放, 指标采集
func ReplayAndMeasure(scenarioName string, events []BucketBenchEvent, opts BucketBenchOptions) BucketBenchResult {
	if opts.IntervalMinutes <= 0 {
		opts.IntervalMinutes = TimelineDumpDefaultIntervalMinutes
	}
	if opts.HitDiscount <= 0 {
		opts.HitDiscount = defaultBucketBenchHitDiscount
	}
	if opts.CreateMultiplier <= 0 {
		opts.CreateMultiplier = defaultBucketBenchCreateMultiplier
	}

	tl := NewTimeline(nil, nil)
	if opts.Sizer != nil {
		tl.SetTimelineBucketSizer(opts.Sizer)
	}

	res := BucketBenchResult{
		Scenario:    scenarioName,
		BudgetLabel: budgetLabel(opts),
		NumEvents:   len(events),
	}

	var lastFrozenHash string
	var sumFrozen, sumCreate, sumHit, totalSeen int64
	var samples []int64
	maxSubBlocks := 0

	for idx, ev := range events {
		tr := &aitool.ToolResult{
			ID:      ev.ID,
			Name:    ev.Name,
			Success: ev.Success,
			Data:    ev.Content,
		}
		injectBenchTimelineItem(tl, ev.ID, ev.Ts, tr)
		_ = idx

		// 路径选择: sizer 走生产路径 GroupByMinutes (内部调 packTimelineIntervalSubBlocksWithSizer);
		// 固定 budget 走 GroupByMinutesAndBytes (原行为, 不被 sizer 拦截)。
		var groups *TimelineGroups
		if opts.Sizer != nil {
			groups = tl.GroupByMinutes(opts.IntervalMinutes)
		} else {
			groups = tl.GroupByMinutesAndBytes(opts.IntervalMinutes, opts.Budget)
		}
		rb := groups.GetAllRenderable()
		frozenBody := rb.RenderFrozenOnly(TimelineDumpDefaultAITagName)
		openBody := rb.RenderOpenOnly(TimelineDumpDefaultAITagName)

		frozenSize := int64(len(frozenBody))
		samples = append(samples, frozenSize)
		totalSeen += frozenSize
		if frozenSize > res.MaxFrozenBytes {
			res.MaxFrozenBytes = frozenSize
		}
		// frozen 子块数: TotalInBucket * frozenBlockCount, 这里直接数 frozen blocks
		frozenBlocks := 0
		for _, blk := range rb {
			if blk != nil && !blk.IsOpen() {
				frozenBlocks++
			}
		}
		if frozenBlocks > maxSubBlocks {
			maxSubBlocks = frozenBlocks
		}

		// hash 比对
		h := sha256.Sum256([]byte(frozenBody))
		hexHash := hex.EncodeToString(h[:8])
		if hexHash != lastFrozenHash {
			if lastFrozenHash != "" || frozenSize > 0 {
				res.FlushCount++
				sumCreate += frozenSize
			}
			lastFrozenHash = hexHash
		} else {
			res.StableHitCount++
			sumHit += frozenSize
		}
		sumFrozen += frozenSize

		// 末态
		if idx == len(events)-1 {
			res.OpenBytesAtEnd = int64(len(openBody))
		}
	}

	if res.NumEvents > 0 {
		res.AvgFrozenBytes = sumFrozen / int64(res.NumEvents)
	}
	res.P95FrozenBytes = percentile(samples, 95)
	res.TotalFrozenSeen = totalSeen
	res.EstCreateCost = int64(float64(sumCreate) * opts.CreateMultiplier)
	res.EstHitSavings = int64(float64(sumHit) * opts.HitDiscount)
	res.EstNetCost = res.EstCreateCost - res.EstHitSavings
	res.FrozenSubBlockMax = maxSubBlocks

	return res
}

func budgetLabel(opts BucketBenchOptions) string {
	if opts.Sizer != nil {
		return "sizer"
	}
	switch {
	case opts.Budget < 0:
		return "no-byte-split"
	case opts.Budget == 0:
		return fmt.Sprintf("%dK(default)", TimelineDumpDefaultBucketByteSize/1024)
	default:
		return fmt.Sprintf("%dK", opts.Budget/1024)
	}
}

// injectBenchTimelineItem 在不通过 PushToolResult (避免 time.Now 干扰) 的前提下,
// 把一条 item 直接写入 timeline 的三张 ordered map。等价于
// timeline_groups_render_test.go 里的 injectTimelineItem, 但用 bench 自己的命名
// 避免污染 test 命名空间。
// 关键词: injectBenchTimelineItem, 离线重放注入
func injectBenchTimelineItem(tl *Timeline, id int64, ts time.Time, value TimelineItemValue) {
	tsMs := ts.UnixMilli()
	item := &TimelineItem{
		createdAt: ts,
		value:     value,
	}
	tl.idToTs.Set(id, tsMs)
	tl.OrderInsertId(id, item)
	tl.OrderInsertTs(tsMs, item)
}

// percentile 返回 samples 在百分位 p (0-100) 的近似值。
func percentile(samples []int64, p int) int64 {
	if len(samples) == 0 {
		return 0
	}
	cp := make([]int64, len(samples))
	copy(cp, samples)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	idx := (p * (len(cp) - 1)) / 100
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

// FormatBucketBenchTable 把多组实验结果格式化成 markdown 表格。
// 关键词: FormatBucketBenchTable, 实验报告
func FormatBucketBenchTable(results []BucketBenchResult) string {
	if len(results) == 0 {
		return "(no results)\n"
	}
	var buf bytes.Buffer
	buf.WriteString("| scenario | budget | events | flush | stable | avg-frozen | p95-frozen | max-frozen | est-create | est-hit | net-cost | sub-blocks |\n")
	buf.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, r := range results {
		buf.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %d | %s | %s | %s | %s | %s | %s | %d |\n",
			r.Scenario,
			r.BudgetLabel,
			r.NumEvents,
			r.FlushCount,
			r.StableHitCount,
			humanBytes(r.AvgFrozenBytes),
			humanBytes(r.P95FrozenBytes),
			humanBytes(r.MaxFrozenBytes),
			humanBytes(r.EstCreateCost),
			humanBytes(r.EstHitSavings),
			signedHumanBytes(r.EstNetCost),
			r.FrozenSubBlockMax,
		))
	}
	return buf.String()
}

func humanBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1fK", float64(n)/1024.0)
	}
	return fmt.Sprintf("%.2fM", float64(n)/(1024.0*1024.0))
}

func signedHumanBytes(n int64) string {
	if n < 0 {
		return "-" + humanBytes(-n)
	}
	return humanBytes(n)
}
