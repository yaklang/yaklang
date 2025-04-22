package yakurl_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func initRiskTest(t *testing.T, programName string) {
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
	*/

	err := yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:   programName,
		CodeSourceUrl: fmt.Sprintf("/%s/a.go", programName),
		FunctionName:  "funcA",
		Title:         "test1",
		ResultID:      1,
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:   programName,
		CodeSourceUrl: fmt.Sprintf("/%s/b/b1.go", programName),
		FunctionName:  "funcB1",
		Title:         "test2",
		ResultID:      2,
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:   programName,
		CodeSourceUrl: fmt.Sprintf("/%s/b/b2.go", programName),
		FunctionName:  "funcB2",
		Title:         "test3",
		ResultID:      3,
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:   programName,
		CodeSourceUrl: fmt.Sprintf("/%s/c.go", programName),
		FunctionName:  "funcC",
		Title:         "test4",
		ResultID:      4,
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:   programName,
		CodeSourceUrl: fmt.Sprintf("/%s/c.go", programName),
		FunctionName:  "funcC",
		Title:         "test5",
		ResultID:      5,
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
		var count, filterCount int
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

func TestRiskAction(t *testing.T) {
	programName1 := uuid.NewString()
	initRiskTest(t, programName1)
	programName2 := uuid.NewString()
	initRiskTest(t, programName2)

	urlProgramPath := func(progName string) string {
		return "/" + progName
	}

	sourcePath := func(progName, source string) string {
		return path.Join("/", progName, source)
	}

	urlPath := func(progName, source string) string {
		return path.Join(urlProgramPath(progName), sourcePath(progName, source))
	}

	urlFunctionPath := func(progName, source, function string) string {
		return path.Join(urlPath(progName, source), function)
	}

	t.Cleanup(func() {
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{programName1}})
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{programName2}})
	})
	local, err := yakgrpc.NewLocalClient()
	if err != nil {
		t.Error(err)
	}

	checkPathAndSearch := func(path, search string, want map[string]data, contain ...bool) {
		url := &ypb.YakURL{
			Schema: "ssarisk",
			Path:   path,
			Query:  []*ypb.KVPair{{Key: "search", Value: search}},
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
		})
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
