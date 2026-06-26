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
	"unicode/utf8"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	fromScriptTag = "trafficguard"
	// flowTag 是写入 HTTPFlow.Tags 的内置"总括"标签(便于在 History 中一键筛选全部 trafficguard 命中的流量)。
	flowTag = "trafficguard-secret"
	// ruleTagPrefix 是"具体规则名"标签 / extracted_data RuleVerbose 的统一前缀。
	// 既让用户能在 History 按命中的具体规则筛选, 又能一眼看出归属(都以 TrafficGuard: 开头)。
	ruleTagPrefix = "TrafficGuard: "
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

// cappedSeverity 对落库 Risk 的严重度施加上限(SeverityCeiling, 即最高中危)。
// 内置敏感信息检测定位为"线索提示", 不在被动扫描里产生高危/严重告警。
func cappedSeverity(sev string) string {
	if severityRank(sev) > severityRank(SeverityCeiling) {
		return SeverityCeiling
	}
	return sev
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
		Severity:        severityForSchema(cappedSeverity(f.Severity)),
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
	findings := Dedup(s.ScanHTTPFlow(hostOf(url), req, rsp))
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
	findings := ScanFindings(flow.Url, request, response)
	ApplyToFlow(db, flow, findings, request, response)
}

// ScanFindings 只做扫描(无副作用): 对请求/响应跑多阶段检测并去重, 返回命中集合。
//
// target 为目标 URL(或 host); 用于第三阶段上下文校验(厂商自有域抑制等)。
// 设计为可独立调用: MITM 流水线可在"过滤判定之前"调用它拿到 findings,
// 再据此决定是否把流量以"插件流量"形式保留(见 ApplyToFlow)。这样敏感流量永不被过滤丢弃。
func ScanFindings(target string, request, response []byte) []Finding {
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
	return Dedup(s.ScanHTTPFlow(hostOf(target), req, rsp))
}

// hostOf 从 target(URL 或 host) 解析出小写、无端口的 host, 供第三阶段上下文校验使用。
// 解析失败时返回空串(此时跳过依赖 host 的校验)。
func hostOf(target string) string {
	if target == "" {
		return ""
	}
	h := ""
	if u, err := url.Parse(target); err == nil && u != nil && u.Host != "" {
		h = u.Host
	} else {
		h = target
	}
	h = strings.ToLower(h)
	if i := strings.IndexByte(h, '/'); i >= 0 {
		h = h[:i]
	}
	if i := strings.IndexByte(h, ':'); i >= 0 {
		h = h[:i]
	}
	return h
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
	//    - flowTag(trafficguard-secret): 总括标签, 一键筛选全部命中流量;
	//    - ruleTagsOf(findings): 具体规则名标签(去重), 让用户能按"到底命中了哪条规则"精确筛选。
	flow.Red()
	flow.AddTagToFirst(flowTag)
	flow.AddTag(ruleTagsOf(findings)...)

	// 1.5) 按 yaklang 既有 MITM 规则标注机制, 为每条命中写一条 extracted_data(进"提取数据"专表),
	//      据 DataIndex/Length 在报文里高亮; 同时把命中内容与上下文写入 flow.Payload, 便于在流量列表直接感知。
	annotateFlowWithFindings(flow, findings, request, response)

	// 2) 一个流量(请求+响应)只合并为一条 Risk: 取最高危险等级(再压到上限)作为整体等级,
	//    标题/描述里聚合所有命中, Description 用 markdown 给出命中值与上下文, 让人一眼判真假。
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
//   - 整体 Severity 取命中里最高的等级, 再经 cappedSeverity 压到上限(最高中危);
//   - Title 以严重度 + 命中规则名(去重, 最多列3个) + 目标 Host/Path 组成, 一眼看清"哪泄漏了啥";
//   - Description 用 markdown 给出每条命中的"命中值 + 前后上下文", 让人一眼判真假(真凭证 or 源码/埋点误报);
//   - Solution 聚合去重; Details 写入脱敏值与指纹(机器可读, 不含完整明文);
//   - Hash 基于(host/path + 排序后的命中规则ID集合), 同一接口的重复命中去重(降频),
//     避免埋点等"每次 query 不同"的流量反复刷新 Risk。
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
	// 落库严重度受上限约束(最高中危); 标题展示同一上限后的等级。
	cappedSev := cappedSeverity(topSev)
	title := fmt.Sprintf("[%s] 敏感信息泄漏线索: %s @ %s", severityVerbose(cappedSev), titleRule, host)

	// 描述: 用 markdown 给出每条命中的"命中值 + 前后上下文", 让人一眼判真假(真凭证 or 源码/埋点误报)。
	descB := buildMarkdownDescription(findings, host, cats, request, response)

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

	// 合并的 hash: host/path + 排序后的命中规则ID集合(降频去重)。
	// 用 host/path(不含 query)而非完整 target, 使同一接口的重复命中(埋点等每次 query 不同)收敛为一条 Risk,
	// 经 CreateOrUpdateRisk 按 hash 去重更新, 而非不断新建, 显著降低刷屏频率。
	idSet := make(map[int]struct{})
	for _, f := range findings {
		idSet[f.RuleID] = struct{}{}
	}
	ids := make([]int, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	idParts := make([]string, 0, len(ids))
	for _, id := range ids {
		idParts = append(idParts, itoa(id))
	}

	r := &schema.Risk{
		Title:           title,
		TitleVerbose:    title,
		RiskType:        "info-exposure",
		RiskTypeVerbose: "敏感信息泄漏",
		Severity:        severityForSchema(cappedSev),
		Description:     descB,
		Solution:        strings.Join(sols, "\n\n"),
		Parameter:       fmt.Sprintf("%d 项命中 (请求+响应)", len(findings)),
		Tags:            strings.Join(append([]string{fromScriptTag, "builtin-mitm-rules"}, cats...), "|"),
		FromYakScript:   fromScriptTag,
		YakScriptUUID:   "builtin-trafficguard",
		Hash:            stableHash(host + "|" + strings.Join(idParts, ",")),
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

// annotateFlowWithFindings 按 yaklang 既有 MITM 规则标注机制为命中流量打标:
//   - 每条命中写一条 schema.ExtractedData(SourceType=httpflow, TraceId=flow.HiddenIndex,
//     DataIndex/Length 与 HookColor 一致, 供前端在报文里高亮), 进入"提取数据"专表;
//   - 把命中内容(规则名 + 命中值)写入 flow.Payload, 让流量列表一眼可见提取到了什么。
//
// 关键词: ExtractedData 标注, MITM 规则提取数据, flow.Payload 上下文
func annotateFlowWithFindings(flow *schema.HTTPFlow, findings []Finding, request, response []byte) {
	if flow == nil || len(findings) == 0 {
		return
	}
	trace := flow.HiddenIndex
	payloads := make([]string, 0, len(findings))
	for _, f := range findings {
		buf := response
		if f.Direction == "request" {
			buf = request
		}
		raw := extractRaw(f, buf)
		hitDisp := clipForDisplay(sanitizeInline(string(raw)), 160)

		// extracted_data: 仅在有 trace(HiddenIndex)时落库, 复用工程库异步写入(与 HookColor 一致)。
		if trace != "" {
			ruleRegex := ""
			if rule := builtinRuleByID[f.RuleID]; rule != nil {
				ruleRegex = rule.Regex
			}
			// DataIndex/Length 必须是 rune(字符)下标, 才能与 yaklang HookColor 及前端高亮约定一致:
			// go-pcre2-lite/regexp2 的 Capture.Index/Length 报告的是 rune 下标(HookColor 据此写 extracted_data),
			// 而 Finding.From/To 是 PCRE2 底层接口给出的 byte 偏移。报文里命中点之前若有多字节字符(如中文注释/字符串),
			// 直接用 byte 偏移会让前端高亮整体右移(用户看到的"偏移/颜色错位"即源于此)。这里统一换算为 rune 下标。
			dataIndex, dataLen := runeSpan(buf, f.From, f.To)
			ed := &schema.ExtractedData{
				SourceType:     "httpflow",
				TraceId:        trace,
				Regexp:         ruleRegex,
				RuleVerbose:    ruleTagPrefix + f.RuleName,
				Data:           clipForDisplay(sanitizeInline(string(raw)), 512),
				DataIndex:      dataIndex,
				Length:         dataLen,
				IsMatchRequest: f.Direction == "request",
			}
			if err := yakit.CreateOrUpdateExtractedDataEx(-1, ed); err != nil {
				log.Errorf("trafficguard: save extracted data failed: %v", err)
			}
		}
		payloads = append(payloads, fmt.Sprintf("[%s] %s: %s", f.Direction, f.RuleName, hitDisp))
	}
	if len(payloads) > 0 {
		flow.Payload = clipForDisplay(strings.Join(payloads, " | "), 500)
	}
}

// buildMarkdownDescription 生成可一眼判真假的 markdown 描述: 列出每条命中的命中值与前后上下文。
func buildMarkdownDescription(findings []Finding, host string, cats []string, request, response []byte) string {
	var b strings.Builder
	b.WriteString("## 敏感信息泄漏线索(内置检测)\n\n")
	fmt.Fprintf(&b, "- 目标: `%s`\n", host)
	fmt.Fprintf(&b, "- 命中: %d 项(类别: %s)\n", len(findings), strings.Join(cats, ", "))
	b.WriteString("\n> 以下为每条命中的值与前后上下文, 请据此判断是真实凭证还是源码/埋点等误报。\n")
	for i, f := range findings {
		buf := response
		if f.Direction == "request" {
			buf = request
		}
		raw := extractRaw(f, buf)
		hitDisp := clipForDisplay(sanitizeInline(string(raw)), 200)
		ctx := contextSnippet(buf, f.From, f.To, 80)
		fmt.Fprintf(&b, "\n### %d. [%s] %s\n", i+1, severityVerbose(f.Severity), f.RuleName)
		fmt.Fprintf(&b, "- 类别: `%s` | 方向: `%s` | 长度: %d\n", f.Category, f.Direction, f.ValueLength)
		fmt.Fprintf(&b, "- 命中值:\n\n```\n%s\n```\n", hitDisp)
		if ctx != "" {
			b.WriteString("- 上下文(命中处以 「」 标注):\n\n```\n")
			b.WriteString(ctx)
			b.WriteString("\n```\n")
		}
		fmt.Fprintf(&b, "- 说明: %s\n", f.Description)
	}
	return strings.ToValidUTF8(b.String(), "")
}

// runeSpan 把字节偏移 [from,to) 换算为 rune(字符)下标与 rune 长度。
//
// 为什么必须换算: 前端高亮(以及 yaklang 既有 MITM 规则 HookColor)使用的是 rune 下标
// —— go-pcre2-lite/regexp2 的 Capture.Index / Capture.Length 报告的就是 rune 下标;
// 而 trafficguard 阶段二走 PCRE2 底层接口, 返回的是 byte 偏移。
// 报文(尤其是含中文注释/字符串的 JS、JSON)在命中点之前出现多字节字符时,
// byte 偏移 > rune 下标, 直接落库会让高亮整体右移, 即用户看到的"偏移/颜色错位"。
//
// 越界/非法偏移时做安全退化(钳到合法范围), 绝不 panic。
func runeSpan(buf []byte, from, to int) (index, length int) {
	if from < 0 {
		from = 0
	}
	if to > len(buf) {
		to = len(buf)
	}
	if to < from {
		to = from
	}
	index = utf8.RuneCount(buf[:from])
	length = utf8.RuneCount(buf[from:to])
	return index, length
}

// ruleTagsOf 收集命中的"具体规则名"作为 flow TAG(去重, 统一加 TrafficGuard: 前缀便于识别与归类)。
// 这样用户在 History 既能用总括标签筛全部命中, 也能按"具体命中了哪条规则"精确筛选。
func ruleTagsOf(findings []Finding) []string {
	seen := make(map[string]struct{}, len(findings))
	tags := make([]string, 0, len(findings))
	for _, f := range findings {
		name := strings.TrimSpace(f.RuleName)
		if name == "" {
			continue
		}
		tag := ruleTagPrefix + name
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	return tags
}

// extractRaw 返回命中的原始字节: 优先用 Finding.RawValue, 否则按偏移从缓冲区切片。
func extractRaw(f Finding, buf []byte) []byte {
	if len(f.RawValue) > 0 {
		return f.RawValue
	}
	if f.From >= 0 && f.To <= len(buf) && f.From < f.To {
		return buf[f.From:f.To]
	}
	return nil
}

// contextSnippet 取命中处前后 pad 字节的上下文, 命中片段用 「」 包裹, 便于人工判真假。
// 越界/非法偏移时返回空串。
func contextSnippet(buf []byte, from, to, pad int) string {
	if from < 0 || to > len(buf) || from >= to {
		return ""
	}
	start := from - pad
	if start < 0 {
		start = 0
	}
	end := to + pad
	if end > len(buf) {
		end = len(buf)
	}
	before := sanitizeInline(string(buf[start:from]))
	hit := clipForDisplay(sanitizeInline(string(buf[from:to])), 200)
	after := sanitizeInline(string(buf[to:end]))
	var b strings.Builder
	if start > 0 {
		b.WriteString("…")
	}
	b.WriteString(before)
	b.WriteString("「")
	b.WriteString(hit)
	b.WriteString("」")
	b.WriteString(after)
	if end < len(buf) {
		b.WriteString("…")
	}
	return strings.ToValidUTF8(b.String(), "")
}

// sanitizeInline 把控制字符/空白规整为单空格或 '.', 让命中值与上下文成为紧凑、可读的单行片段。
func sanitizeInline(s string) string {
	var b strings.Builder
	lastSpace := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
		case c < 0x20 || c == 0x7f:
			b.WriteByte('.')
			lastSpace = false
		default:
			b.WriteByte(c)
			lastSpace = false
		}
	}
	return b.String()
}

// clipForDisplay 对过长串做"首段 + 长度 + 尾段"截断, 避免描述/字段过长, 同时保留可判真假的首尾特征。
func clipForDisplay(s string, max int) string {
	if len(s) <= max {
		return strings.ToValidUTF8(s, "")
	}
	head := max * 2 / 3
	tail := max - head - 12
	if tail < 0 {
		tail = 0
	}
	if head > len(s) {
		head = len(s)
	}
	out := s[:head] + "…(len=" + itoa(len(s)) + ")…"
	if tail > 0 && tail < len(s) {
		out += s[len(s)-tail:]
	}
	return strings.ToValidUTF8(out, "")
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
