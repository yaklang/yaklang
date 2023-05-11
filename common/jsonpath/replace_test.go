package jsonpath

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"sort"
	"strings"
	"testing"
)

type replaceTestCase struct {
	Raw      string
	Expected []string
	Replaced any
}

func TestFindKeys(t *testing.T) {
	for _, c := range []*replaceTestCase{
		{
			Raw: `{"a":1,"b":{"c":"d"}}`,
			Expected: []string{
				"a", "b", "c",
			},
		},
	} {
		result := fetchAllIterKey(c.Raw)
		spew.Dump(result)
	}
}

func TestReplace2(t *testing.T) {
	for _, c := range []*replaceTestCase{
		{
			Raw:      `{"a":1,"b":{"c":"d"}}`,
			Replaced: 23,
			Expected: []string{
				`{"a":23,"b":{"c":"d"}}`,
				`{"a":1,"b":{"c":23}}`,
				`{"a":1,"b":23}`,
			},
		},
		{
			Raw:      `{"a":1,"b":{"c":"d"}}`,
			Replaced: "112",
			Expected: []string{
				`{"a":"112","b":{"c":"d"}}`,
				`{"a":1,"b":{"c":"112"}}`,
				`{"a":1,"b":"112"}`,
			},
		},
	} {
		result := RecursiveDeepReplaceString(c.Raw, c.Replaced)
		if result == nil {
			spew.Dump(c)
			spew.Dump(result)
			panic(1)
		}
		sort.SliceStable(c.Expected, func(i, j int) bool {
			return c.Expected[i] < c.Expected[j]
		})
		sort.SliceStable(result, func(i, j int) bool {
			return result[i] < result[j]
		})
		if len(result) != len(c.Expected) {
			spew.Dump(c)
			spew.Dump(result)
			panic(1)
		}

		hash1, hash2 := utils.CalcSha1(strings.Join(c.Expected, "|")), utils.CalcSha1(strings.Join(result, "|"))
		if hash2 != hash1 {
			spew.Dump(c)
			spew.Dump(result)
			panic(1)
		}
		spew.Dump(result)
		spew.Dump("h1", hash1, "h2", hash2)
	}
}
