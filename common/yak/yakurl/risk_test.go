package yakurl_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yakurl"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type filter struct {
	level yakurl.SSARiskResponseLevel
	node  bool
	// filter
	program   string
	rule      string
	source    string
	function  string
	search    string
	result_id uint64
	task_id   string
}

func TestSSARiskRequestParse(t *testing.T) {
	// yakurl.GetSSARiskCountFilter()
	check := func(t *testing.T, path string, param map[string]string, want filter) {
		url := &ypb.YakURL{
			Schema: "ssarisk",
			Path:   path,
		}
		for k, v := range param {
			url.Query = append(url.Query, &ypb.KVPair{
				Key: k, Value: v,
			})
		}
		res, err := yakurl.GetSSARiskCountFilter(url)
		require.NoError(t, err)
		// spew.Dump(res)
		got := filter{
			level: res.Level,
			node:  res.LeafNode,
		}
		if len(res.Filter.ProgramName) > 0 {
			got.program = res.Filter.ProgramName[0]
			if len(res.Filter.ProgramName) > 1 {
				t.Fatalf("expected only one program name, got %d: %v", len(res.Filter.ProgramName), res.Filter.ProgramName)
			}
		}

		if len(res.Filter.FromRule) > 0 {
			got.rule = res.Filter.FromRule[0]
			if len(res.Filter.FromRule) > 1 {
				t.Fatalf("expected only one rule, got %d: %v", len(res.Filter.FromRule), res.Filter.FromRule)
			}
		}

		if len(res.Filter.CodeSourceUrl) > 0 {
			got.source = res.Filter.CodeSourceUrl[0]
			if len(res.Filter.CodeSourceUrl) > 1 {
				t.Fatalf("expected only one source URL, got %d: %v", len(res.Filter.CodeSourceUrl), res.Filter.CodeSourceUrl)
			}
		}

		if len(res.Filter.FunctionName) > 0 {
			got.function = res.Filter.FunctionName[0]
			if len(res.Filter.FunctionName) > 1 {
				t.Fatalf("expected only one function name, got %d: %v", len(res.Filter.FunctionName), res.Filter.FunctionName)
			}
		}

		if len(res.Filter.ResultID) > 0 {
			got.result_id = res.Filter.ResultID[0]
			if len(res.Filter.ResultID) > 1 {
				t.Fatalf("expected only one result ID, got %d: %v", len(res.Filter.ResultID), res.Filter.ResultID)
			}
		}

		if len(res.Filter.RuntimeID) > 0 {
			got.task_id = res.Filter.RuntimeID[0]
			if len(res.Filter.RuntimeID) > 1 {
				t.Fatalf("expected only one runtime ID, got %d: %v", len(res.Filter.RuntimeID), res.Filter.RuntimeID)
			}
		}

		got.search = res.Filter.Search
		require.Equal(t, want, got)
	}

	//{{{ // check type=""		path:="${program}/${source}/${function}(leaf)/${risk}"
	t.Run("check get program", func(t *testing.T) {
		check(t, "/", nil, filter{
			level: yakurl.SSARiskLevelProgram,
			node:  false,
		})
	})

	t.Run("check get source", func(t *testing.T) {
		check(t, "/paaaa", nil, filter{
			level:   yakurl.SSARiskLevelSource,
			node:    false,
			program: "paaaa",
		})
	})
	t.Run("check get function", func(t *testing.T) {
		check(t, "/proaaa/bbb.go", nil, filter{
			level:   yakurl.SSARiskLevelFunction,
			node:    true,
			program: "proaaa",
			source:  "/bbb.go",
		})
	})
	t.Run("check get risk", func(t *testing.T) {
		check(t, "/paaa/bb.go/ff", nil, filter{
			level:    yakurl.SSARiskLevelRisk,
			node:     true,
			program:  "paaa",
			source:   "/bb.go",
			function: "ff",
		})
	})

	t.Run("check get risk with file type ", func(t *testing.T) {
		check(t, "/proaaa/bbb.go", map[string]string{
			"type": string(yakurl.SSARiskTypeFile),
		}, filter{
			level:   yakurl.SSARiskLevelRisk,
			node:    true,
			program: "proaaa",
			source:  "/bbb.go",
		})
	})
	// }}}

	// {{{ param["program"]  path:="/${source}/${function}(leaf)/${risk}(leaf)"
	t.Run("check get source by program param", func(t *testing.T) {
		check(t, "/", map[string]string{
			"program": "p",
		}, filter{
			level:   yakurl.SSARiskLevelSource,
			node:    false,
			program: "p",
		})
	})

	t.Run("check get function by program param", func(t *testing.T) {
		check(t, "/ssss.go", map[string]string{
			"program": "ppp",
		}, filter{
			level:   yakurl.SSARiskLevelFunction,
			node:    true,
			program: "ppp",
			source:  "/ssss.go",
		})
	})

	t.Run("check get risk by program param", func(t *testing.T) {
		check(t, "/ssss.go/fff", map[string]string{
			"program": "pppp",
		}, filter{
			level:    yakurl.SSARiskLevelRisk,
			node:     true,
			program:  "pppp",
			source:   "/ssss.go",
			function: "fff",
		})
	})
	// }}}

	//{{{ param["type"]="rule" 			// 	path:="${program}/${rule}/${path}/${risk}"
	t.Run("check get program with rule type", func(t *testing.T) {
		check(t, "/", map[string]string{
			"type": string(yakurl.SSARiskTypeRule),
		}, filter{
			level: yakurl.SSARiskLevelProgram,
			node:  false,
		})
	})

	t.Run("check get rule with rule type", func(t *testing.T) {
		check(t, "/aa", map[string]string{
			"type": string(yakurl.SSARiskTypeRule),
		}, filter{
			level:   yakurl.SSARiskLevelRule,
			node:    false,
			program: "aa",
		})
	})

	t.Run("check get source with rule type", func(t *testing.T) {
		check(t, "/aa/bb", map[string]string{
			"type": string(yakurl.SSARiskTypeRule),
		}, filter{
			level:   yakurl.SSARiskLevelSource,
			node:    false,
			program: "aa",
			rule:    "bb",
		})
	})

	t.Run("check get risk with rule type", func(t *testing.T) {
		check(t, "/aa/bb/cc.go", map[string]string{
			"type": string(yakurl.SSARiskTypeRule),
		}, filter{
			level:   yakurl.SSARiskLevelRisk,
			node:    true,
			program: "aa",
			rule:    "bb",
			source:  "/cc.go",
		})
	})
	// }}}

	//{{{ param["program"]  		path:="/${rule}/${path}/${risk}"
	t.Run("check rule-type get rule with program param", func(t *testing.T) {
		check(t, "/", map[string]string{
			"program": "pppp",
			"type":    string(yakurl.SSARiskTypeRule),
		}, filter{
			level:   yakurl.SSARiskLevelRule,
			node:    false,
			program: "pppp",
		})
	})

	t.Run("check rule-type get source with program param", func(t *testing.T) {
		check(t, "/bb", map[string]string{
			"program": "pppp",
			"type":    string(yakurl.SSARiskTypeRule),
		}, filter{
			level:   yakurl.SSARiskLevelSource,
			node:    false,
			program: "pppp",
			rule:    "bb",
		})
	})

	t.Run("check rule-type get risk with program param", func(t *testing.T) {
		check(t, "/bb/cc.go", map[string]string{
			"program": "pppp",
			"type":    string(yakurl.SSARiskTypeRule),
		}, filter{
			level:   yakurl.SSARiskLevelRisk,
			node:    true,
			program: "pppp",
			rule:    "bb",
			source:  "/cc.go",
		})
	})
	//}}}

	//{{{ param["rule"]  // 	path:="${program}/${source}/${risk}"
	t.Run("check rule-type get program with rule param", func(t *testing.T) {
		check(t, "/", map[string]string{
			"rule": "rrrr",
			"type": string(yakurl.SSARiskTypeRule),
		}, filter{
			level: yakurl.SSARiskLevelProgram,
			node:  false,
			rule:  "rrrr",
		})
	})

	t.Run("check rule-type get source with rule param", func(t *testing.T) {
		check(t, "/pppp", map[string]string{
			"rule": "rrrr",
			"type": string(yakurl.SSARiskTypeRule),
		}, filter{
			level:   yakurl.SSARiskLevelSource,
			node:    false,
			program: "pppp",
			rule:    "rrrr",
		})
	})

	t.Run("check rule-type get risk with rule param", func(t *testing.T) {
		check(t, "/pppp/cc.go", map[string]string{
			"rule": "rrrr",
			"type": string(yakurl.SSARiskTypeRule),
		}, filter{
			level:   yakurl.SSARiskLevelRisk,
			node:    true,
			program: "pppp",
			rule:    "rrrr",
			source:  "/cc.go",
		})
	})
	// }}}

	// search
	t.Run("check get risk with search param", func(t *testing.T) {
		check(t, "/paaa", map[string]string{
			"search": "ssssss",
		}, filter{
			level:   yakurl.SSARiskLevelSource,
			node:    false,
			program: "paaa",
			search:  "ssssss",
		})
	})

	// task id
	t.Run("check get root with task-id param", func(t *testing.T) {
		check(t, "/", map[string]string{
			"task_id": "ttttttt",
		}, filter{
			level:   yakurl.SSARiskLevelProgram,
			node:    false,
			task_id: "ttttttt",
		})
	})

	t.Run("check get root with result-id param", func(t *testing.T) {
		check(t, "/", map[string]string{
			"result_id": "123",
		}, filter{
			level:     yakurl.SSARiskLevelProgram,
			node:      false,
			result_id: 123,
		})
	})
}

func initRiskTest(t *testing.T, programName, RuntimeId string, extendPaths ...string) {
	db := ssadb.GetDB()
	/*
		--programName (5)
			--a.go (1)
				-- funcA (1)
					-- test1
			--b/b1.go (1)
				-- funcB1 (1)
					-- test2
			--b/b2.go (1)
				-- funcB2 (1)
					-- test3
			--c.go (2)
				-- funcC (2)
					-- test4
					-- test5
		--programName(5)
			-- rule1 (1)
				-- /a.go (1)
					-- test1
			-- rule2 (2)
				-- /b/b1.go (1)
					-- test2
				-- /c.go (1)
					-- test4
			-- rule3 (1)
				-- /b/b2.go (1)
					-- test3
			-- "" (1)
				-- /c.go (1)
					-- test5
	*/
	extendPath := ""
	if len(extendPaths) > 0 {
		extendPath = "/" + strings.Join(extendPaths, "/")
	}

	err := yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:     programName,
		CodeSourceUrl:   fmt.Sprintf("/%s%s/a.go", programName, extendPath),
		FunctionName:    "funcA",
		Title:           "test1",
		FromRule:        "rule1",
		RuntimeId:       RuntimeId,
		ResultID:        1,
		Variable:        "a",
		Index:           1,
		RiskFeatureHash: fmt.Sprintf("%s-test1-hash", programName), // 伪造RiskFeatureHash
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:     programName,
		CodeSourceUrl:   fmt.Sprintf("/%s%s/b/b1.go", programName, extendPath),
		FunctionName:    "funcB1",
		Title:           "test2",
		FromRule:        "rule2",
		RuntimeId:       RuntimeId,
		ResultID:        2,
		Variable:        "b1",
		Index:           2,
		RiskFeatureHash: fmt.Sprintf("%s-test2-hash", programName), // 伪造RiskFeatureHash
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:     programName,
		CodeSourceUrl:   fmt.Sprintf("/%s%s/b/b2.go", programName, extendPath),
		FunctionName:    "funcB2",
		Title:           "test3",
		FromRule:        "rule3",
		RuntimeId:       RuntimeId,
		ResultID:        3,
		Variable:        "b2",
		Index:           3,
		RiskFeatureHash: fmt.Sprintf("%s-test3-hash", programName), // 伪造RiskFeatureHash
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:     programName,
		CodeSourceUrl:   fmt.Sprintf("/%s%s/c.go", programName, extendPath),
		FunctionName:    "funcC",
		Title:           "test4",
		FromRule:        "rule2",
		RuntimeId:       RuntimeId,
		ResultID:        4,
		Variable:        "c",
		Index:           4,
		RiskFeatureHash: fmt.Sprintf("%s-test4-hash", programName), // 伪造RiskFeatureHash
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:     programName,
		CodeSourceUrl:   fmt.Sprintf("/%s%s/c.go", programName, extendPath),
		FunctionName:    "funcC",
		Title:           "test5",
		FromRule:        "",
		RuntimeId:       RuntimeId,
		ResultID:        5,
		Variable:        "c",
		Index:           5,
		RiskFeatureHash: fmt.Sprintf("%s-test5-hash", programName), // 伪造RiskFeatureHash
	})
	require.NoError(t, err)
}

type data struct {
	Name  string
	Type  string
	Count int
}

func GetSSARisk(t *testing.T, local ypb.YakClient, url *ypb.YakURL) map[string]data {
	req := &ypb.RequestYakURLParams{
		Method: "GET",
		Url:    url,
	}
	res, err := local.RequestYakURL(context.Background(), req)
	require.NoError(t, err)
	got := make(map[string]data)
	for _, resource := range res.GetResources() {
		// check got exist this path
		if _, ok := got[resource.Path]; ok {
			require.Fail(t, "resource path same  %v", res.GetResources())
		}
		var count, filterCount int
		var result_id, variable, index string
		log.Infof("resource: %v", resource)
		for _, extra := range resource.Extra {
			if extra.Key == "count" {
				count, err = strconv.Atoi(extra.Value)
				require.NoError(t, err)
			}
			if extra.Key == "filter" {
				var filter *ypb.SSARisksFilter
				err = json.Unmarshal([]byte(extra.Value), &filter)
				log.Infof("filter: %v", filter)
				require.NoError(t, err)
				paging, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), filter, nil)
				_ = risks
				require.NoError(t, err)
				filterCount = paging.TotalRecord
			}
			if extra.Key == "hash" {
				paging, _, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
					Hash: []string{extra.Value},
				}, nil)
				require.NoError(t, err)
				require.Equal(t, 1, paging.TotalRecord, "result_hash not exist: %s", extra.Value)
			}

			if extra.Key == "severity" {
				// haveSeverity := true
				require.Contains(t, schema.GetAllSFSeverityTypes(), string(schema.ValidSeverityType(extra.Value)))
			}

			if extra.Key == "result_id" {
				result_id = extra.Value
			}
			if extra.Key == "variable" {
				variable = extra.Value
			}
			if extra.Key == "index" {
				index = extra.Value
			}
			// }
		}

		if resource.ResourceType == string(yakurl.SSARiskLevelRisk) {
			require.NotEmpty(t, result_id)
			require.NotEmpty(t, variable)
			require.NotEmpty(t, index)
			// TODO: this risk data build manual
			// data, err := SendURL(local, fmt.Sprintf("/%s/%s", variable, index), result_id)
			// log.Info("data--", data)
			// require.NoError(t, err)
		}
		require.Equal(t, count, filterCount, "filter count not equal with msg count")
		got[resource.Path] = data{
			Name:  resource.ResourceName,
			Type:  resource.ResourceType,
			Count: count,
		}
	}
	return got
}

func urlProgramPath(progName string) string {
	return "/" + progName
}

func sourcePath(progName, source string) string {
	return path.Join("/", progName, source)
}

func urlPath(progName, source string) string {
	return path.Join(urlProgramPath(progName), sourcePath(progName, source))
}

func urlFunctionPath(progName, source, function string) string {
	return path.Join(urlPath(progName, source), function)
}

func urlRulePath(progName, ruleName string) string {
	return path.Join(urlProgramPath(progName), ruleName)
}

func urlRuleSourcePath(progName, ruleName, source string) string {
	return path.Join(urlProgramPath(progName), ruleName, sourcePath(progName, source))
}

func TestRiskAction(t *testing.T) {
	programName1 := uuid.NewString()
	initRiskTest(t, programName1, "")
	programName2 := uuid.NewString()
	initRiskTest(t, programName2, "")

	t.Cleanup(func() {
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{programName1}})
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{programName2}})
	})
	local, err := yakgrpc.NewLocalClient()
	if err != nil {
		t.Error(err)
	}

	check := func(path, search, program string, want map[string]data, contain ...bool) {
		url := &ypb.YakURL{
			Schema: "ssarisk",
			Path:   path,
			Query: []*ypb.KVPair{
				{Key: "search", Value: search},
				{Key: "program", Value: program},
			},
		}
		got := GetSSARisk(t, local, url)
		log.Infof("got: %v", got)
		log.Infof("want: %v", want)
		if len(contain) > 0 && contain[0] {
			// Check if got contains all entries from want
			for wantPath, wantData := range want {
				gotData, exists := got[wantPath]
				require.True(t, exists, "Path %s not found in results", wantPath)
				require.Equal(t, wantData.Name, gotData.Name, "Name mismatch for path %s", wantPath)
				require.Equal(t, wantData.Type, gotData.Type, "Type mismatch for path %s", wantPath)
				require.Equal(t, wantData.Count, gotData.Count, "Count mismatch for path %s", wantPath)
			}
		} else {
			require.Equal(t, want, got)
		}
	}
	checkPathAndSearch := func(path, search string, want map[string]data, contain ...bool) {
		check(path, search, "", want, contain...)
	}

	t.Run("check path root", func(t *testing.T) {
		// ssarisk://
		// contain two  program is ok
		checkPathAndSearch("/", "", map[string]data{
			urlProgramPath(programName1): {
				Name:  programName1,
				Type:  "program",
				Count: 5,
			},
			urlProgramPath(programName2): {
				Name:  programName2,
				Type:  "program",
				Count: 5,
			},
		}, true)
	})

	t.Run("check path program", func(t *testing.T) {
		// ssarisk://program
		checkPathAndSearch(urlProgramPath(programName1), "", map[string]data{
			urlPath(programName1, "a.go"): {
				Name:  sourcePath(programName1, "a.go"),
				Type:  "source",
				Count: 1,
			},
			urlPath(programName1, "b/b1.go"): {
				Name:  sourcePath(programName1, "b/b1.go"),
				Type:  "source",
				Count: 1,
			},
			urlPath(programName1, "b/b2.go"): {
				Name:  sourcePath(programName1, "b/b2.go"),
				Type:  "source",
				Count: 1,
			},
			urlPath(programName1, "c.go"): {
				Name:  sourcePath(programName1, "c.go"),
				Type:  "source",
				Count: 2,
			},
		})
	})

	t.Run("check path with program param", func(t *testing.T) {
		check("/", "", programName1, map[string]data{
			sourcePath(programName1, "a.go"): {
				Name:  sourcePath(programName1, "a.go"),
				Type:  "source",
				Count: 1,
			},
			sourcePath(programName1, "b/b1.go"): {
				Name:  sourcePath(programName1, "b/b1.go"),
				Type:  "source",
				Count: 1,
			},
			sourcePath(programName1, "b/b2.go"): {
				Name:  sourcePath(programName1, "b/b2.go"),
				Type:  "source",
				Count: 1,
			},
			sourcePath(programName1, "c.go"): {
				Name:  sourcePath(programName1, "c.go"),
				Type:  "source",
				Count: 2,
			},
		})
	})

	t.Run("check path source", func(t *testing.T) {
		// ssarisk://program/c.go
		checkPathAndSearch(urlPath(programName1, "c.go"), "", map[string]data{
			urlFunctionPath(programName1, "c.go", "funcC"): {
				Name:  "funcC",
				Type:  "function",
				Count: 2,
			},
		})
	})
	t.Run("check function with program param", func(t *testing.T) {
		path := sourcePath(programName1, "/c.go")
		check(path, "", programName1, map[string]data{
			path + "/funcC": {
				Name:  "funcC",
				Type:  "function",
				Count: 2,
			},
		})
	})

	t.Run("check path function get risk", func(t *testing.T) {
		// checkPathAndSearch(urlFunctionPath(programName1, "c.go", "funcC"), "", map[string]data{})
		url := &ypb.YakURL{
			Schema: "ssarisk",
			Path:   urlFunctionPath(programName1, "c.go", "funcC"),
		}
		got := GetSSARisk(t, local, url)
		log.Infof("got: %v", got)
		require.Equal(t, len(got), 2)
		gotTitle := lo.MapToSlice(got, func(key string, value data) string {
			return value.Name
		})
		sort.Strings(gotTitle)
		require.Equal(t, gotTitle, []string{"test4", "test5"})
	})

	t.Run("check search source(file)", func(t *testing.T) {
		// ssarisk://?search=/c.go
		checkPathAndSearch("/", "/c.go", map[string]data{
			urlProgramPath(programName1): {
				Name:  programName1,
				Type:  "program",
				Count: 2,
			},
			urlProgramPath(programName2): {
				Name:  programName2,
				Type:  "program",
				Count: 2,
			},
		}, true)
	})

	t.Run("check search source(dir)", func(t *testing.T) {
		// ssarisk://?search=/b/
		checkPathAndSearch("/", "/b/", map[string]data{
			urlProgramPath(programName1): {
				Name:  programName1,
				Type:  "program",
				Count: 2,
			},
			urlProgramPath(programName2): {
				Name:  programName2,
				Type:  "program",
				Count: 2,
			},
		}, true)
	})

	t.Run("check search function", func(t *testing.T) {
		// ssarisk://?search=funcA
		checkPathAndSearch("/", "funcA", map[string]data{
			urlProgramPath(programName1): {
				Name:  programName1,
				Type:  "program",
				Count: 1,
			},
			urlProgramPath(programName2): {
				Name:  programName2,
				Type:  "program",
				Count: 1,
			},
		}, true)
	})

	t.Run("check search function fuzzy", func(t *testing.T) {
		// ssarisk://?search=func
		checkPathAndSearch("/", "func", map[string]data{
			urlProgramPath(programName1): {
				Name:  programName1,
				Type:  "program",
				Count: 5,
			},
			urlProgramPath(programName2): {
				Name:  programName2,
				Type:  "program",
				Count: 5,
			},
		}, true)
	})

	t.Run("check path program and search source", func(t *testing.T) {
		// ssarisk://program/?search=/b
		// 注意：搜索'/b'会匹配所有包含字符'b'的字段，不仅仅是路径
		// 所以我们使用包含性检查，确保期望的结果都包含在实际结果中
		checkPathAndSearch(urlProgramPath(programName1), "/b", map[string]data{
			urlPath(programName1, "b/b1.go"): {
				Name:  sourcePath(programName1, "b/b1.go"),
				Type:  "source",
				Count: 1,
			},
			urlPath(programName1, "b/b2.go"): {
				Name:  sourcePath(programName1, "b/b2.go"),
				Type:  "source",
				Count: 1,
			},
		}, true) // 使用包含性检查
	})

	t.Run("check path program and search function", func(t *testing.T) {
		// ssarisk://program/?search=/funcB1
		checkPathAndSearch(urlProgramPath(programName1), "funcB1", map[string]data{
			urlPath(programName1, "b/b1.go"): {
				Name:  sourcePath(programName1, "b/b1.go"),
				Type:  "source",
				Count: 1,
			},
		})
	})

	t.Run("check path source and search function", func(t *testing.T) {
		// ssarisk://program/b/?search=/funcB1
		checkPathAndSearch(urlPath(programName1, "b/b1.go"), "funcB1", map[string]data{
			urlFunctionPath(programName1, "b/b1.go", "funcB1"): {
				Name:  "funcB1",
				Type:  "function",
				Count: 1,
			},
		})
	})

	t.Run("check path function and search function but not find", func(t *testing.T) {
		// ssarisk://program/b/?search=/funcB1
		checkPathAndSearch(urlFunctionPath(programName1, "b/b1.go", "funcB1"), "funcB2", map[string]data{})
	})

}

func TestRiskActionRule(t *testing.T) {
	/*
		测试架构图 - Program、Task、RiskFeatureHash 关系：

		Program1 (旧扫描)              Program2 (新扫描)
		├─ TaskID1                     ├─ TaskID2
		│  ├─ Risk1: hash-test1  ──────┼─→ Risk1: hash-test1  (相同Hash)
		│  ├─ Risk2: hash-test2  ──────┼─→ Risk2: hash-test2  (相同Hash)
		│  ├─ Risk3: hash-test3  ──────┼─→ Risk3: hash-test3  (相同Hash)
		│  ├─ Risk4: hash-test4  ──────┼─→ Risk4: hash-test4  (相同Hash)
		│  └─ Risk5: hash-test5  ──────┼─→ Risk5: hash-test5  (相同Hash)
		                               └─→ Risk6: hash-test6-new  (新增)

		Diff对比逻辑：
		- 基于RiskFeatureHash进行对比
		- 相同Hash的风险被过滤（认为是已存在的）
		- 只返回新增的风险：Risk6 (Count=1)
	*/

	programName1 := uuid.NewString()
	programName2 := uuid.NewString()
	taskID1 := uuid.NewString() // 旧的扫描结果
	taskID2 := uuid.NewString() // 新的扫描结果

	// 创建相同的基础数据，使用相同的RiskFeatureHash（模拟相同的代码基础）
	baseHash := "common-base-hash" // 使用相同的Hash前缀

	// 为程序1创建基础风险（旧版本）
	err := yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
		ProgramName:     programName1,
		CodeSourceUrl:   fmt.Sprintf("/%s/a.go", programName1),
		FunctionName:    "funcA",
		Title:           "test1",
		FromRule:        "rule1",
		RuntimeId:       taskID1,
		ResultID:        1,
		Variable:        "a",
		Index:           1,
		RiskFeatureHash: baseHash + "-test1", // 相同的Hash
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
		ProgramName:     programName1,
		CodeSourceUrl:   fmt.Sprintf("/%s/b/b1.go", programName1),
		FunctionName:    "funcB1",
		Title:           "test2",
		FromRule:        "rule2",
		RuntimeId:       taskID1,
		ResultID:        2,
		Variable:        "b1",
		Index:           2,
		RiskFeatureHash: baseHash + "-test2", // 相同的Hash
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
		ProgramName:     programName1,
		CodeSourceUrl:   fmt.Sprintf("/%s/b/b2.go", programName1),
		FunctionName:    "funcB2",
		Title:           "test3",
		FromRule:        "rule3",
		RuntimeId:       taskID1,
		ResultID:        3,
		Variable:        "b2",
		Index:           3,
		RiskFeatureHash: baseHash + "-test3", // 相同的Hash
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
		ProgramName:     programName1,
		CodeSourceUrl:   fmt.Sprintf("/%s/c.go", programName1),
		FunctionName:    "funcC",
		Title:           "test4",
		FromRule:        "rule2",
		RuntimeId:       taskID1,
		ResultID:        4,
		Variable:        "c",
		Index:           4,
		RiskFeatureHash: baseHash + "-test4", // 相同的Hash
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
		ProgramName:     programName1,
		CodeSourceUrl:   fmt.Sprintf("/%s/c.go", programName1),
		FunctionName:    "funcC",
		Title:           "test5",
		FromRule:        "",
		RuntimeId:       taskID1,
		ResultID:        5,
		Variable:        "c",
		Index:           5,
		RiskFeatureHash: baseHash + "-test5", // 相同的Hash
	})
	require.NoError(t, err)

	// 为程序2创建相同的基础风险（新版本）+ 一个新增风险
	err = yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
		ProgramName:     programName2,
		CodeSourceUrl:   fmt.Sprintf("/%s/a.go", programName2),
		FunctionName:    "funcA",
		Title:           "test1",
		FromRule:        "rule1",
		RuntimeId:       taskID2,
		ResultID:        1,
		Variable:        "a",
		Index:           1,
		RiskFeatureHash: baseHash + "-test1", // 相同的Hash，diff时会被过滤
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
		ProgramName:     programName2,
		CodeSourceUrl:   fmt.Sprintf("/%s/b/b1.go", programName2),
		FunctionName:    "funcB1",
		Title:           "test2",
		FromRule:        "rule2",
		RuntimeId:       taskID2,
		ResultID:        2,
		Variable:        "b1",
		Index:           2,
		RiskFeatureHash: baseHash + "-test2", // 相同的Hash，diff时会被过滤
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
		ProgramName:     programName2,
		CodeSourceUrl:   fmt.Sprintf("/%s/b/b2.go", programName2),
		FunctionName:    "funcB2",
		Title:           "test3",
		FromRule:        "rule3",
		RuntimeId:       taskID2,
		ResultID:        3,
		Variable:        "b2",
		Index:           3,
		RiskFeatureHash: baseHash + "-test3", // 相同的Hash，diff时会被过滤
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
		ProgramName:     programName2,
		CodeSourceUrl:   fmt.Sprintf("/%s/c.go", programName2),
		FunctionName:    "funcC",
		Title:           "test4",
		FromRule:        "rule2",
		RuntimeId:       taskID2,
		ResultID:        4,
		Variable:        "c",
		Index:           4,
		RiskFeatureHash: baseHash + "-test4", // 相同的Hash，diff时会被过滤
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
		ProgramName:     programName2,
		CodeSourceUrl:   fmt.Sprintf("/%s/c.go", programName2),
		FunctionName:    "funcC",
		Title:           "test5",
		FromRule:        "",
		RuntimeId:       taskID2,
		ResultID:        5,
		Variable:        "c",
		Index:           5,
		RiskFeatureHash: baseHash + "-test5", // 相同的Hash，diff时会被过滤
	})
	require.NoError(t, err)

	// 新增的风险（只在程序2中存在，diff时会被识别）
	err = yakit.CreateSSARisk(ssadb.GetDB(), &schema.SSARisk{
		ProgramName:     programName2,
		CodeSourceUrl:   fmt.Sprintf("/%s/d.go", programName2),
		FunctionName:    "funcD",
		Title:           "test6",
		FromRule:        "rule4",
		RuntimeId:       taskID2,
		ResultID:        6,
		Variable:        "d",
		Index:           6,
		RiskFeatureHash: baseHash + "-test6-new", // 新增风险的独特Hash
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{programName1}})
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{programName2}})
	})
	local, err := yakgrpc.NewLocalClient()
	if err != nil {
		t.Error(err)
	}

	checkRuleAndSearch := func(path, search, base, compare string, want map[string]data, contain ...bool) {
		url := &ypb.YakURL{
			Schema: "ssarisk",
			Path:   path,
			Query: []*ypb.KVPair{
				{Key: "search", Value: search},
				{Key: "type", Value: "rule"},
				{Key: "task_id", Value: base},
				{Key: "compare", Value: compare},
			},
		}
		got := GetSSARisk(t, local, url)
		log.Infof("got: %v", got)
		log.Infof("want: %v", want)
		if len(contain) > 0 && contain[0] {
			// Check if got contains all entries from want
			for wantPath, wantData := range want {
				gotData, exists := got[wantPath]
				require.True(t, exists, "Path %s not found in results", wantPath)
				require.Equal(t, wantData.Name, gotData.Name, "Name mismatch for path %s", wantPath)
				require.Equal(t, wantData.Type, gotData.Type, "Type mismatch for path %s", wantPath)
				require.Equal(t, wantData.Count, gotData.Count, "Count mismatch for path %s", wantPath)
			}
		} else {
			require.Equal(t, want, got)
		}
	}
	t.Run("check rule root get program", func(t *testing.T) {
		// ssarisk://?type=rule
		// get program
		checkRuleAndSearch("/", "", "", "", map[string]data{
			urlProgramPath(programName1): {
				Name:  programName1,
				Type:  "program",
				Count: 5,
			},
			urlProgramPath(programName2): {
				Name:  programName2,
				Type:  "program",
				Count: 6,
			},
		}, true)
	})

	t.Run("check rule program get rule", func(t *testing.T) {
		// ssarisk://program?type=rule
		// get rule
		checkRuleAndSearch(urlProgramPath(programName1), "", "", "", map[string]data{
			urlRulePath(programName1, "rule1"): {
				Name:  "rule1",
				Type:  "rule",
				Count: 1,
			},
			urlRulePath(programName1, "rule2"): {
				Name:  "rule2",
				Type:  "rule",
				Count: 2,
			},
			urlRulePath(programName1, "rule3"): {
				Name:  "rule3",
				Type:  "rule",
				Count: 1,
			},
		})
	})

	t.Run("check rule get source", func(t *testing.T) {
		// ssarisk://program/ruleName?type=rule
		// get source
		rule2Path := urlRulePath(programName1, "rule2")            // /program1/rule2
		b1 := urlRuleSourcePath(programName1, "rule2", "/b/b1.go") //  /program1/rule2/b/b1.go
		c := urlRuleSourcePath(programName1, "rule2", "c.go")      //  /program1/rule2/c.go
		checkRuleAndSearch(rule2Path, "", "", "", map[string]data{
			b1: {
				Name:  sourcePath(programName1, "b/b1.go"),
				Type:  "source",
				Count: 1,
			},
			c: {
				Name:  sourcePath(programName1, "c.go"),
				Type:  "source",
				Count: 1,
			},
		})
	})

	t.Run("check source get risk", func(t *testing.T) {
		// checkRuleAndSearch(urlRuleSourcePath(programName1, "rule2", "b/b1.go"), "", map[string]data{})
		url := &ypb.YakURL{
			Schema: "ssarisk",
			Path:   urlRuleSourcePath(programName1, "rule2", "b/b1.go"),
			Query:  []*ypb.KVPair{{Key: "type", Value: "rule"}},
		}
		got := GetSSARisk(t, local, url)
		log.Infof("got: %v", got)
		require.Equal(t, len(got), 1)
		gotTitle := lo.MapToSlice(got, func(key string, value data) string {
			return value.Name
		})
		sort.Strings(gotTitle)
		require.Equal(t, gotTitle, []string{"test2"})
	})

	t.Run("check rule program search", func(t *testing.T) {
		// ssarisk://program?type=rule&search=test1
		checkRuleAndSearch(urlProgramPath(programName1), "rule1", "", "", map[string]data{
			urlRulePath(programName1, "rule1"): {
				Name:  "rule1",
				Type:  "rule",
				Count: 1,
			},
		})
	})

	t.Run("check rule program search source", func(t *testing.T) {
		// ssarisk://program?type=rule&search=/c.go
		checkRuleAndSearch(urlProgramPath(programName1), "/c.go", "", "", map[string]data{
			urlRulePath(programName1, "rule2"): {
				Name:  "rule2",
				Type:  "rule",
				Count: 1,
			},
		})
	})

	t.Run("check rule program search function", func(t *testing.T) {
		// ssarisk://program?type=rule&search=funcC
		checkRuleAndSearch(urlProgramPath(programName1), "funcC", "", "", map[string]data{
			urlRulePath(programName1, "rule2"): {
				Name:  "rule2",
				Type:  "rule",
				Count: 1,
			},
		})
	})

	t.Run("check rule path search source", func(t *testing.T) {
		// ssarisk://program/rule2?type=rule&search=/c.go
		checkRuleAndSearch(urlRulePath(programName1, "rule2"), "/c.go", "", "", map[string]data{
			urlRuleSourcePath(programName1, "rule2", "c.go"): {
				Name:  sourcePath(programName1, "c.go"),
				Type:  "source",
				Count: 1,
			},
		})
		checkRuleAndSearch(urlRulePath(programName1, "rule2"), "/b.go", "", "", map[string]data{})
	})

	t.Run("check rule path search function", func(t *testing.T) {
		// ssarisk://program/rule2?type=rule&search=funcC
		checkRuleAndSearch(urlRulePath(programName1, "rule2"), "funcC", "", "", map[string]data{
			urlRuleSourcePath(programName1, "rule2", "c.go"): {
				Name:  sourcePath(programName1, "c.go"),
				Type:  "source",
				Count: 1,
			},
		})
		checkRuleAndSearch(urlRulePath(programName1, "rule2"), "funcA", "", "", map[string]data{})
	})

	t.Run("check rule root get program with diff", func(t *testing.T) {
		// ssarisk://?type=rule&compare={runtimeId}
		// get program
		checkRuleAndSearch("/", "", taskID2, taskID1, map[string]data{
			urlProgramPath(programName2): {
				Name:  programName2,
				Type:  "program",
				Count: 1,
			},
		}, true)
	})

	t.Run("check rule program get rule with diff", func(t *testing.T) {
		// ssarisk://program?type=rule&compare={runtimeId}
		// get rule
		checkRuleAndSearch(urlProgramPath(programName2), "", taskID2, taskID1, map[string]data{
			urlRulePath(programName2, "rule4"): {
				Name:  "rule4",
				Type:  "rule",
				Count: 1,
			},
		})
	})

	t.Run("check rule get source with diff", func(t *testing.T) {
		// ssarisk://program/ruleName?type=rule
		// get source
		checkRuleAndSearch(urlRulePath(programName2, "rule4"), "", taskID2, taskID1, map[string]data{
			urlRuleSourcePath(programName2, "rule4", "d.go"): {
				Name:  sourcePath(programName2, "d.go"),
				Type:  "source",
				Count: 1,
			},
		})
	})

	t.Run("check rule path search source with diff", func(t *testing.T) {
		// ssarisk://program/rule4?type=rule&search=/c.go&compare={runtimeId}
		checkRuleAndSearch(urlRulePath(programName2, "rule4"), "/d.go", taskID2, taskID1, map[string]data{
			urlRuleSourcePath(programName2, "rule4", "d.go"): {
				Name:  sourcePath(programName2, "d.go"),
				Type:  "source",
				Count: 1,
			},
		})
	})
}

func TestRiskActionCompare(t *testing.T) {
	client, err := yakgrpc.NewLocalClient()
	require.NoError(t, err)

	programName := "compare_test_" + uuid.NewString()
	var taskID1, taskID2 string

	testCode1 := `
package main

func test1() {
	sink1()
	sink2()
}
`

	testCode2 := `
package main

func test1() {
	sink1()
	sink2()
}

func test2() {
	sink3()
	sink4()
	sink5()
}
`

	risk1 := uuid.NewString()
	risk2 := uuid.NewString()
	risk3 := uuid.NewString()
	risk4 := uuid.NewString()
	risk5 := uuid.NewString()

	testRule := fmt.Sprintf(`
sink1 as $sink1
alert $sink1 for {
	desc: "Source-Sink vulnerability"
	Title: "Test Risk 1"
	level: "high"
	risk: "%s"
}

sink2 as $sink2
alert $sink2 for {
	desc: "Source-Sink vulnerability"
	Title: "Test Risk 2"
	level: "high"
	risk: "%s"
}

sink3 as $sink3
alert $sink3 for {
	desc: "Source-Sink vulnerability"
	Title: "Test Risk 3"
	level: "high"
	risk: "%s"
}

sink4 as $sink4
alert $sink4 for {
	desc: "Source-Sink vulnerability"
	Title: "Test Risk 4"
	level: "high"
	risk: "%s"
}

sink5 as $sink5
alert $sink5 for {
	desc: "Source-Sink vulnerability"
	Title: "Test Risk 5"
	level: "high"
	risk: "%s"
}
	`, risk1, risk2, risk3, risk4, risk5)

	// 清理测试数据
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName)
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{programName},
		})
	}()

	// 第一次扫描 - 基线扫描（2个风险）
	t.Run("BaselineScan", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("test.go", testCode1)

		programs, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(consts.GO), ssaapi.WithProgramName(programName))
		require.NoError(t, err)
		require.NotEmpty(t, programs)

		stream, err := client.SyntaxFlowScan(context.Background())
		require.NoError(t, err)

		err = stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "start",
			ProgramName: []string{programName},
			RuleInput: &ypb.SyntaxFlowRuleInput{
				Content:  testRule,
				Language: "go",
			},
		})
		require.NoError(t, err)

		// 等待扫描完成
		for {
			resp, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
			}
			if resp.GetStatus() == "finished" || resp.GetStatus() == "error" {
				break
			}
		}

		// 查询第一次扫描的结果
		_, firstRisks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{programName},
		}, nil)
		require.NoError(t, err)
		require.Len(t, firstRisks, 2) // 第一次扫描应该有2个风险
		taskID1 = firstRisks[0].RuntimeId
		t.Logf("第一次扫描TaskID: %s, 风险数量: %d", taskID1, len(firstRisks))
		for i, risk := range firstRisks {
			t.Logf("  Risk%d: ID=%d, Title=%s, RiskFeatureHash=%s", i+1, risk.ID, risk.Title, risk.RiskFeatureHash)
		}
	})

	// 添加延迟确保第二次扫描的时间戳不同
	time.Sleep(2 * time.Second)

	// 第二次扫描 - 对比扫描（5个风险：2个老的+3个新的）
	t.Run("CompareScan", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("test.go", testCode2)

		programs, err := ssaapi.ParseProjectWithFS(
			vf,
			ssaapi.WithLanguage(consts.GO),
			ssaapi.WithProgramName(programName),
			ssaapi.WithReCompile(true),
		)
		require.NoError(t, err)
		require.NotEmpty(t, programs)

		stream, err := client.SyntaxFlowScan(context.Background())
		require.NoError(t, err)

		err = stream.Send(&ypb.SyntaxFlowScanRequest{
			ControlMode: "start",
			ProgramName: []string{programName},
			RuleInput: &ypb.SyntaxFlowRuleInput{
				Content:  testRule,
				Language: "go",
			},
		})
		require.NoError(t, err)

		// 等待扫描完成
		for {
			resp, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
			}
			if resp.GetStatus() == "finished" || resp.GetStatus() == "error" {
				break
			}
		}

		// 查询该程序的所有风险，找到最新的taskID
		_, allRisks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{programName},
		}, nil)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(allRisks), 5) // 总共应该至少有5个风险

		// 找到不同于第一次扫描的taskID（即第二次扫描的taskID）
		for _, risk := range allRisks {
			if risk.RuntimeId != taskID1 {
				taskID2 = risk.RuntimeId
				break
			}
		}
		require.NotEmpty(t, taskID2, "第二次扫描TaskID不能为空")

		// 查询第二次扫描的所有风险
		_, secondRisks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
			RuntimeID: []string{taskID2},
		}, nil)
		require.NoError(t, err)
		require.Len(t, secondRisks, 5) // 第二次扫描应该有5个风险

		t.Logf("第二次扫描TaskID: %s, 风险数量: %d", taskID2, len(secondRisks))
		for i, risk := range secondRisks {
			t.Logf("  Risk%d: ID=%d, Title=%s, RiskFeatureHash=%s", i+1, risk.ID, risk.Title, risk.RiskFeatureHash)
		}
	})

	checkRuleAndSearch_WithDiff := func(t *testing.T, path, search, base, compare string, want map[string]data, contain ...bool) {
		url := &ypb.YakURL{
			Schema: "ssarisk",
			Path:   path,
			Query: []*ypb.KVPair{
				{Key: "task_id", Value: base},
				{Key: "search", Value: search},
				{Key: "compare", Value: compare},
			},
		}
		got := GetSSARisk(t, client, url)
		log.Infof("got: %v", got)
		log.Infof("want: %v", want)
		if len(contain) > 0 && contain[0] {
			// Check if got contains all entries from want
			for wantPath, wantData := range want {
				gotData, exists := got[wantPath]
				require.True(t, exists, "Path %s not found in results", wantPath)
				require.Equal(t, wantData.Name, gotData.Name, "Name mismatch for path %s", wantPath)
				require.Equal(t, wantData.Type, gotData.Type, "Type mismatch for path %s", wantPath)
				require.Equal(t, wantData.Count, gotData.Count, "Count mismatch for path %s", wantPath)
			}
		} else {
			require.Equal(t, want, got)
		}
	}

	// 测试修改后的Compare功能（使用RiskFeatureHash对比）
	t.Run("test RiskFeatureHash based compare", func(t *testing.T) {
		require.NotEmpty(t, taskID1, "第一次扫描TaskID不能为空")
		require.NotEmpty(t, taskID2, "第二次扫描TaskID不能为空")

		// 直接使用yakit进行compare查询，验证RiskFeatureHash对比逻辑
		_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
			SSARiskDiffRequest: &ypb.SSARiskDiffRequest{
				BaseLine: &ypb.SSARiskDiffItem{RiskRuntimeId: taskID2},
				Compare:  &ypb.SSARiskDiffItem{RiskRuntimeId: taskID1},
			},
		}, nil)
		require.NoError(t, err)
		require.Len(t, risks, 3, "Compare查询应该只返回新增的3个风险")

		t.Logf("Compare查询结果：发现 %d 个新增风险", len(risks))
		for i, risk := range risks {
			t.Logf("  新增Risk%d: ID=%d, Title=%s, RiskFeatureHash=%s",
				i+1, risk.ID, risk.Title, risk.RiskFeatureHash)
		}

		// 验证新增风险的标题应该是Test Risk 3, 4, 5
		titles := make([]string, len(risks))
		for i, risk := range risks {
			titles[i] = risk.Title
		}
		require.Contains(t, titles, "Test Risk 3", "应该包含Test Risk 3")
		require.Contains(t, titles, "Test Risk 4", "应该包含Test Risk 4")
		require.Contains(t, titles, "Test Risk 5", "应该包含Test Risk 5")
	})

	// 测试URL方式的compare查询
	t.Run("test URL based compare", func(t *testing.T) {
		// ssarisk://program?task_id={compareTaskID}&compare={baselineTaskID}
		checkRuleAndSearch_WithDiff(t, urlProgramPath(programName), "", taskID2, taskID1, map[string]data{
			urlPath(programName, "test.go"): {
				Name:  sourcePath(programName, "test.go"),
				Type:  "source",
				Count: 3,
			},
		}, false)
	})
}

func TestRiskActionIncremental(t *testing.T) {
	/*
		测试增量查询功能需求说明 - 基于处置状态的跨批次风险聚合

		========================================================================
		核心需求：
		1. 查询A的增量扫描：返回A中所有未处置的风险
		2. 查询B的增量扫描：返回和B产生的risk中相同RiskFeatureHash且批次≤B的所有未处置风险
		3. 查询C的增量扫描：返回和C产生的risk中相同RiskFeatureHash且批次≤C的所有未处置风险

		重要：本次或以前被处置过的漏洞不该查出来
		========================================================================

		测试场景设计 - 三次扫描ABC，批次递增：

		扫描A (Batch 1):
		├── 代码: sink1(), sink2()
		├── 产生风险: Test Risk 1, Test Risk 2
		├── RiskFeatureHash: hash1, hash2
		└── 处置: Risk1(hash1) -> 已处置

		扫描B (Batch 2):
		├── 代码: sink1(), sink2(), sink3(), sink4()
		├── 产生风险: Test Risk 1, Test Risk 2, Test Risk 3, Test Risk 4
		├── RiskFeatureHash: hash1, hash2, hash3, hash4
		└── 处置: Risk3(hash3) -> 已处置

		扫描C (Batch 3):
		├── 代码: sink1(), sink2(), sink3(), sink4(), sink5(), sink6()
		├── 产生风险: Test Risk 1, Test Risk 2, Test Risk 3, Test Risk 4, Test Risk 5, Test Risk 6
		├── RiskFeatureHash: hash1, hash2, hash3, hash4, hash5, hash6
		└── 无新处置

		增量查询期望结果：

		查询A增量 (基于A的RiskFeatureHash: [hash1, hash2]):
		├── 查找批次≤1中hash1, hash2的未处置风险
		├── hash1已处置 -> 过滤
		├── hash2未处置 -> 保留
		└── 返回: [Test Risk 2] (1个)

		查询B增量 (基于B的RiskFeatureHash: [hash1, hash2, hash3, hash4]):
		├── 查找批次≤2中hash1, hash2, hash3, hash4的未处置风险
		├── hash1已处置(Batch1) -> 过滤
		├── hash2未处置 -> 保留所有批次≤2的Risk2实例
		├── hash3已处置(Batch2) -> 过滤
		├── hash4未处置 -> 保留所有批次≤2的Risk4实例
		└── 返回: [Test Risk 2(Batch1), Test Risk 2(Batch2), Test Risk 4(Batch2)] (3个)

		查询C增量 (基于C的RiskFeatureHash: [hash1, hash2, hash3, hash4, hash5, hash6]):
		├── 查找批次≤3中hash1, hash2, hash3, hash4, hash5, hash6的未处置风险
		├── hash1已处置(Batch1) -> 过滤
		├── hash2未处置 -> 保留所有批次≤3的Risk2实例
		├── hash3已处置(Batch2) -> 过滤
		├── hash4未处置 -> 保留所有批次≤3的Risk4实例
		├── hash5未处置 -> 保留所有批次≤3的Risk5实例
		├── hash6未处置 -> 保留所有批次≤3的Risk6实例
		└── 返回: [Test Risk 2(Batch1), Test Risk 2(Batch2), Test Risk 2(Batch3),
		          Test Risk 4(Batch2), Test Risk 4(Batch3),
		          Test Risk 5(Batch3), Test Risk 6(Batch3)] (7个)
	*/

	programName := "incremental_test_" + uuid.NewString()

	// 定义测试风险
	risk1 := ssatest.NewSFScanTestRisk(uuid.NewString(), "Test Risk 1", "sink1")
	risk2 := ssatest.NewSFScanTestRisk(uuid.NewString(), "Test Risk 2", "sink2")
	risk3 := ssatest.NewSFScanTestRisk(uuid.NewString(), "Test Risk 3", "sink3")
	risk4 := ssatest.NewSFScanTestRisk(uuid.NewString(), "Test Risk 4", "sink4")
	risk5 := ssatest.NewSFScanTestRisk(uuid.NewString(), "Test Risk 5", "sink5")
	risk6 := ssatest.NewSFScanTestRisk(uuid.NewString(), "Test Risk 6", "sink6")

	// 第一次扫描：2个sink
	testCode1 := `
package main

func test1() {
	sink1()
	sink2()
}
`

	// 第二次扫描：4个sink
	testCode2 := `
package main

func test1() {
	sink1()
	sink2()
	sink3()
	sink4()
}
`

	// 第三次扫描：6个sink
	testCode3 := `
package main

func test1() {
	sink1()
	sink2()
	sink3()
	sink4()
	sink5()
	sink6()
}
`

	suite, cleanup := ssatest.NewSFScanRiskTestSuite(t, programName, consts.GO)
	defer cleanup()

	// 执行所有扫描操作（在主测试函数中，确保所有子测试都能访问到结果）

	// 第一次扫描
	suite.InitProgram(testCode1, risk1, risk2, risk3, risk4, risk5, risk6).
		Scan().
		CheckRiskCount(2, 0).                                              // 检查第一次扫描产生2个风险
		CheckRiskTitlesContain([]string{"Test Risk 1", "Test Risk 2"}, 0). // 检查第一次扫描的风险标题
		Disposal("Test Risk 1", "is_issue", "已确认为问题")                      // 处置第一个风险

	// 添加延迟确保时间戳不同
	time.Sleep(2 * time.Second)

	// 第二次扫描
	suite.InitProgram(testCode2, risk1, risk2, risk3, risk4, risk5, risk6).
		Scan().
		CheckRiskCount(4, 1).                                                                            // 检查第二次扫描产生4个风险
		CheckRiskTitlesContain([]string{"Test Risk 1", "Test Risk 2", "Test Risk 3", "Test Risk 4"}, 1). // 检查第二次扫描的风险标题
		Disposal("Test Risk 3", "not_issue", "误报")                                                       // 处置第三个风险

	// 添加延迟确保时间戳不同
	time.Sleep(2 * time.Second)

	// 第三次扫描
	suite.InitProgram(testCode3, risk1, risk2, risk3, risk4, risk5, risk6).
		Scan().
		CheckRiskCount(6, 2).                                                                                                         // 检查第三次扫描产生6个风险
		CheckRiskTitlesContain([]string{"Test Risk 1", "Test Risk 2", "Test Risk 3", "Test Risk 4", "Test Risk 5", "Test Risk 6"}, 2) // 检查第三次扫描的风险标题

	// 子测试：验证各种查询功能
	t.Run("FirstScanValidation", func(t *testing.T) {
		// 验证第一次扫描的结果
		suite.CheckRiskCount(2, 0)
		suite.CheckRiskTitlesContain([]string{"Test Risk 1", "Test Risk 2"}, 0)
	})

	t.Run("SecondScanValidation", func(t *testing.T) {
		// 验证第二次扫描的结果
		suite.CheckRiskCount(4, 1)
		suite.CheckRiskTitlesContain([]string{"Test Risk 1", "Test Risk 2", "Test Risk 3", "Test Risk 4"}, 1)
	})

	t.Run("ThirdScanValidation", func(t *testing.T) {
		// 验证第三次扫描的结果
		suite.CheckRiskCount(6, 2)
		suite.CheckRiskTitlesContain([]string{"Test Risk 1", "Test Risk 2", "Test Risk 3", "Test Risk 4", "Test Risk 5", "Test Risk 6"}, 2)
	})

	t.Run("IncrementalQueries", func(t *testing.T) {
		// 测试第一次扫描的增量查询 - 期望：1个风险 (Test Risk 2)
		suite.CheckIncrementalResult(0, 1)
		// 验证增量查询返回的风险标题是否正确
		suite.CheckRiskTitlesContain([]string{"Test Risk 2"})

		// 测试第二次扫描的增量查询 - 期望：2个风险 (Test Risk 2 + Test Risk 4)
		suite.CheckIncrementalResult(1, 2)
		// 验证增量查询返回的风险标题是否正确
		suite.CheckRiskTitlesContain([]string{"Test Risk 2", "Test Risk 4"})

		// 测试第三次扫描的增量查询 - 期望：4个风险 (Test Risk 2 + Test Risk 4 + Test Risk 5 + Test Risk 6)
		suite.CheckIncrementalResult(2, 4)
		// 验证增量查询返回的风险标题是否正确
		suite.CheckRiskTitlesContain([]string{"Test Risk 2", "Test Risk 4", "Test Risk 5", "Test Risk 6"})
	})

	t.Run("URLBasedIncrementalQueries", func(t *testing.T) {
		// 测试第一次扫描通过URL进行增量查询
		url := &ypb.YakURL{
			Schema: "ssarisk",
			Path:   urlProgramPath(programName),
			Query: []*ypb.KVPair{
				{Key: "task_id", Value: suite.TaskIDs[0]},
				{Key: "increment", Value: "true"},
			},
		}
		got := GetSSARisk(t, suite.Client, url)
		totalCount := 0
		for _, item := range got {
			totalCount += item.Count
		}
		require.Equal(t, 1, totalCount, "第一次扫描增量查询应该返回1个风险")

		// 测试第二次扫描通过URL进行增量查询
		url.Query[0].Value = suite.TaskIDs[1]
		got = GetSSARisk(t, suite.Client, url)
		totalCount = 0
		for _, item := range got {
			totalCount += item.Count
		}
		require.Equal(t, 2, totalCount, "第二次扫描增量查询应该返回2个风险")

		// 测试第三次扫描通过URL进行增量查询
		url.Query[0].Value = suite.TaskIDs[2]
		got = GetSSARisk(t, suite.Client, url)
		totalCount = 0
		for _, item := range got {
			totalCount += item.Count
		}
		require.Equal(t, 4, totalCount, "第三次扫描增量查询应该返回4个风险")
	})
}
