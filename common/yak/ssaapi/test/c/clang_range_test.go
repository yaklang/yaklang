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

		p, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.C))
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

		p, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.C))
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

	t.Run("test include with leading spaces", func(t *testing.T) {
		headerCode := `
#ifndef CONFIG_ADVANCED_H
#define CONFIG_ADVANCED_H

#define BASE_SIZE 64
#  include "config_extra.h"

#endif
		`

		extraHeader := `
#ifndef CONFIG_EXTRA_H
#define CONFIG_EXTRA_H

#define EXTRA_FACTOR 4

#endif
		`

		mainCode := `
#include "config_advanced.h"

int main() {
    int factor = EXTRA_FACTOR;
    int size = BASE_SIZE * factor;
    return size + factor;
}
		`

		vf := filesys.NewVirtualFs()
		vf.AddFile("src/config_advanced.h", headerCode)
		vf.AddFile("src/config_extra.h", extraHeader)
		vf.AddFile("src/main.c", mainCode)

		p, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.C))
		require.Nil(t, err)
		p.Show()

		results, err := p.SyntaxFlowWithError(`
			factor #-> as $factor
			size #-> as $size
		`, ssaapi.QueryWithEnableDebug())
		require.Nil(t, err)
		require.NotNil(t, results)
		results.Show()

		factor := results.GetValues("factor")
		check(t, factor, []string{
			"2:18 - 2:19: 4"},
			ssa.ToConstInst)

		size := results.GetValues("size")
		check(t, size, []string{
			"2:18 - 2:19: 4",
			"3:16 - 3:18: 64"},
			ssa.ToConstInst)
	})

	t.Run("test nested include chain", func(t *testing.T) {
		level1 := `
#ifndef LEVEL1_H
#define LEVEL1_H
#include "level2.h"
#define LEVEL1_VALUE 10
#endif
		`

		level2 := `
#ifndef LEVEL2_H
#define LEVEL2_H
#include "level3.h"
#define LEVEL2_VALUE LEVEL3_VALUE + 5
#endif
		`

		level3 := `
#ifndef LEVEL3_H
#define LEVEL3_H
#define LEVEL3_VALUE 20
#endif
		`

		mainCode := `
#include "level1.h"

int main() {
    int value = LEVEL1_VALUE + LEVEL2_VALUE;
    return value;
}
		`

		vf := filesys.NewVirtualFs()
		vf.AddFile("src/level1.h", level1)
		vf.AddFile("src/level2.h", level2)
		vf.AddFile("src/level3.h", level3)
		vf.AddFile("src/main.c", mainCode)

		p, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.C))
		require.Nil(t, err)

		results, err := p.SyntaxFlowWithError(`
			value #-> as $value
		`, ssaapi.QueryWithEnableDebug())
		require.Nil(t, err)
		require.NotNil(t, results)
		results.Show()

		value := results.GetValues("value")
		check(t, value, []string{
			"2:17 - 2:19: 10",
			"2:22 - 2:24: 20",
			"2:27 - 2:28: 5"},
			ssa.ToConstInst)
	})

	t.Run("test system include filtered", func(t *testing.T) {
		headerCode := `
#ifndef PLATFORM_CONFIG_H
#define PLATFORM_CONFIG_H
#  include <winerror.h>
#define PLATFORM_VALUE 7
#endif
		`

		mainCode := `
#include "platform_config.h"

int main() {
    int value = PLATFORM_VALUE;
    return value;
}
		`

		vf := filesys.NewVirtualFs()
		vf.AddFile("src/platform_config.h", headerCode)
		vf.AddFile("src/main.c", mainCode)

		p, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.C))
		require.Nil(t, err)

		results, err := p.SyntaxFlowWithError(`
			value #-> as $value
		`, ssaapi.QueryWithEnableDebug())
		require.Nil(t, err)
		require.NotNil(t, results)
		results.Show()

		value := results.GetValues("value")
		check(t, value, []string{
			"2:17 - 2:18: 7"},
			ssa.ToConstInst)
	})

}
