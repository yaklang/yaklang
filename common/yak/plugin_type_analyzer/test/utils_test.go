package test

import (
	"sort"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak"
)

func check(t *testing.T, code string, want []string, typ ...string) {
	pluginType := "yak"
	if len(typ) != 0 {
		pluginType = typ[0]
	}
	gotErr := yak.AnalyzeStaticYaklangWithType(code, pluginType)
	got := lo.Map(gotErr, func(res *yak.StaticAnalyzeResult, _ int) string {
		return res.Message
	})

	test := assert.New(t)
	sort.Strings(want)
	log.Info("want: ", want)
	sort.Strings(got)
	log.Info("got: ", got)

	test.Equal(want, got)
}
