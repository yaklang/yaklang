package fingerprint

import (
	"context"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp/fingerprint/parsers"
	"github.com/yaklang/yaklang/common/schema"
)

// GetAllFingerprint 从本地指纹规则库中读取全部指纹规则，并以 channel 形式逐条返回
// 该函数依赖本地规则数据库，规则数量取决于已加载的指纹库
// 返回值:
//   - 一个只读 channel，逐条产出 *schema.GeneralRule 指纹规则
//
// Example:
// ```
// // 该示例为示意性用法：遍历本地指纹规则库
// count = 0
//
//	for rule = range fp.GetAllFingerprint() {
//	    count++
//	    if count >= 5 {
//	        break
//	    }
//	}
//
// println("read rules:", count)
// ```
func GetAllFingerprint() chan *schema.GeneralRule {
	db := consts.GetGormProfileDatabase()
	var allFingerprint []*schema.GeneralRule
	db.Model(&schema.GeneralRule{}).Find(&allFingerprint)
	ch := make(chan *schema.GeneralRule, len(allFingerprint))
	for _, fp := range allFingerprint {
		ch <- fp
	}
	close(ch)
	return ch
}

// MatchRspByRule 使用单条指纹规则匹配给定的响应报文，命中返回 true
// 参数:
//   - rsp: 待匹配的原始响应报文(字节数组)
//   - rule: 指纹规则，可以是 *schema.GeneralRule 对象，或形如 `body="xxx"` 的匹配表达式字符串
//
// 返回值:
//   - 是否命中该指纹规则
//
// Example:
// ```
// rsp = "HTTP/1.1 200 OK\r\nServer: nginx\r\n\r\n<html>welcome to nginx</html>"
// // 用 body 匹配表达式判断响应体是否包含指定内容
// hit = fp.MatchRspByRule([]byte(rsp), `body="welcome to nginx"`)
// println(hit)   // OUT: true
// assert hit == true, "rule should match the response body"
// miss = fp.MatchRspByRule([]byte(rsp), `body="this-should-not-appear-xyz"`)
// assert miss == false, "rule should not match absent content"
// ```
func MatchRspByRule(rsp []byte, rule any) bool {
	switch rule := rule.(type) {
	case *schema.GeneralRule:
		rules, _ := parsers.ParseExpRule(rule)
		matcher := NewMatcher()
		info := matcher.Match(context.Background(), rsp, rules)
		return len(info) > 0
	case string:
		fp := &schema.GeneralRule{
			MatchExpression: rule,
			CPE:             &schema.CPE{Product: uuid.New().String()},
		}
		return MatchRspByRule(rsp, fp)
	}
	return false
}

// MatchRsp 使用本地全部指纹规则库匹配给定的响应报文，返回所有命中的产品名称(CPE Product)
// 该函数依赖本地规则数据库
// 参数:
//   - rsp: 待匹配的原始响应报文(字节数组)
//
// 返回值:
//   - 命中的产品名称字符串切片，未命中时为空切片
//
// Example:
// ```
// // 该示例为示意性用法：用本地指纹库匹配响应报文
// rsp = "HTTP/1.1 200 OK\r\nServer: nginx\r\n\r\n<html>welcome</html>"
// products = fp.MatchRsp([]byte(rsp))
// println("matched products:", len(products))
// ```
func MatchRsp(rsp []byte) []string {
	db := consts.GetGormProfileDatabase()
	var rules []*schema.GeneralRule
	db.Model(&schema.GeneralRule{}).Find(&rules)
	matched := []string{}
	for _, rule := range rules {
		if MatchRspByRule(rsp, rule) {
			matched = append(matched, rule.CPE.Product)
		}
	}
	return matched
}

var Exports = map[string]any{
	"MatchRspByRule":    MatchRspByRule,
	"MatchRsp":          MatchRsp,
	"GetAllFingerprint": GetAllFingerprint,
}
