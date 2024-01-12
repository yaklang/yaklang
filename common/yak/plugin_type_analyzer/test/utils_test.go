package test

import (
	"sort"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/plugin_type_analyzer"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func check(t *testing.T, code string, want []string, typ ...string) *ssaapi.Program {
	pluginType := "yak"
	if len(typ) != 0 {
		pluginType = typ[0]
	}
	test := assert.New(t)

	prog, err := ssaapi.Parse(code, plugin_type_analyzer.GetPluginSSAOpt(pluginType)...)
	test.Nil(err)
	prog.Show()
	gotErr := yak.AnalyzeStaticYaklangWithType(code, pluginType)
	got := lo.Map(gotErr, func(res *yak.StaticAnalyzeResult, _ int) string {
		return res.Message
	})

	sort.Strings(want)
	log.Info("want: ", want)
	sort.Strings(got)
	log.Info("got: ", got)

	test.Equal(want, got)

	return prog
}
