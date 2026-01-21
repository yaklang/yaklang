package aicommon

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"sort"
	"strings"
	"time"
)

type MemoryIntent string

// MemoryIntent describes the current "dialogue intent" (用户当前对话意图).
//
// Why:
// - 同一条记忆在不同意图下的效用不同：事实核验更看重来源/时效，建议更看重可执行性/偏好，情绪类更看重情绪线索等。
// - 这使得 C.O.R.E. P.A.C.T. 的 7 维“冷数据”能够被动态转化为当前对话中的“活认知”（Utility/Route/Inject）。
const (
	MemoryIntentGeneric    MemoryIntent = "generic"
	MemoryIntentFactCheck  MemoryIntent = "fact_check"
	MemoryIntentAdvice     MemoryIntent = "advice"
	MemoryIntentEmotional  MemoryIntent = "emotional"
	MemoryIntentBrainstorm MemoryIntent = "brainstorm"
)

type MemoryRoute string

// MemoryRoute decides "how to use a memory" in the final prompt injection.
//
// Why:
// - Inject 阶段不应该把所有 Content 平铺进 Prompt；不同记忆应扮演不同角色（约束/待确认/可执行提示/情绪上下文/关联线索/普通背景）。
// - Route 既是结构化呈现（让 LLM 快速读懂分量），也是安全策略（低可信记忆以“待确认”方式呈现，避免误用）。
const (
	// MemoryRouteMustAware : 高 Relevance + 高 Preference（或接近约束/稳定偏好），应当像“长期规则/风格约束”一样被优先遵守。
	MemoryRouteMustAware MemoryRoute = "must_aware"
	// MemoryRouteReliabilityWarning : 高 Relevance + 低 Origin，内容可能很关键但不可靠，应触发“先确认再使用”的对话策略。
	MemoryRouteReliabilityWarning MemoryRoute = "reliability_warning"
	// MemoryRouteActionTips : 高 Actionability 的经验/方法，适合进入“行动步骤/指令补充”。
	MemoryRouteActionTips MemoryRoute = "action_tips"
	// MemoryRouteEmotionalContext : 情绪线索（通常 E 低或强波动），用于同理心与沟通风格调整，而不是事实断言。
	MemoryRouteEmotionalContext MemoryRoute = "emotional_context"
	// MemoryRouteConnectionLinks : 高 Connectivity 的桥梁型记忆，用于引导关联探索/发散联想。
	MemoryRouteConnectionLinks MemoryRoute = "connection_links"
	// MemoryRouteContext : 其余作为普通背景（可被引用但权重更低）。
	MemoryRouteContext MemoryRoute = "context"
)

type MemoryRerankWeights struct {
	// WSim is the weight for vector similarity (retrieval score).
	// Why:
	// - Retrieve 给出的相似度包含了“语义相关”信号，但不能完全替代 7 维 C.O.R.E. P.A.C.T. 的策略信号；
	// - 通过 WSim + 7维权重求和，可以在不同意图下对记忆进行“策略重排”(Rerank)。
	WSim float64
	C    float64
	O    float64
	R    float64
	E    float64
	P    float64
	A    float64
	T    float64
}

// DefaultWeightsForIntent maps intent -> C.O.R.E. P.A.C.T. weights for Utility scoring.
//
// Why these mappings:
// - FactCheck: O(可信) 与 T(时效)优先，避免引用“相关但不可靠/过期”的记忆当作事实。
// - Advice: A(可执行) + P(偏好) + R(相关)优先，让建议既能落地又贴合用户风格。
// - Emotional: E(情绪) + P(偏好)优先，主要影响语气与安抚方式，不把情绪记忆当作硬约束事实。
// - Brainstorm: C(关联) + R(相关)优先，促进跨主题联想与新点子。
func DefaultWeightsForIntent(intent MemoryIntent) MemoryRerankWeights {
	switch intent {
	case MemoryIntentFactCheck:
		return MemoryRerankWeights{WSim: 0.45, O: 0.50, R: 0.30, T: 0.20}
	case MemoryIntentAdvice:
		return MemoryRerankWeights{WSim: 0.45, A: 0.40, P: 0.30, R: 0.30}
	case MemoryIntentEmotional:
		return MemoryRerankWeights{WSim: 0.35, E: 0.60, P: 0.40}
	case MemoryIntentBrainstorm:
		return MemoryRerankWeights{WSim: 0.35, C: 0.50, R: 0.50}
	default:
		return MemoryRerankWeights{WSim: 0.40, R: 0.25, C: 0.20, T: 0.15, A: 0.15, P: 0.10, O: 0.10, E: 0.05}
	}
}

type MemoryCandidate struct {
	Entity     *MemoryEntity // 记忆实体
	Similarity float64       // 向量相似度分数
	Utility    float64       // 总体效用分数（Rerank 结果）
	Route      MemoryRoute   // 注入角色路由
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// MemoryRouteThresholds centralizes all routing thresholds in one place to make tweaking easy.
//
// Why:
// - Route 是“如何使用这条记忆”的角色决策（Inject），不是“相关性排序”（Rerank）；
// - 角色阈值一旦散落在逻辑里会难以维护/调参；集中在 struct 里可以做到“只改一处”。
type MemoryRouteThresholds struct {
	ActionMinA float64
	ActionMinR float64

	MustAwareMinR float64
	MustAwareMinP float64
	MustAwareAltP float64
	MustAwareAltR float64

	ReliabilityMinR float64
	ReliabilityMaxO float64

	EmotionalMaxE float64
	EmotionalMinR float64

	ConnectionMinC float64
	ConnectionMinR float64
}

// DefaultMemoryRouteThresholds is the default routing thresholds.
// Change this to tune routing globally, or pass a custom value via WithMemoryInjectRouteThresholds / WithMemoryInjectRouteThresholdsPatch.
var DefaultMemoryRouteThresholds = MemoryRouteThresholds{
	ActionMinA: 0.80,
	ActionMinR: 0.45,

	MustAwareMinR: 0.70,
	MustAwareMinP: 0.75,
	MustAwareAltP: 0.85,
	MustAwareAltR: 0.55,

	ReliabilityMinR: 0.70,
	ReliabilityMaxO: 0.40,

	EmotionalMaxE: 0.35,
	EmotionalMinR: 0.50,

	ConnectionMinC: 0.80,
	ConnectionMinR: 0.40,
}

type MemoryRouteMatches struct {
	ActionTips         bool
	MustAware          bool
	ReliabilityWarning bool
	EmotionalContext   bool
	ConnectionLinks    bool
}

// MatchMemoryRoutes extracts the "role judgement" from routeForIntent and makes it configurable.
//
// Why:
// - 保持 routeForIntent 足够薄：只负责按优先级选择角色；
// - 角色判定逻辑集中在这里，方便未来增加新角色/调阈值/做 AB 测试；
// - intent 会影响部分判定（如 Emotional 在情绪对话中更容易被采用）。
func MatchMemoryRoutes(e *MemoryEntity, intent MemoryIntent, th MemoryRouteThresholds) MemoryRouteMatches {
	if e == nil {
		return MemoryRouteMatches{}
	}

	action := clamp01(e.A_Score) >= th.ActionMinA && clamp01(e.R_Score) >= th.ActionMinR
	mustAware := (clamp01(e.R_Score) >= th.MustAwareMinR && clamp01(e.P_Score) >= th.MustAwareMinP) ||
		(clamp01(e.P_Score) >= th.MustAwareAltP && clamp01(e.R_Score) >= th.MustAwareAltR)
	reliabilityWarning := clamp01(e.R_Score) >= th.ReliabilityMinR && clamp01(e.O_Score) <= th.ReliabilityMaxO

	emotional := clamp01(e.E_Score) <= th.EmotionalMaxE && (intent == MemoryIntentEmotional || clamp01(e.R_Score) >= th.EmotionalMinR)
	connection := clamp01(e.C_Score) >= th.ConnectionMinC && clamp01(e.R_Score) >= th.ConnectionMinR

	return MemoryRouteMatches{
		ActionTips:         action,
		MustAware:          mustAware,
		ReliabilityWarning: reliabilityWarning,
		EmotionalContext:   emotional,
		ConnectionLinks:    connection,
	}
}

// RerankAndRouteMemories performs:
// - Rerank: compute Utility = similarity*WSim + Σ(score_i * weight_i)
// - Route: decide the injection role (MemoryRoute) for each memory based on its score pattern
//
// Note:
// - 权重只用于“排序/筛选优先级”，路由阈值用于“用法选择”，两者分离能避免把不可靠记忆当作硬约束注入。
func RerankAndRouteMemories(results []*SearchResult, intent MemoryIntent) []*MemoryCandidate {
	return RerankAndRouteMemoriesWithThresholds(results, intent, DefaultMemoryRouteThresholds)
}

func RerankAndRouteMemoriesWithThresholds(results []*SearchResult, intent MemoryIntent, thresholds MemoryRouteThresholds) []*MemoryCandidate {
	if len(results) == 0 {
		return nil
	}

	weights := DefaultWeightsForIntent(intent)
	candidates := make([]*MemoryCandidate, 0, len(results))
	for _, r := range results {
		if r == nil || r.Entity == nil {
			continue
		}
		sim := clamp01(r.Score)
		utility := sim*weights.WSim +
			clamp01(r.Entity.C_Score)*weights.C +
			clamp01(r.Entity.O_Score)*weights.O +
			clamp01(r.Entity.R_Score)*weights.R +
			clamp01(r.Entity.E_Score)*weights.E +
			clamp01(r.Entity.P_Score)*weights.P +
			clamp01(r.Entity.A_Score)*weights.A +
			clamp01(r.Entity.T_Score)*weights.T

		candidates = append(candidates, &MemoryCandidate{
			Entity:     r.Entity,
			Similarity: sim,
			Utility:    utility,
			Route:      routeForIntent(r.Entity, intent, thresholds),
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Utility > candidates[j].Utility
	})
	return candidates
}

type MemoryInjectConfig struct {
	Now             time.Time
	MaxTotal        int
	MaxPerRoute     int
	MaxContentRunes int
	MinUtility      float64
	RouteThresholds MemoryRouteThresholds
}

// MemoryInjectOption is a functional option used to configure memory injection.
//
// Why:
// - 便于在调用点按需覆写少量参数（例如只改 MaxTotal 或只微调阈值）；
// - 避免传入一个“看起来必须全填”的大 struct，同时方便后续扩展字段不破坏 API。
type MemoryInjectOption func(*MemoryInjectConfig)

func NewDefaultMemoryInjectConfig() *MemoryInjectConfig {
	return &MemoryInjectConfig{
		Now:             time.Now(),
		MaxTotal:        12,
		MaxPerRoute:     4,
		MaxContentRunes: 200,
		MinUtility:      0,
		RouteThresholds: DefaultMemoryRouteThresholds,
	}
}

func WithMemoryInjectNow(now time.Time) MemoryInjectOption {
	return func(c *MemoryInjectConfig) {
		if c == nil {
			return
		}
		if !now.IsZero() {
			c.Now = now
		}
	}
}

func WithMemoryInjectMaxTotal(maxTotal int) MemoryInjectOption {
	return func(c *MemoryInjectConfig) {
		if c == nil {
			return
		}
		c.MaxTotal = maxTotal
	}
}

func WithMemoryInjectMaxPerRoute(maxPerRoute int) MemoryInjectOption {
	return func(c *MemoryInjectConfig) {
		if c == nil {
			return
		}
		c.MaxPerRoute = maxPerRoute
	}
}

func WithMemoryInjectMaxContentRunes(maxContentRunes int) MemoryInjectOption {
	return func(c *MemoryInjectConfig) {
		if c == nil {
			return
		}
		c.MaxContentRunes = maxContentRunes
	}
}

func WithMemoryInjectMinUtility(minUtility float64) MemoryInjectOption {
	return func(c *MemoryInjectConfig) {
		if c == nil {
			return
		}
		c.MinUtility = minUtility
	}
}

func WithMemoryInjectRouteThresholds(thresholds MemoryRouteThresholds) MemoryInjectOption {
	return func(c *MemoryInjectConfig) {
		if c == nil {
			return
		}
		c.RouteThresholds = thresholds
	}
}

// WithMemoryInjectRouteThresholdsPatch allows partial tuning without copying the whole thresholds struct.
func WithMemoryInjectRouteThresholdsPatch(patch func(*MemoryRouteThresholds)) MemoryInjectOption {
	return func(c *MemoryInjectConfig) {
		if c == nil || patch == nil {
			return
		}
		patch(&c.RouteThresholds)
	}
}

// BuildPromptMemoriesMarkdown converts a batch of memories into a prompt-ready Markdown block.
// It performs: Retrieve (accept pre-retrieved results) -> Rerank (Utility by intent) -> Inject (route + format).
func BuildPromptMemoriesMarkdown(results []*SearchResult, intent MemoryIntent, opts ...MemoryInjectOption) string {
	cfg := NewDefaultMemoryInjectConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	candidates := RerankAndRouteMemoriesWithThresholds(results, intent, cfg.RouteThresholds)
	if len(candidates) == 0 {
		return ""
	}

	routeOrder := routeOrderForIntent(intent)
	byRoute := make(map[MemoryRoute][]*MemoryCandidate, len(routeOrder))
	total := 0

	for _, c := range candidates {
		if c == nil || c.Entity == nil {
			continue
		}
		if cfg.MinUtility > 0 && c.Utility < cfg.MinUtility {
			continue
		}
		byRoute[c.Route] = append(byRoute[c.Route], c)
	}

	var buf bytes.Buffer
	buf.WriteString("### Retrieved Memories (Contextual)\n\n")

	for _, route := range routeOrder {
		list := byRoute[route]
		if len(list) == 0 {
			continue
		}
		buf.WriteString("[ " + string(route) + " ]\n")

		perRoute := 0
		for _, c := range list {
			if total >= cfg.MaxTotal || perRoute >= cfg.MaxPerRoute {
				break
			}
			line := formatMemoryBullet(route, c, cfg.Now, cfg.MaxContentRunes)
			if line == "" {
				continue
			}
			buf.WriteString("- " + line + "\n")
			perRoute++
			total++
		}
		buf.WriteString("\n")
		if total >= cfg.MaxTotal {
			break
		}
	}

	out := strings.TrimSpace(buf.String())
	if out == "### Retrieved Memories (Contextual)" {
		return ""
	}
	return out + "\n"
}

func BuildPromptMemoriesMarkdownFromEntities(entities []*MemoryEntity, intent MemoryIntent, opts ...MemoryInjectOption) string {
	if len(entities) == 0 {
		return ""
	}
	results := make([]*SearchResult, 0, len(entities))
	for _, e := range entities {
		if e == nil {
			continue
		}
		results = append(results, &SearchResult{Entity: e, Score: 0})
	}
	return BuildPromptMemoriesMarkdown(results, intent, opts...)
}

// routeOrderForIntent defines the priority of "memory roles" in injection.
//
// Why:
// - Prompt 空间有限，且不同意图下“先看到什么”会显著影响模型的决策路径；
// - 例如 Advice 优先把可执行建议/约束放前面，FactCheck 优先把“低可信但高相关”的待确认放前面以触发核验流程；
// - 这相当于 Inject 阶段的“结构化注意力分配”。
func routeOrderForIntent(intent MemoryIntent) []MemoryRoute {
	switch intent {
	case MemoryIntentAdvice:
		return []MemoryRoute{MemoryRouteActionTips, MemoryRouteMustAware, MemoryRouteReliabilityWarning, MemoryRouteContext, MemoryRouteConnectionLinks, MemoryRouteEmotionalContext}
	case MemoryIntentFactCheck:
		return []MemoryRoute{MemoryRouteReliabilityWarning, MemoryRouteMustAware, MemoryRouteContext, MemoryRouteActionTips, MemoryRouteConnectionLinks, MemoryRouteEmotionalContext}
	case MemoryIntentEmotional:
		return []MemoryRoute{MemoryRouteEmotionalContext, MemoryRouteMustAware, MemoryRouteContext, MemoryRouteReliabilityWarning, MemoryRouteActionTips, MemoryRouteConnectionLinks}
	case MemoryIntentBrainstorm:
		return []MemoryRoute{MemoryRouteConnectionLinks, MemoryRouteContext, MemoryRouteMustAware, MemoryRouteActionTips, MemoryRouteReliabilityWarning, MemoryRouteEmotionalContext}
	default:
		return []MemoryRoute{MemoryRouteMustAware, MemoryRouteActionTips, MemoryRouteReliabilityWarning, MemoryRouteContext, MemoryRouteConnectionLinks, MemoryRouteEmotionalContext}
	}
}

// routeForIntent maps a memory's score pattern to a MemoryRoute.
//
// Why:
// - 同一个实体的 Content 不变，但“该如何用”完全取决于 7 维数据：高 A 更像操作提示，高 P 更像长期约束，低 O 则需要先确认；
// - 这里用阈值做“角色判断”，并且按 intent 的 routeOrder 决定优先落入哪个角色，避免一条记忆同时命中多个角色时产生混乱。
func routeForIntent(e *MemoryEntity, intent MemoryIntent, thresholds MemoryRouteThresholds) MemoryRoute {
	if e == nil {
		return MemoryRouteContext
	}

	matches := MatchMemoryRoutes(e, intent, thresholds)

	// 再根据 intent 的 routeOrder 决定最终落入哪个角色
	for _, route := range routeOrderForIntent(intent) {
		switch route {
		case MemoryRouteActionTips:
			if matches.ActionTips {
				return MemoryRouteActionTips
			}
		case MemoryRouteMustAware:
			if matches.MustAware {
				return MemoryRouteMustAware
			}
		case MemoryRouteReliabilityWarning:
			if matches.ReliabilityWarning {
				return MemoryRouteReliabilityWarning
			}
		case MemoryRouteEmotionalContext:
			if matches.EmotionalContext {
				return MemoryRouteEmotionalContext
			}
		case MemoryRouteConnectionLinks:
			if matches.ConnectionLinks {
				return MemoryRouteConnectionLinks
			}
		}
	}

	return MemoryRouteContext
}

func formatMemoryBullet(route MemoryRoute, c *MemoryCandidate, now time.Time, maxContentRunes int) string {
	if c == nil || c.Entity == nil {
		return ""
	}

	content := utils.ShrinkString(c.Entity.Content, maxContentRunes)
	if content == "" {
		return ""
	}

	prefix := ""
	keys := []string{"u", "sim"}

	switch route {
	case MemoryRouteActionTips:
		prefix = "经验/可执行提示："
		keys = append(keys, "A", "R", "T")
	case MemoryRouteMustAware:
		prefix = "关键偏好/约束："
		keys = append(keys, "P", "R")
	case MemoryRouteReliabilityWarning:
		prefix = "待确认（低可信但高相关）："
		keys = append(keys, "O", "R")
	case MemoryRouteEmotionalContext:
		prefix = "情绪线索："
		keys = append(keys, "E", "R")
	case MemoryRouteConnectionLinks:
		prefix = "关联线索："
		keys = append(keys, "C", "R")
	default:
		prefix = "背景："
		keys = append(keys, "R", "T")
	}

	meta := formatMeta(c, now, keys)
	if meta != "" {
		return prefix + content + " " + meta
	}
	return prefix + content
}

func formatMeta(c *MemoryCandidate, now time.Time, keys []string) string {
	if c == nil || c.Entity == nil {
		return ""
	}
	parts := make([]string, 0, len(keys)+2)

	for _, k := range keys {
		switch k {
		case "u":
			parts = append(parts, fmt.Sprintf("u=%.2f", c.Utility))
		case "sim":
			if c.Similarity > 0 {
				parts = append(parts, fmt.Sprintf("sim=%.2f", c.Similarity))
			}
		case "C":
			parts = append(parts, fmt.Sprintf("C=%.2f", clamp01(c.Entity.C_Score)))
		case "O":
			parts = append(parts, fmt.Sprintf("O=%.2f", clamp01(c.Entity.O_Score)))
		case "R":
			parts = append(parts, fmt.Sprintf("R=%.2f", clamp01(c.Entity.R_Score)))
		case "E":
			parts = append(parts, fmt.Sprintf("E=%.2f", clamp01(c.Entity.E_Score)))
		case "P":
			parts = append(parts, fmt.Sprintf("P=%.2f", clamp01(c.Entity.P_Score)))
		case "A":
			parts = append(parts, fmt.Sprintf("A=%.2f", clamp01(c.Entity.A_Score)))
		case "T":
			parts = append(parts, fmt.Sprintf("T=%.2f", clamp01(c.Entity.T_Score)))
		}
	}

	if !c.Entity.CreatedAt.IsZero() && !now.IsZero() {
		parts = append(parts, "age="+now.Sub(c.Entity.CreatedAt).Round(time.Second).String())
	}
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, ", ") + ")"
}
