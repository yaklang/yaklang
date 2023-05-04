package yaktest

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"testing"
)

func TestMisc_Mutate1(t *testing.T) {
	res, err := mutate.QuickMutate(
		"test_{{yak(handle)}} {{yak(handle1)}}", nil,
		yak.MutateWithYaklang(`
handle1 = func(a) {
    result = make([]string)
	result = append(result, "123123")
	result = append(result, "123123111")
	result = append(result, "12312sadfasdfasd3")
	return result
}

handle = func(params){
	return "test"
}`))
	if err != nil {
		panic(err)
	}
	spew.Dump(res)
}

func TestMisc_Mutate(t *testing.T) {
	randomPort := utils.GetRandomAvailableTCPPort()
	utils.NewWebHookServer(randomPort, func(data interface{}) {

	})

	cases := []YakTestCase{
		{
			Name: "测试 mutate.Fuzz",
			Src:  "res = (fuzz.HTTPRequest(`GET / HTTP/1.1\nHost: www.baidu.com\n\n`)[0]).ExecFirst()[0]; desc(res)",
		},
		{
			Name: "测试 mutate.Strings",
			Src: `
assert(len( fuzz.Strings("{{int(1-3)}}")) == 3);
assert(len( fuzz.Strings(["{{int(1-3)}}", "{{int(1-10)}}", "111"])) == 14);
assert(len( fuzz.Strings(["{{int(1-3)}}", "{{int(1-10)}}"])) == 13);
`,
		},
		{
			Name: "测试 mutate.StringsFunc",
			Src: `
assert(fuzz.StringsFunc("{{int(1)}}", func(i){
	desc(i)
}) == nil)
`,
		},
		{
			Name: "测试 mutate.StringsFunc2",
			Src: `
assert(fuzz.StringsFunc("{{params(tar)}}", func(i){
	dump(i.Result)
	println(i.Result)
	assert(x.If(i.Result == "asaaaaaa", true, false))
}, {"tar": "asaaaaaa"}) == nil)
`,
		},
	}

	Run("fuzz 可用性测试(外部网络): ExecFirst", t, cases...)
}
