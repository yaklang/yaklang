package ssaapi

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func check[T ssa.Instruction](t *testing.T, gots ssaapi.Values, want []string, Cover func(ssa.Instruction) (T, bool)) {
	gotString := make([]string, 0, len(gots))
	for _, got := range gots {
		t, ok := Cover(got.GetSSAInst())
		if ok {
			gotString = append(gotString, t.GetRange().String())
		}
	}
	slices.Sort(want)
	slices.Sort(gotString)
	require.Equal(t, want, gotString)
}

func TestRange_SimpleMacro(t *testing.T) {

	t.Run("test simpleMacro", func(t *testing.T) {
		code := `
#define BUFFER_SIZE 512
#define SAFE_COPY(dest, src) strncpy(dest, src, BUFFER_SIZE - 1)

int main() {
    char buffer[BUFFER_SIZE];
    int size = BUFFER_SIZE;
    SAFE_COPY(buffer, "aaaaaaaaaaaaaaaaaaa");
    return 0;
}
		`

		vf := filesys.NewVirtualFs()
		vf.AddFile("src/main.c", code)

		cf, err := filesys.NewPreprocessedCFs(vf)
		require.Nil(t, err)

		p, err := ssaapi.ParseProjectWithFS(cf, ssaapi.WithLanguage(ssaconfig.C))
		require.Nil(t, err)

		results, err := p.SyntaxFlowWithError(`
			strncpy(* #-> as $target)
		`, ssaapi.QueryWithEnableDebug())
		results.Show()
		require.Nil(t, err)
		require.NotNil(t, results)

		ent := results.GetValues("target")
		check(t, ent, []string{
			"4:21 - 4:42: \"aaaaaaaaaaaaaaaaaaa\"",
			"4:44 - 4:47: 512",
			"4:50 - 4:51: 1"},
			ssa.ToConstInst)
	})

	t.Run("test macro with include", func(t *testing.T) {
		headerCode := `
#ifndef CONFIG_H
#define CONFIG_H

#define MAX_SIZE 1024
#define MIN_SIZE 128
#define MULTIPLY(a, b) ((a) * (b))

#endif
		`

		mainCode := `
#include "config.h"

int main() {
    int max = MAX_SIZE;
    int min = MIN_SIZE;
    int result = MULTIPLY(max, min);
    return 0;
}
		`

		vf := filesys.NewVirtualFs()
		vf.AddFile("src/config.h", headerCode)
		vf.AddFile("src/main.c", mainCode)

		cf, err := filesys.NewPreprocessedCFs(vf)
		require.Nil(t, err)

		p, err := ssaapi.ParseProjectWithFS(cf, ssaapi.WithLanguage(ssaconfig.C))
		require.Nil(t, err)

		results, err := p.SyntaxFlowWithError(`
			max #-> as $max
			min #-> as $min
			result #-> as $result
		`, ssaapi.QueryWithEnableDebug())
		require.Nil(t, err)
		require.NotNil(t, results)

		max := results.GetValues("max")
		check(t, max, []string{
			"2:15 - 2:19: 1024"},
			ssa.ToConstInst)

		min := results.GetValues("min")
		check(t, min, []string{
			"3:15 - 3:18: 128"},
			ssa.ToConstInst)

		result := results.GetValues("result")
		check(t, result, []string{
			"2:15 - 2:19: 1024",
			"3:15 - 3:18: 128"},
			ssa.ToConstInst)
	})

}
