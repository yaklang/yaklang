package chaosmaker

import (
	"context"

	"github.com/gopacket/gopacket"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/match"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// YieldRules 从本地规则库中读取全部规则，以 channel 形式逐条返回
// 在 yak 中通过 suricata.YieldRules 调用，依赖本地规则数据库
// 返回值:
//   - 一个只读 channel，逐条产出规则存储对象
//
// Example:
// ```
// // 该示例为示意性用法：遍历本地规则库
//
//	for rule = range suricata.YieldRules() {
//	    println(rule.Name)
//	}
//
// ```
func yieldRules() chan *rule.Storage {
	return rule.YieldRules(consts.GetGormProfileDatabase().Model(&rule.Storage{}), context.Background())
}

// YieldRulesByKeyword 按关键词(以及可选协议)从本地规则库中检索匹配的规则，以 channel 形式返回
// 在 yak 中通过 suricata.YieldRulesByKeyword 调用，依赖本地规则数据库
// 参数:
//   - keywords: 检索关键词，多个关键词可用逗号分隔
//   - protos: 可选的协议过滤，如 "tcp"、"http"
//
// 返回值:
//   - 一个只读 channel，逐条产出匹配的规则存储对象
//
// Example:
// ```
// // 该示例为示意性用法：按关键词检索规则
//
//	for rule = range suricata.YieldRulesByKeyword("redis", "tcp") {
//	    println(rule.Name)
//	}
//
// ```
func YieldRulesByKeywords(keywords string, protos ...string) chan *rule.Storage {
	return YieldRulesByKeywordsWithType("", keywords, protos...)
}

// YieldSuricataRulesByKeywords 按关键词检索类型为 suricata 的规则，以 channel 形式返回
// 在 yak 中通过 suricata.YieldSuricataRulesByKeywords 调用，依赖本地规则数据库
// 参数:
//   - keywords: 检索关键词，多个关键词可用逗号分隔
//   - protos: 可选的协议过滤，如 "tcp"、"http"
//
// 返回值:
//   - 一个只读 channel，逐条产出匹配的 suricata 规则存储对象
//
// Example:
// ```
// // 该示例为示意性用法：检索 suricata 规则
//
//	for rule = range suricata.YieldSuricataRulesByKeywords("trojan") {
//	    println(rule.Name)
//	}
//
// ```
func YieldSuricataRulesByKeywords(keywords string, protos ...string) chan *rule.Storage {
	return YieldRulesByKeywordsWithType("suricata", keywords, protos...)
}

func YieldRulesByKeywordsWithType(ruleType string, keywords string, protos ...string) chan *rule.Storage {
	db := consts.GetGormProfileDatabase().Model(&rule.Storage{})
	protos = utils.RemoveRepeatedWithStringSlice(protos)
	if len(protos) > 0 {
		db = bizhelper.ExactQueryStringArrayOr(db, "protocol", protos)
	}
	if ruleType != "" {
		db = db.Where("rule_type = ?", ruleType)
	}
	db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
		"name", "keywords",
	}, utils.PrettifyListFromStringSplitEx(keywords, ",", "|"), false)
	return rule.YieldRules(db, context.Background())
}

// LoadSuricataToDatabase 解析 Suricata 规则文本并将其保存到本地规则数据库
// 在 yak 中通过 suricata.LoadSuricataToDatabase 调用
// 参数:
//   - raw: Suricata 规则文本，可包含多条规则
//
// 返回值:
//   - 错误信息，解析失败时非 nil(单条保存失败仅记录告警)
//
// Example:
// ```
// // 该示例为示意性用法：导入规则到本地库
// err = suricata.LoadSuricataToDatabase(`alert tcp any any -> any any (msg:"x"; content:"a"; sid:1;)`)
// ```
func LoadSuricataToDatabase(raw string) error {
	rules, err := surirule.Parse(raw)
	if err != nil {
		return err
	}
	for _, r := range rules {
		err := rule.SaveSuricata(consts.GetGormProfileDatabase(), r)
		if err != nil {
			log.Warnf("save suricata error: %s", err)
		}
	}
	return nil
}

// DeleteSuricataRuleByID 根据规则 ID 从本地规则数据库中删除一条 Suricata 规则
// 在 yak 中通过 suricata.DeleteSuricataRuleByID 调用
// 参数:
//   - id: 规则在数据库中的 ID
//
// 返回值:
//   - 错误信息，删除失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：按 ID 删除规则
// err = suricata.DeleteSuricataRuleByID(1)
// ```
func DeleteSuricataRuleByID(id int64) error {
	return rule.DeleteSuricataRuleByID(consts.GetGormProfileDatabase(), id)
}

var (
	ChaosMakerExports = map[string]any{
		"NewSuricataMatcherGroup": NewSuricataMatcherGroup,
		"groupCallback":           GroupOnMatchedCallback,

		"NewSuricataMatcher":           NewSuricataMatcher,
		"ParseSuricata":                ParseSuricata,
		"YieldRules":                   yieldRules,
		"YieldRulesByKeyword":          YieldRulesByKeywords,
		"YieldSuricataRulesByKeywords": YieldSuricataRulesByKeywords,
		"LoadSuricataToDatabase":       LoadSuricataToDatabase,
		"DeleteSuricataRuleByID":       DeleteSuricataRuleByID,
		"TrafficGenerator":             NewChaosMaker,
	}
)

// ParseSuricata 解析 Suricata 规则文本，返回结构化的规则对象列表
// 在 yak 中通过 suricata.ParseSuricata 调用
// 参数:
//   - raw: Suricata 规则文本，可包含一条或多条规则
//
// 返回值:
//   - 解析得到的规则对象列表
//   - 错误信息，解析失败时非 nil
//
// Example:
// ```
// rule = `alert tcp any any -> any any (msg:"test rule"; content:"hello"; sid:1000001;)`
// rules, err = suricata.ParseSuricata(rule)
// assert err == nil, "should parse suricata rule"
// println(len(rules))   // OUT: 1
// assert rules[0].Message == "test rule", "rule message should be parsed"
// ```
func ParseSuricata(raw string) ([]*surirule.Rule, error) {
	return surirule.Parse(raw)
}

// NewSuricataMatcher 基于单条 Suricata 规则创建一个流量匹配器，用于判断数据包是否命中该规则
// 在 yak 中通过 suricata.NewSuricataMatcher 调用
// 参数:
//   - r: 单条 Suricata 规则对象(通常来自 suricata.ParseSuricata)
//
// 返回值:
//   - 流量匹配器对象
//
// Example:
// ```
// // 该示例为示意性用法：用单条规则创建匹配器
// rules = suricata.ParseSuricata(`alert tcp any any -> any any (msg:"x"; content:"a"; sid:1;)`)~
// matcher = suricata.NewSuricataMatcher(rules[0])
// println(matcher != nil)
// ```
func NewSuricataMatcher(r *surirule.Rule) *match.Matcher {
	return match.New(r)
}

// NewSuricataMatcherGroup 创建一个 Suricata 规则匹配器组，可批量加载规则并对数据包进行匹配
// 在 yak 中通过 suricata.NewSuricataMatcherGroup 调用
// 参数:
//   - opt: 可选配置项，如 suricata.groupCallback 设置命中回调
//
// 返回值:
//   - 规则匹配器组对象
//
// Example:
// ```
// // 该示例为示意性用法：创建匹配器组并设置命中回调
//
//	group = suricata.NewSuricataMatcherGroup(suricata.groupCallback(func(packet, matchedRule) {
//	    println("matched:", matchedRule.Message)
//	}))
//
// println(group != nil)
// ```
func NewSuricataMatcherGroup(opt ...match.GroupOption) *match.Group {
	return match.NewGroup(opt...)
}

// groupCallback 设置规则匹配器组在命中规则时触发的回调函数
// 在 yak 中通过 suricata.groupCallback 调用
// 参数:
//   - cb: 命中回调，接收命中的数据包与对应规则
//
// 返回值:
//   - 一个 suricata.NewSuricataMatcherGroup 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置命中回调
//
//	group = suricata.NewSuricataMatcherGroup(suricata.groupCallback(func(packet, matchedRule) {
//	    println("matched:", matchedRule.Message)
//	}))
//
// ```
func GroupOnMatchedCallback(cb func(packet gopacket.Packet, match *surirule.Rule)) match.GroupOption {
	return match.WithGroupOnMatchedCallback(cb)
}

func init() {
	yakit.RegisterPostInitDatabaseFunction(func() error {
		match.RegisterSuricataRuleLoader(func(query string) (chan *surirule.Rule, error) {
			var rc = make(chan *surirule.Rule)
			go func() {
				defer close(rc)
				for result := range YieldSuricataRulesByKeywords(query) {
					if result.RuleType == "suricata" {
						srule, err := surirule.Parse(result.SuricataRaw)
						if err != nil {
							continue
						}
						for _, r := range srule {
							rc <- r
						}
					}
				}
			}()
			return rc, nil
		})
		return nil
	}, "register-suricata-rule-loader")
}
