package trafficguard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	fromScriptTag = "trafficguard"
	// flowTag 是写入 HTTPFlow.Tags 的内置标签(便于在 History 中筛选 trafficguard 命中的流量)。
	flowTag = "trafficguard-secret"
)

// severityForSchema 把 trafficguard 内部等级映射为 schema.Risk 的 severity 取值。
// yaklib 的 WithRiskParam_Severity 接受 high/critical/warning/low 等。
func severityForSchema(sev string) string {
	switch sev {
	case severityCritical:
		return "critical"
	case severityHigh:
		return "high"
	case severityMedium:
		return "warning"
	default:
		return "low"
	}
}

// BuildRisk 把单个 Finding 构造为一条 schema.Risk(不落库)。
// target 为目标 URL; request/response 为原始报文(用于 Risk 回溯定位)。
func BuildRisk(f Finding, target string, request, response []byte) *schema.Risk {
	// 标题中带上目标 Host/Path, 让用户一眼知道是哪个目标泄漏了凭证。
	title := fmt.Sprintf("[%s] %s @ %s", severityVerbose(f.Severity), f.RuleName, hostPathOf(target))
	riskType := "info-exposure" // 敏感信息泄漏
	if f.Category == "private-key" || f.Category == "connection-string" {
		riskType = "info-exposure"
	}

	r := &schema.Risk{
		Title:           title,
		TitleVerbose:    title,
		RiskType:        riskType,
		RiskTypeVerbose: "敏感信息泄漏",
		Severity:        severityForSchema(f.Severity),
		Description:     f.Description,
		Solution:        f.Solution,
		Parameter:       f.Direction + "/" + f.Surface,
		Tags:            strings.Join([]string{fromScriptTag, f.Category, "builtin-mitm-rules"}, "|"),
		FromYakScript:   fromScriptTag,
		YakScriptUUID:   fmt.Sprintf("builtin-trafficguard-%d", f.RuleID),
	}

	if len(request) > 0 {
		r.QuotedRequest = string(request)
	}
	if len(response) > 0 {
		r.QuotedResponse = string(response)
	}

	// details 写入脱敏后的命中信息 + 指纹,绝不写完整明文。
	r.Details = fmt.Sprintf(
		`{"source":"trafficguard","rule_id":%d,"rule_name":%q,"category":%q,"severity":%q,`+
			`"direction":%q,"surface":%q,"masked_value":%q,"fingerprint":%q,"value_length":%d,"detected_at":%q}`,
		f.RuleID, f.RuleName, f.Category, f.Severity, f.Direction, f.Surface,
		f.MaskedValue, f.Fingerprint, f.ValueLength, time.Now().Format(time.RFC3339),
	)

	// 用稳定指纹生成 hash,使同一(规则+目标+命中值指纹)在 CreateOrUpdateRisk 里去重,
	// 避免重复刷屏; 同一明文只在目标变化或新凭证出现时新增记录。
	r.Hash = stableHash(fmt.Sprintf("%d|%s|%s", f.RuleID, target, f.Fingerprint))
	return r
}

func stableHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

var riskBuiltin = schema.Risk{}

// SaveRisksForHTTPFlow 扫描一次 HTTP 事务并把命中落库为 Risk。
//
// 这是 MITM 集成点调用的总入口: 扫描 request/response -> 去重 -> 逐条构造 Risk ->
// 经 yakit.CreateOrUpdateRisk 按 hash 去重写入 db。
//
// 该函数对任何错误都降级为"仅记录日志、不阻断",保证绝不影响正常代理流量(fail-open)。
func SaveRisksForHTTPFlow(db *gorm.DB, url string, request, response []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("trafficguard: save risks panic recovered (fail-open): %v", r)
		}
	}()
	s := DefaultScanner()
	if s == nil || !s.Ready() {
		return
	}
	// 大报文做软上限,避免极端输入拖垮热路径(本组只匹配有界特征,不会因此漏报高危)。
	req, rsp := capForScan(request, response)
	findings := Dedup(s.ScanHTTPFlow(req, rsp))
	if len(findings) == 0 {
		return
	}
	for _, f := range findings {
		r := BuildRisk(f, url, request, response)
		if db == nil {
			// 无 db 时退化为全局库(与 yakit.SaveRisk 一致)。
			if _, err := yakit.NewRisk(url,
				yakit.WithRiskParam_Title(r.Title),
				yakit.WithRiskParam_RiskType(r.RiskType),
				yakit.WithRiskParam_Severity(r.Severity),
				yakit.WithRiskParam_Description(r.Description),
				yakit.WithRiskParam_Solution(r.Solution),
				yakit.WithRiskParam_Tags(r.Tags),
				yakit.WithRiskParam_FromScript(r.FromYakScript),
			); err != nil {
				log.Errorf("trafficguard: save risk failed: %v", err)
			}
			continue
		}
		if err := yakit.CreateOrUpdateRisk(db, r.Hash, r); err != nil {
			log.Errorf("trafficguard: save risk failed: %v", err)
		}
	}
	_ = riskBuiltin
}

// MarkAndSaveRisksForFlow 是 MITM 镜像保存 flow 时调用的总入口。
//
// 它做两件事:
//  1. 用内置超级正则组扫描 flow 的请求/响应; 命中即把该 HTTPFlow 标红(RED 颜色)
//     并打上 trafficguard 内置 TAG, 让用户在 HTTP History 第一眼就看到红色高危流量;
//  2. 同时为每个命中生成"高危/中危" Risk(标题含 Host/Path), 写入漏洞库。
//
// 关键: 该检测不受 MITM 过滤影响(在 flow 保存路径上无条件执行), 且默认开启不可关闭。
// 任何异常都 fail-open(仅记日志、绝不阻断代理流量)。
func MarkAndSaveRisksForFlow(db *gorm.DB, flow *schema.HTTPFlow, request, response []byte) {
	if flow == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("trafficguard: mark flow panic recovered (fail-open): %v", r)
		}
	}()
	s := DefaultScanner()
	if s == nil || !s.Ready() {
		return
	}
	// 扫描 + 应用一次性完成(MITM 之外的便捷入口)。
	findings := ScanFindings(request, response)
	ApplyToFlow(db, flow, findings, request, response)
}

// ScanFindings 只做扫描(无副作用): 对请求/响应跑两阶段检测并去重, 返回命中集合。
//
// 设计为可独立调用: MITM 流水线可在"过滤判定之前"调用它拿到 findings,
// 再据此决定是否强制保留流量(见 ApplyToFlow)。这样敏感流量永不被过滤丢弃。
func ScanFindings(request, response []byte) []Finding {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("trafficguard: scan panic recovered (fail-open): %v", r)
		}
	}()
	s := DefaultScanner()
	if s == nil || !s.Ready() {
		return nil
	}
	req, rsp := capForScan(request, response)
	return Dedup(s.ScanHTTPFlow(req, rsp))
}

// ApplyToFlow 把已扫描得到的 findings 应用到一个 HTTPFlow: 标红 + 打 TAG + 生成合并 Risk。
//
// 与 ScanFindings 分离是为了: MITM 可先扫描(过滤前), 命中则强制保留流量,
// 再在 flow 对象构建完成后调用本函数复用 findings(避免二次扫描)。
// findings 为空时为 no-op。
func ApplyToFlow(db *gorm.DB, flow *schema.HTTPFlow, findings []Finding, request, response []byte) {
	if flow == nil || len(findings) == 0 {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("trafficguard: apply flow panic recovered (fail-open): %v", r)
		}
	}()

	// 1) 把流量标红 + 打内置 TAG, 让用户在 History 一眼可见并可用 TAG 筛选。
	//    流量本身存在感比 Risk 更强, 红色是最直接的"这里有敏感数据"信号。
	flow.Red()
	flow.AddTagToFirst(flowTag)

	// 2) 一个流量(请求+响应)只合并为一条 Risk: 取最高危险等级作为整体等级,
	//    标题/描述里聚合所有命中的规则与脱敏值, 让用户通过单条 Risk 就能看清这条流量泄漏了什么。
	r := BuildMergedRisk(findings, flow.Url, request, response)
	if r == nil {
		return
	}
	if db == nil {
		if _, err := yakit.NewRisk(flow.Url,
			yakit.WithRiskParam_Title(r.Title),
			yakit.WithRiskParam_RiskType(r.RiskType),
			yakit.WithRiskParam_Severity(r.Severity),
			yakit.WithRiskParam_Description(r.Description),
			yakit.WithRiskParam_Solution(r.Solution),
			yakit.WithRiskParam_Tags(r.Tags),
			yakit.WithRiskParam_FromScript(r.FromYakScript),
		); err != nil {
			log.Errorf("trafficguard: save risk failed: %v", err)
		}
		return
	}
	if err := yakit.CreateOrUpdateRisk(db, r.Hash, r); err != nil {
		log.Errorf("trafficguard: save risk failed: %v", err)
	}
}

// severityRank 给危险等级排序: 越高数字越大, 用于合并时取最高等级。
func severityRank(sev string) int {
	switch sev {
	case severityCritical:
		return 4
	case severityHigh:
		return 3
	case severityMedium:
		return 2
	default:
		return 1
	}
}

// BuildMergedRisk 把一个流量(请求+响应)的全部命中合并为单条 schema.Risk。
//
// 合并策略:
//   - 整体 Severity 取命中里最高的等级;
//   - Title 以最高等级 + 命中规则名(去重, 最多列3个) + 目标 Host/Path 组成, 一眼看清"哪泄漏了啥";
//   - Description/Solution 聚合所有命中规则的说明与修复建议;
//   - Details 写入每条命中的脱敏值与指纹(绝不含明文), 便于追溯;
//   - Hash 基于(目标 + 所有命中指纹的集合), 同一流量去重、新凭证才更新。
func BuildMergedRisk(findings []Finding, target string, request, response []byte) *schema.Risk {
	if len(findings) == 0 {
		return nil
	}
	host := hostPathOf(target)

	// 取最高危险等级 + 收集去重的规则名/类别。
	topRank := 0
	topSev := severityMedium
	seenRules := make(map[string]struct{})
	seenCat := make(map[string]struct{})
	var ruleNames []string
	var cats []string
	for _, f := range findings {
		if rk := severityRank(f.Severity); rk > topRank {
			topRank = rk
			topSev = f.Severity
		}
		if _, ok := seenRules[f.RuleName]; !ok {
			seenRules[f.RuleName] = struct{}{}
			ruleNames = append(ruleNames, f.RuleName)
		}
		if _, ok := seenCat[f.Category]; !ok {
			seenCat[f.Category] = struct{}{}
			cats = append(cats, f.Category)
		}
	}
	_ = topRank

	// 标题: [严重度] 命中规则名(最多列3个, 超出用"+N") @ host/path
	shown := ruleNames
	more := 0
	if len(shown) > 3 {
		shown, more = shown[:3], len(shown)-3
	}
	titleRule := strings.Join(shown, " / ")
	if more > 0 {
		titleRule = fmt.Sprintf("%s +%d", titleRule, more)
	}
	title := fmt.Sprintf("[%s] 敏感凭证泄漏: %s @ %s", severityVerbose(topSev), titleRule, host)

	// 描述: 聚合每条命中的说明 + 脱敏展示(不含明文)。
	var descB strings.Builder
	fmt.Fprintf(&descB, "流量命中 %d 个内置高危特征(类别: %s):\n", len(findings), strings.Join(cats, ", "))
	for _, f := range findings {
		fmt.Fprintf(&descB, "\n• [%s][%s] %s\n  方向: %s, 脱敏值: %s, 长度: %d, 指纹: %s",
			severityVerbose(f.Severity), f.RuleName, f.Description, f.Direction, f.MaskedValue, f.ValueLength, f.Fingerprint)
	}

	// 修复建议: 聚合去重。
	seenSol := make(map[string]struct{})
	var sols []string
	for _, f := range findings {
		if _, ok := seenSol[f.Solution]; !ok {
			seenSol[f.Solution] = struct{}{}
			sols = append(sols, f.Solution)
		}
	}

	// details: 每条命中一项, 仅脱敏值 + 指纹。
	type detItem struct {
		RuleID      int    `json:"rule_id"`
		RuleName    string `json:"rule_name"`
		Category    string `json:"category"`
		Severity    string `json:"severity"`
		Direction   string `json:"direction"`
		MaskedValue string `json:"masked_value"`
		Fingerprint string `json:"fingerprint"`
		ValueLength int    `json:"value_length"`
	}
	items := make([]detItem, 0, len(findings))
	for _, f := range findings {
		items = append(items, detItem{f.RuleID, f.RuleName, f.Category, f.Severity, f.Direction, f.MaskedValue, f.Fingerprint, f.ValueLength})
	}
	detailsRaw, _ := json.Marshal(struct {
		Source     string    `json:"source"`
		Target     string    `json:"target"`
		Severity   string    `json:"severity"`
		Findings   []detItem `json:"findings"`
		DetectedAt string    `json:"detected_at"`
	}{fromScriptTag, target, topSev, items, time.Now().Format(time.RFC3339)})

	// 合并的 hash: 目标 + 排序后的全部命中指纹, 保证同一流量只有一条 Risk、新凭证才更新。
	fpSet := make(map[string]struct{})
	for _, f := range findings {
		fpSet[f.Fingerprint] = struct{}{}
	}
	fps := make([]string, 0, len(fpSet))
	for fp := range fpSet {
		fps = append(fps, fp)
	}
	sort.Strings(fps)

	r := &schema.Risk{
		Title:           title,
		TitleVerbose:    title,
		RiskType:        "info-exposure",
		RiskTypeVerbose: "敏感信息泄漏",
		Severity:        severityForSchema(topSev),
		Description:     descB.String(),
		Solution:        strings.Join(sols, "\n\n"),
		Parameter:       fmt.Sprintf("%d 项命中 (请求+响应)", len(findings)),
		Tags:            strings.Join(append([]string{fromScriptTag, "builtin-mitm-rules"}, cats...), "|"),
		FromYakScript:   fromScriptTag,
		YakScriptUUID:   "builtin-trafficguard",
		Hash:            stableHash(target + "|" + strings.Join(fps, ",")),
	}
	if len(request) > 0 {
		r.QuotedRequest = string(request)
	}
	if len(response) > 0 {
		r.QuotedResponse = string(response)
	}
	r.Details = string(detailsRaw)
	return r
}

// hostPathOf 从目标 URL 中提取 "host + path" 的简短展示串, 用于 Risk 标题。
// 解析失败时原样返回, 保证标题始终有可读内容。
func hostPathOf(target string) string {
	if target == "" {
		return "unknown"
	}
	if u, err := url.Parse(target); err == nil && u != nil && u.Host != "" {
		p := u.Path
		if p == "" {
			p = "/"
		}
		// path 过长则截断, 保持标题简洁。
		if len(p) > 48 {
			p = p[:48] + "…"
		}
		return u.Host + p
	}
	// 不是完整 URL, 尝试按首个空白/分号截断, 去掉可能的 query 残留。
	if i := strings.IndexAny(target, " \t;?"); i > 0 {
		return target[:i]
	}
	return target
}

// capForScan 对待扫描报文做软上限,超长部分截断; 同时返回原始报文用于 Risk 回溯。
// 上限取 2MiB: 足够覆盖常规 API/页面,且超级正则组只匹配有界特征,截断不影响高危命中。
const scanCapBytes = 2 * 1024 * 1024

func capForScan(request, response []byte) ([]byte, []byte) {
	req := request
	rsp := response
	if len(req) > scanCapBytes {
		req = req[:scanCapBytes]
	}
	if len(rsp) > scanCapBytes {
		rsp = rsp[:scanCapBytes]
	}
	return req, rsp
}
