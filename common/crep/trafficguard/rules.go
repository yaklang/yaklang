package trafficguard

// rules.go 定义 trafficguard 内置"超级正则组"。
//
// 设计目标: 用一套精心编写、低开销、可在 MITM 实时热路径运行的 RE2 正则组,
// 覆盖流量中"最高危"的凭证 / 密钥泄漏特征。每条规则都满足:
//
//  1. 高危: 命中即可判定为敏感凭证泄漏(强厂商前缀、固定格式私钥、带口令的连接串等);
//  2. 精准: 拥有稳定的必需字面量(供 minirehs Aho-Corasick 预过滤快速排除无命中输入),
//     并以字符集 + 定长约束把误报压到极低;
//  3. 低开销: 全部 RE2 兼容(无 backreference / lookaround / 无界 .* 跨行),可被 minirehs
//     MVS 后端编译为位并行 NFA 一次扫描全部规则,避免逐条正则的 O(N x L) 开销;
//  4. 危险度可衡量: 每条规则显式标注 Severity(critical/high/warning) 与类别; 但落库 Risk 的
//     严重度统一受 SeverityCeiling 上限约束(最高中危), 因为本组定位是"辅助人工判真假的线索"。
//
// 该规则组默认随 MITM 开启,当前不提供关闭开关(见 scanner.go 的 DefaultScanner)。
//
// 误报治理(见 validators.go validateFinding 第三阶段校验): PCRE2 精确提取后, 还会按 host/方向/值形态
// 做上下文校验, 剔除明显误报:
//   - 厂商自有域的第一方自用流量: Google/Chrome 自有域(content-autofill.googleapis.com 的
//     x-goog-api-key、搜索建议、gstatic、Firebase 遥测等)上的 key/token/通用 api-key 凭证字段与鉴权头
//     (规则 4/5/19/23/24/25)一律抑制 —— 浏览器自带流量并非泄漏;
//   - 非真 JWT 的 eyJ base64 块(首段无 alg);
//   - JS 源码里 password:function(...) 之类的源码型口令字段(值形态收紧);
//   - 登录页/表单的本地化文案与 UI 词(i18n): password:"设置密码" / pwd:"忘记密码" / "请输入密码" /
//     "Password" / "Login" 之类, 是给人看的标签而非真实口令(用户反馈"访问任意登录页就报 password"的根因),
//     常规上下文抑制; 但若写在注释里(被注释掉的默认/初始口令)则必须报出。
// JS 仍会被完整扫描(JS 硬编码与注释里可能藏真实凭证), 残留疑似项交由 Risk 上下文供人工判真假。

// Severity 取值对齐 yaklib risk 体系:
//
//	"critical" -> 严重 / "high" -> 高危 / "warning" -> 中危 / "low" -> 低危
const (
	severityCritical = "critical"
	severityHigh     = "high"
	severityMedium   = "warning" // 中危
)

// PluginName 是被 MITM 过滤掉、但命中内置敏感信息检测的流量的"来源插件名"。
// 这类流量不进 MITM History(source_type=mitm), 而是以"插件流量"(source_type=scan + FromPlugin)
// 的形式保存, 既不污染 MITM TAB, 又能在"插件输出"中留存证据。见 grpc_mitm.go 的集成点。
const PluginName = "内置敏感信息检测"

// SeverityCeiling 是 trafficguard 生成 Risk 的严重度上限: 一律不超过"中危"(warning)。
// 内置敏感信息检测以"提示线索、辅助人工判真假"为定位, 故即便命中私钥等本应严重的特征,
// 落库 Risk 也最高只给中危, 避免在被动扫描里制造高危告警噪声。
const SeverityCeiling = severityMedium

// rule 是超级正则组中的一条规则。
type rule struct {
	// ID 是稳定编号,写入 Risk 的 from_yak_script / tags,便于追溯。
	ID int
	// Name 中文规则名,直接作为 Risk 标题前缀。
	Name string
	// Category 凭证类别,如 vendor-token / private-key / connection-string。
	Category string
	// Severity 危险等级(critical/high/warning)。
	Severity string
	// Regex RE2 兼容正则; 必须含稳定字面量前缀以利于 minirehs 预过滤。
	Regex string
	// Description 风险描述,写入 Risk.description。
	Description string
	// Solution 修复建议,写入 Risk.solution。
	Solution string
	// RedactHead/RedactTail 脱敏时保留的首尾明文长度; 私钥等置 0。
	RedactHead int
	RedactTail int
}

// builtinRules 是内置超级正则组。顺序即 minirehs BuildGroup 的 pattern 下标,
// 命中后通过下标回指本表拿到规则的元信息。
//
// 编写约定:
//   - 能用厂商固定前缀(AKIA/AIza/ghp_/eyJ/sk_live_/-----BEGIN)就用前缀,保证预过滤命中;
//   - 字符集尽量收紧到该凭证真实使用的字母表(base64url / hex / base62);
//   - 长度用定长或窄区间,既压低误报,也让 NFA 匹配窗口有界;
//   - 中危(warning)仅放"带强关键词上下文的口令/凭证字段",不做无上下文的高熵扫描。
var builtinRules = []rule{
	// ---------------- 私钥(最高危,固定 PEM 边界) ----------------
	{
		ID:       1,
		Name:     "PEM 私钥泄漏",
		Category: "private-key",
		Severity: severityCritical,
		Regex:    `-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP |ENCRYPTED |)PRIVATE KEY-----`,
		Description: "流量中出现 PEM 格式私钥(RSA/EC/DSA/OpenSSH/PGP)。私钥一旦泄漏,攻击者可直接" +
			"解密通信、伪造身份、登录服务器,属于最高危凭证泄漏。",
		Solution:   "立即吊销并轮换该私钥,排查其进入流量的来源(如错误日志/调试接口/代码仓库),并对相关服务重新签发证书。",
		RedactHead: 0,
		RedactTail: 0,
	},

	// ---------------- 云厂商 AK / Token ----------------
	{
		ID:       2,
		Name:     "AWS 访问密钥 ID 泄漏(AKIA/ASIA)",
		Category: "cloud-credential",
		Severity: severityCritical,
		// AWS Access Key ID: 4 位固定前缀 + 16 位大写字母数字,严格定长。
		Regex: `(?:AKIA|ASIA|AIDA|AROA|AIPA|ANPA|AGPA|ACCA)[0-9A-Z]{16}`,
		Description: "AWS Access Key ID 以 AKIA/ASIA 等前缀 + 16 位固定字符构成。泄漏后攻击者可枚举该账号下的" +
			"资源与权限,配合 Secret Key 可完全接管云资产。",
		Solution:   "立即在 IAM 控制台禁用并删除该 Access Key,轮换新密钥,审计 CloudTrail 日志确认是否被滥用。",
		RedactHead: 4,
		RedactTail: 2,
	},
	{
		ID:       3,
		Name:     "AWS Secret Access Key 泄漏",
		Category: "cloud-credential",
		Severity: severityCritical,
		// 40 位 base64 标准字符,且必须出现在 aws_secret / secret_access_key 等强关键词上下文之后。
		Regex: `(?i)(?:aws_secret_access_key|aws_secret_key|secret_access_key)["'\s:=]{1,5}[A-Za-z0-9/+=]{40}\b`,
		Description: "AWS Secret Access Key(40 位)出现在强关键词上下文中。它与 Access Key ID 配对使用," +
			"泄漏即可直接调用 AWS API,属于高危凭证。",
		Solution:   "立即轮换该 Secret Access Key,排查其进入流量的路径(配置文件/环境变量/日志),收紧最小权限。",
		RedactHead: 3,
		RedactTail: 3,
	},
	{
		ID:       4,
		Name:     "Google API Key 泄漏",
		Category: "cloud-credential",
		Severity: severityHigh,
		// AIza + 35 位 base64url,严格定长 39。
		Regex:       `AIza[0-9A-Za-z_-]{35}`,
		Description: "Google API Key 以 AIza 前缀 + 35 位字符构成。泄漏后可被盗用消耗配额、访问受该 Key 授权的服务。",
		Solution:    "在 Google Cloud Console 轮换 Key,限制其 HTTP Referer / IP 来源,审计用量是否异常。",
		RedactHead:  4,
		RedactTail:  2,
	},
	{
		ID:          5,
		Name:        "Google OAuth Access Token 泄漏",
		Category:    "authorization-token",
		Severity:    severityHigh,
		Regex:       `ya29\.[0-9A-Za-z_-]{16,512}`,
		Description: "Google OAuth Access Token(ya29.*)用于代表用户访问 Google API。泄漏后在有效期内可冒充用户身份。",
		Solution:    "撤销该 Token(撤销授权或等待过期),排查 Token 泄漏来源,避免将其写入前端或日志。",
		RedactHead:  5,
		RedactTail:  3,
	},
	{
		ID:       6,
		Name:     "Azure 存储账户密钥泄漏",
		Category: "cloud-credential",
		Severity: severityCritical,
		// 出现在连接串中,AccountKey= 后跟 50+ 位 base64。
		Regex:       `(?i)AccountKey=[A-Za-z0-9+/=]{50,}`,
		Description: "Azure 存储账户连接串中的 AccountKey 泄漏。攻击者可用它完全读写对应 Blob/Queue/Table 存储内容。",
		Solution:    "在 Azure Portal 轮换存储账户密钥,改用 SAS + 最小权限或托管身份,排查连接串泄漏来源。",
		RedactHead:  0,
		RedactTail:  0,
	},

	// ---------------- SaaS 厂商 Token ----------------
	{
		ID:       7,
		Name:     "GitHub Token 泄漏",
		Category: "vendor-token",
		Severity: severityCritical,
		// ghp_/gho_/ghu_/ghs_/ghr_ + 36 位以上 base62。
		Regex: `gh[pousr]_[A-Za-z0-9]{36,255}`,
		Description: "GitHub Token(ghp_/gho_/ghu_/ghs_/ghr_)泄漏后可访问/修改私有仓库、触发 Actions、" +
			"读取组织机密,权限极高。",
		Solution:   "立即在 GitHub Settings 撤销该 Token,审计其访问的仓库与 Actions 日志,轮换新 Token。",
		RedactHead: 4,
		RedactTail: 4,
	},
	{
		ID:       8,
		Name:     "GitLab Token 泄漏",
		Category: "vendor-token",
		Severity: severityCritical,
		// glpat- + 20 位字符。
		Regex:       `glpat-[A-Za-z0-9_-]{20}`,
		Description: "GitLab Personal Access Token(glpat-*)泄漏后可读写对应 GitLab 项目与 CI/CD 资源。",
		Solution:    "在 GitLab User Settings 撤销该 Token,审计其使用记录,轮换新 Token。",
		RedactHead:  6,
		RedactTail:  4,
	},
	{
		ID:       9,
		Name:     "Slack Token 泄漏",
		Category: "vendor-token",
		Severity: severityHigh,
		// xox[baprs]- + 字符。
		Regex:       `xox[baprs]-[0-9A-Za-z-]{10,72}`,
		Description: "Slack Token(xox*)泄漏后可读取工作区消息、伪造发消息,泄露内部沟通内容。",
		Solution:    "在 Slack Admin 撤销该 Token,审计其调用记录,轮换新 Token。",
		RedactHead:  4,
		RedactTail:  4,
	},
	{
		ID:          10,
		Name:        "Slack Webhook 泄漏",
		Category:    "vendor-token",
		Severity:    severityMedium,
		Regex:       `https://hooks\.slack\.com/services/T[0-9A-Z]{6,}/B[0-9A-Z]{6,}/[0-9A-Za-z]{16,}`,
		Description: "Slack Incoming Webhook 地址泄漏后,任何人可向该频道投递消息/钓鱼。",
		Solution:    "在 Slack App 设置中禁用并重建该 Webhook,仅服务端持有。",
		RedactHead:  0,
		RedactTail:  0,
	},
	{
		ID:       11,
		Name:     "Stripe 生产环境密钥泄漏",
		Category: "vendor-token",
		Severity: severityCritical,
		// sk_live_ / rk_live_ + 24 位以上。
		Regex: `(?:sk|rk)_live_[0-9a-zA-Z]{24,}`,
		Description: "Stripe 生产环境 Secret/Restricted Key(sk_live_/rk_live_)泄漏后可发起支付、退款、" +
			"读取卡片信息,直接造成资金风险。",
		Solution:   "立即在 Stripe Dashboard 滚动轮换该密钥,审计支付记录,改用受限 Key + 服务端保管。",
		RedactHead: 8,
		RedactTail: 4,
	},
	{
		ID:       12,
		Name:     "OpenAI API Key 泄漏",
		Category: "vendor-token",
		Severity: severityCritical,
		// 新版 sk-proj- 或 48 位定长 sk-。
		Regex:       `sk-proj-[A-Za-z0-9_-]{40,}|sk-[A-Za-z0-9]{48}`,
		Description: "OpenAI API Key 泄漏后会被盗刷调用额度,产生高额账单。",
		Solution:    "在 OpenAI Platform 撤销该 Key,设置用量上限,审计账单,轮换新 Key。",
		RedactHead:  4,
		RedactTail:  4,
	},
	{
		ID:       13,
		Name:     "SendGrid API Key 泄漏",
		Category: "vendor-token",
		Severity: severityHigh,
		// SG. + 22 + . + 43。
		Regex:       `SG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}`,
		Description: "SendGrid API Key(SG.*.*)泄漏后可盗用邮件发送额度、冒充发件人钓鱼。",
		Solution:    "在 SendGrid 后台撤销该 Key,审计邮件发送记录,轮换新 Key。",
		RedactHead:  3,
		RedactTail:  4,
	},
	{
		ID:       14,
		Name:     "Twilio API Key 泄漏",
		Category: "vendor-token",
		Severity: severityHigh,
		// SK + 32 位 hex。
		Regex:       `SK[0-9a-fA-F]{32}`,
		Description: "Twilio API Key(SK + 32 位十六进制)泄漏后可盗发短信/语音,造成资损与钓鱼风险。",
		Solution:    "在 Twilio Console 暂停并轮换该 API Key,审计通话/短信记录,收紧权限。",
		RedactHead:  2,
		RedactTail:  4,
	},
	{
		ID:          15,
		Name:        "Mailgun API Key 泄漏",
		Category:    "vendor-token",
		Severity:    severityHigh,
		Regex:       `key-[0-9a-zA-Z]{32}`,
		Description: "Mailgun API Key(key-*)泄漏后可盗用邮件发送额度。",
		Solution:    "在 Mailgun 控制台轮换该 API Key,审计发送记录。",
		RedactHead:  4,
		RedactTail:  4,
	},
	{
		ID:          16,
		Name:        "Square 访问令牌泄漏",
		Category:    "vendor-token",
		Severity:    severityCritical,
		Regex:       `sq0atp-[0-9A-Za-z_-]{22}|sq0csp-[0-9A-Za-z_-]{43}`,
		Description: "Square Access Token / Secret(sq0atp-/sq0csp-)泄漏后可操作商户支付与资金。",
		Solution:    "在 Square Developer Dashboard 撤销并轮换该凭证,审计交易记录。",
		RedactHead:  7,
		RedactTail:  4,
	},
	{
		ID:          17,
		Name:        "AWS MWS Auth Token 泄漏",
		Category:    "vendor-token",
		Severity:    severityHigh,
		Regex:       `amzn\.mws\.[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`,
		Description: "亚马逊 MWS Auth Token 泄漏后可访问卖家店铺与订单数据。",
		Solution:    "在亚马逊卖家后台撤销并轮换该 Token,审计接口调用。",
		RedactHead:  9,
		RedactTail:  6,
	},
	{
		ID:       18,
		Name:     "Discord Bot Token 泄漏",
		Category: "vendor-token",
		Severity: severityHigh,
		// 机器人令牌: 头部 [MN] + 23 位 . 6 位 . 27 位。
		Regex:       `[MN][A-Za-z0-9_-]{23}\.[A-Za-z0-9_-]{6}\.[A-Za-z0-9_-]{27}`,
		Description: "Discord Bot Token 泄漏后攻击者可完全控制该机器人,读取频道、伪造消息。",
		Solution:    "在 Discord Developer Portal 重置该 Bot Token,审计其行为日志。",
		RedactHead:  4,
		RedactTail:  4,
	},

	// ---------------- 通用认证凭证 ----------------
	{
		ID:       19,
		Name:     "JWT 泄漏",
		Category: "authorization-token",
		Severity: severityHigh,
		// 三段 base64url 用 . 分隔,首段以 eyJ 开头(JSON "{" 的 base64url)。
		// 仅在响应方向 + 首段为含 alg 的真实 JWT header 时才算命中(见 scanner.go validateFinding):
		// 请求方向的 JWT 多为第一方会话凭证(等同 Authorization 头), 抑制以降噪;
		// 响应/JS 源码中出现的 JWT 更可能是硬编码或泄漏, 予以保留。
		Regex:       `eyJ[A-Za-z0-9_-]{8,512}\.eyJ[A-Za-z0-9_-]{8,512}\.[A-Za-z0-9_-]{8,512}`,
		Description: "响应/脚本中出现 JWT(JSON Web Token)。若为硬编码或被接口返回, 泄漏后在有效期内可冒充该身份访问服务。",
		Solution:    "缩短 JWT 有效期、服务端吊销会话、避免将 JWT 写入 URL、前端持久存储或脚本源码,排查泄漏来源。",
		RedactHead:  3,
		RedactTail:  3,
	},
	// 注: Authorization: Bearer / Basic 头(原规则 20/21)已移除。
	// 原因: 在正常带鉴权浏览中, 几乎每个请求都携带 Authorization 头, 命中率极高且全是
	// 用户自己对目标站点的"第一方会话凭证", 并非泄漏, 只会产生大量噪声。X-API-Key 等
	// 自定义鉴权头(规则 25)相对更有意义, 予以保留。JWT(规则 19)仅在响应方向保留(见 scanner.go
	// 的 validateFinding: 请求方向的 JWT 同样视为第一方会话凭证而抑制)。

	// ---------------- 数据库 / 中间件连接串 ----------------
	{
		ID:       22,
		Name:     "数据库/中间件连接串(含口令)泄漏",
		Category: "connection-string",
		Severity: severityCritical,
		// scheme://user:pass@host,口令明文出现在 URL 中。
		Regex:       `(?i)(?:mongodb(?:\+srv)?|postgresql?|mysql|redis|amqps?|mssql|db2|nacos)://[^\s:/@]{1,256}:[^\s@/]{1,256}@[^\s/]{1,256}`,
		Description: "流量中出现带账号口令的数据库/中间件连接串(如 mysql://user:pass@host)。泄漏即可直接连接数据库。",
		Solution:    "立即修改该数据库账号口令,限制来源 IP,改用环境变量/密钥管理服务注入连接串,排查泄漏来源。",
		RedactHead:  0,
		RedactTail:  0,
	},

	// ---------------- 中危: 带强关键词上下文的口令/凭证字段 ----------------
	{
		ID:       23,
		Name:     "敏感口令/凭证字段泄漏",
		Category: "password",
		Severity: severityMedium,
		// JSON/Form 中形如 "password":"xxx" / secret=xxx 的键值,值长度 >=4。
		Regex: `(?i)["']?(?:password|passwd|pwd|passphrase|secret|api[_-]?key|apikey|access[_-]?token|client[_-]?secret|auth[_-]?token)["']?\s*[:=]\s*["']?[^\s"'` + "`" + `&<>]{4,256}`,
		Description: "流量中出现口令/凭证类敏感字段(如 password/secret/api_key/access_token)及其值。" +
			"明文传输口令属于中危信息泄漏。",
		Solution:   "避免明文传输口令,敏感字段改用哈希或加密,排查该接口是否应返回/接收明文凭据。",
		RedactHead: 0,
		RedactTail: 0,
	},
	{
		ID:       24,
		Name:     "URL/表单中的 API Key 参数泄漏",
		Category: "vendor-token",
		Severity: severityMedium,
		// ?api_key=xxx / access_token=xxx 等,值长度 >=16。
		Regex:       `(?i)(?:api[_-]?key|apikey|access[_-]?token|client[_-]?secret|secret[_-]?key)=[A-Za-z0-9._~+/=%_-]{16,256}`,
		Description: "URL 查询串或表单中出现 api_key/access_token/client_secret 等敏感参数及其值。这类凭证常被日志/Referer 记录。",
		Solution:    "将凭证移到请求头或 POST body,避免出现在 URL 中,排查是否已被网关/日志记录。",
		RedactHead:  0,
		RedactTail:  0,
	},
	{
		ID:       25,
		Name:     "自定义鉴权请求头凭证泄漏",
		Category: "authorization-token",
		Severity: severityMedium,
		// X-API-Key / X-Auth-Token / X-Secret 等头部携带长凭证。
		Regex:       `(?i)(?:x[_-]?api[_-]?key|x[_-]?auth[_-]?token|x[_-]?secret|api[_-]?key)\s*:[^\n]{0,3}[A-Za-z0-9+/=_\-.]{16,256}`,
		Description: "请求中出现 X-API-Key/X-Auth-Token/X-Secret 等自定义鉴权头及其凭证。泄漏可被重放冒用。",
		Solution:    "确认该头部凭证来源与权限,必要时轮换,避免明文出现在前端或可被缓存的位置。",
		RedactHead:  0,
		RedactTail:  0,
	},
}

// builtinRuleByID 建立编号 -> 规则的索引,供 Risk 写入稳定来源信息。
var builtinRuleByID = func() map[int]*rule {
	m := make(map[int]*rule, len(builtinRules))
	for i := range builtinRules {
		m[builtinRules[i].ID] = &builtinRules[i]
	}
	return m
}()
