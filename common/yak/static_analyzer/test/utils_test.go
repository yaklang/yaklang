package test

import (
	"sort"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
)

func check(t *testing.T, code string, want []string, typ ...string) *ssaapi.Program {
	pluginType := "yak"
	if len(typ) != 0 {
		pluginType = typ[0]
	}
	test := assert.New(t)

	prog, err := ssaapi.Parse(code, static_analyzer.GetPluginSSAOpt(pluginType)...)
	test.Nil(err)
	prog.Show()
	gotErr := yak.StaticAnalyzeYaklang(code, pluginType)
	got := lo.Map(gotErr, func(res *result.StaticAnalyzeResult, _ int) string {
		return res.Message
	})

	sort.Strings(want)
	log.Info("want: ", want)
	sort.Strings(got)
	log.Info("got: ", got)

	test.Equal(want, got)

	return prog
}
