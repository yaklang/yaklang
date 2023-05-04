package yaktest

import (
	"fmt"
	"testing"
)

func TestRun_Report(t *testing.T) {
	cases := []YakTestCase{
		{
			Name: "测试 report.New()",
			Src: fmt.Sprintf(`r = report.New();
r.Title("生成一份报告的标题")
r.Owner("v1ll4n")
r.From("NAME")
r.Markdown("你好，我是一份报告！")
r.Table(
	["abasdfasdf", 123, 111, "asdfas"],
	["abas123123dfasdf", 123, 111, "asdfas"],
	["abasdfasdadaff", 123, 111, "asdfas"],
	["abasdfasdasdfasdfasdff", 123123123123, 111, "asdfas"],
	["dbbb", 123, ["adfasd", "aaa"], "asdfas"],
)
r.Save()
`),
		},
		{
			Name: "测试 report.New() 1",
			Src: fmt.Sprintf(`r = report.New();
r.Title("自定义了一个报告")
r.Owner("v1ll4n")
r.From("NAME")
r.Markdown("# 你好，我是一份报告！大标题\n\n你好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发\n\n> 引用数据\n\n1. 123123123123\n2. 34weqeasdfasd\n\n")
r.Markdown("# 你好，我是一份报告！大标题\n\n你好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发\n\n> 引用数据\n\n1. 123123123123\n2. 34weqeasdfasd\n\n")
r.Markdown("# 你好，我是一份报告！大标题\n\n你好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发\n\n> 引用数据\n\n1. 123123123123\n2. 34weqeasdfasd\n\n")
r.Markdown("# 你好，我是一份报告！大标题\n\n你好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发\n\n> 引用数据\n\n1. 123123123123\n2. 34weqeasdfasd\n\n")
r.Markdown("# 你好，我是一份报告！大标题\n\n你好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发\n\n> 引用数据\n\n1. 123123123123\n2. 34weqeasdfasd\n\n")
r.Markdown("# 你好，我是一份报告！大标题\n\n你好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发\n\n> 引用数据\n\n1. 123123123123\n2. 34weqeasdfasd\n\n")
r.Markdown("# 你好，我是一份报告！大标题\n\n你好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发\n\n> 引用数据\n\n1. 123123123123\n2. 34weqeasdfasd\n\n")
r.Markdown("# 你好，我是一份报告！大标题\n\n你好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发\n\n> 引用数据\n\n1. 123123123123\n2. 34weqeasdfasd\n\n")
r.Markdown("# 你好，我是一份报告！大标题\n\n你好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发\n\n> 引用数据\n\n1. 123123123123\n2. 34weqeasdfasd\n\n")
r.Markdown("# 你好，我是一份报告！大标题\n\n你好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发\n\n> 引用数据\n\n1. 123123123123\n2. 34weqeasdfasd\n\n")
r.Markdown("# 你好，我是一份报告！大标题\n\n你好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发\n\n> 引用数据\n\n1. 123123123123\n2. 34weqeasdfasd\n\n")
r.Markdown("# 你好，我是一份报告！大标题\n\n你好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发\n\n> 引用数据\n\n1. 123123123123\n2. 34weqeasdfasd\n\n")
r.Markdown("# 你好，我是一份报告！大标题\n\n你好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发\n\n> 引用数据\n\n1. 123123123123\n2. 34weqeasdfasd\n\n")
r.Markdown("# 你好，我是一份报告！大标题\n\n你好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发\n\n> 引用数据\n\n1. 123123123123\n2. 34weqeasdfasd\n\n")
r.Markdown("# 你好，我是一份报告！大标题\n\n你好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发好你好你好好阿斯顿发送到发\n\n> 引用数据\n\n1. 123123123123\n2. 34weqeasdfasd\n\n")
r.Table(
	["abasdfasdf", 123, 111, "asdfas"],
	["abas123123dfasdf", 123, 111, "asdfas"],
	["abasdfasdadaff", 123, 111, "asdfas"],
	["abasdfasdadaff", 123, 111, "asdfas"],
	["abasdfasdadaff", 123, 111, "asdfas"],
	["abasdfasdadaff", 123, 111, "asdfas"],
	["abasdfasdasdfasdfasdff", 123123123123, 111, "asdfas"],
	["dbbb", 123, ["adfasd", "aaa"], "asdfas"],
)
r.Save()
`),
		},
	}

	Run("x.ConvertToMap 可用性测试", t, cases...)
}
