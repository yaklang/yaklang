package aibalance

// custom_429.go 集中定义各类限流 429 的「默认文案」与元信息，作为前后端唯一真相源：
//   - 后端各 429 写出点（gates.go / server.go / memfit_version_gate.go）统一引用这里的常量，
//     保证运行时实际返回的默认文案与「自定义 429 文案」编辑界面展示的默认文案完全一致；
//   - 前端通过 GET /portal/api/rate-limit-config 返回的 custom_429_kind_defaults 拿到每个
//     limit_kind 的默认文案/中文名/触发原因，从而在编辑时直观看到默认文案是什么。
//
// 设计要点：
//   - 文案均「清晰写明 429 原因」，中英双语，便于用户与运维快速理解；
//   - 动态类型（rpm / daily_token）运行时会在默认文案后追加具体数值（排队位置 / 用量），
//     编辑界面展示的是不含动态数值的基础文案；管理员设置覆盖文案后将整体替换 message。
//
// 关键词: custom_429 默认文案集中定义, Custom429Kinds, limit_kind 默认文案, 429 原因清晰化

const (
	// Default429MessageRPM 请求频率限流（RPM 滑动窗口）。运行时会追加当前排队位置。
	// 关键词: 429 rpm 默认文案, 请求频率限流
	Default429MessageRPM = "请求过于频繁，已触发请求频率限流（RPM 滑动窗口）。请稍候约 10 秒后重试。" +
		"Rate limit exceeded (too many requests, RPM sliding window); please retry after about 10 seconds."

	// Default429MessageToken 单个 API Key 的 Token 计费额度耗尽（计费体系唯一的 Key 级限额）。
	// 关键词: 429 token 默认文案, API Key Token 额度
	Default429MessageToken = "该 API Key 的 Token 计费额度已用尽（已达单 Key 上限）。请联系管理员提升额度或重置用量。" +
		"This API key has exhausted its token quota (per-key limit reached). Please contact the administrator to raise the limit or reset usage."

	// Default429MessageDailyToken 免费用户当日 Token 额度耗尽。运行时会追加已用/上限等具体数值。
	// 关键词: 429 daily_token 默认文案, 免费用户日额度
	Default429MessageDailyToken = "今日免费 Token 额度已用尽，每日北京时间 06:00 自动重置。" +
		"Daily token quota exceeded; the free daily allowance resets at 06:00 (Asia/Shanghai)."

	// Default429MessageFreeIP 单个客户端 IP 当日免费模型用量（请求数或 Token）耗尽。
	// 关键词: 429 free_ip 默认文案, 单 IP 免费额度
	Default429MessageFreeIP = "当前环境免费用量已用尽（按客户端 IP 维度限额）。请自行配置 AI 后端使用，或次日北京时间 06:00 后重试。" +
		"Free quota for this IP/environment has been used up; please configure your own AI backend, or retry after 06:00 (Asia/Shanghai)."

	// Default429MessagePaidDailyToken 全平台付费 Token 当日总额度耗尽（第二道硬门）。
	// 关键词: 429 paid_daily_token 默认文案, 付费用户日总额度
	Default429MessagePaidDailyToken = "平台付费 Token 日总额度已用尽（聚合全部付费用量的第二道硬门，每日北京时间 06:00 重置）。请稍后重试或联系管理员。" +
		"Platform-wide paid daily token quota exhausted (aggregated hard gate, resets at 06:00 Asia/Shanghai). Please retry later or contact the administrator."

	// Default429MessageMemfitVersion 旧版本/未知版本 Memfit/Yak 客户端用量达到上限。
	// 关键词: 429 memfit_version 默认文案, 客户端版本控流
	Default429MessageMemfitVersion = "针对旧版本/未知版本 Memfit/Yak 客户端的免费使用量已达到最大上限（最大上限为 1 亿 Token）。请更新到最新版本 Yak 引擎或 Memfit/Yak Project 系统后继续使用。" +
		"Usage limit for legacy/unknown Memfit/Yak clients reached (max 100M tokens). Please upgrade to the latest Yak engine or Memfit/Yak Project."
)

// Custom429KindMeta 描述一个可自定义 429 文案的限流类型，用于前端编辑界面展示默认文案与触发原因。
// 关键词: Custom429KindMeta, limit_kind 默认文案元信息
type Custom429KindMeta struct {
	Kind        string `json:"kind"`            // limit_kind 取值，与各写出点约定一致
	Type        string `json:"type"`            // 对外 error.type
	LabelZh     string `json:"label_zh"`        // 中文名（编辑界面标题）
	Default     string `json:"default_message"` // 默认文案（动态类型为不含运行时数值的基础文案）
	Dynamic     bool   `json:"dynamic"`         // 是否含运行时动态数值（如排队位置 / 已用量）
	Description string `json:"description"`     // 触发原因说明
}

// Custom429Kinds 返回所有可自定义 429 文案的限流类型（保持稳定展示顺序）。
// 注意：旧的字节流量限额（traffic）已彻底停用，不再列出；新增 free_ip 与 paid_daily_token。
// 关键词: Custom429Kinds, custom_429_kind_defaults, 去 traffic 增 paid_daily_token/free_ip
func Custom429Kinds() []Custom429KindMeta {
	return []Custom429KindMeta{
		{
			Kind:        "rpm",
			Type:        "rate_limit_exceeded",
			LabelZh:     "请求频率限流",
			Default:     Default429MessageRPM,
			Dynamic:     true,
			Description: "单位时间内请求过多，触发按 API Key 的 RPM 滑动窗口限流（实际返回会附带排队位置）。",
		},
		{
			Kind:        "token",
			Type:        "token_limit_exceeded",
			LabelZh:     "API Key Token 额度",
			Default:     Default429MessageToken,
			Dynamic:     false,
			Description: "单个 API Key 的 Token 计费用量达到设定上限（计费体系唯一的 Key 级硬门）。",
		},
		{
			Kind:        "daily_token",
			Type:        "daily_token_limit_exceeded",
			LabelZh:     "免费用户日额度",
			Default:     Default429MessageDailyToken,
			Dynamic:     true,
			Description: "免费用户当日 Token 额度（全局或模型级）已用尽，每日北京时间 06:00 重置（实际返回会附带已用/上限）。",
		},
		{
			Kind:        "free_ip",
			Type:        "free_ip_limit_exceeded",
			LabelZh:     "单 IP 免费额度",
			Default:     Default429MessageFreeIP,
			Dynamic:     false,
			Description: "单个客户端 IP 当日免费模型用量（请求次数或加权 Token）已用尽。",
		},
		{
			Kind:        "paid_daily_token",
			Type:        "paid_daily_token_limit_exceeded",
			LabelZh:     "付费用户日总额度",
			Default:     Default429MessagePaidDailyToken,
			Dynamic:     false,
			Description: "全平台付费 Token 当日总额度已用尽（与免费日额度并列的第二道硬门），每日北京时间 06:00 重置。",
		},
		{
			Kind:        "memfit_version",
			Type:        "memfit_client_version_limited",
			LabelZh:     "客户端版本控流",
			Default:     Default429MessageMemfitVersion,
			Dynamic:     false,
			Description: "旧版本/未知版本的 Memfit/Yak 客户端免费使用量达到上限，引导升级到最新版本。",
		},
	}
}
