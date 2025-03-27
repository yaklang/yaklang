package yakurl_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
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
		for _, extra := range resource.Extra {
			if extra.Key == "count" {
				count, err = strconv.Atoi(extra.Value)
				require.NoError(t, err)
			}
			if extra.Key == "filter" {
				var filter *ypb.SSARisksFilter
				err = json.Unmarshal([]byte(extra.Value), &filter)
				require.NoError(t, err)
				_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), filter, nil)
				require.NoError(t, err)
				filterCount = len(risks)
			}
		}
		require.Equal(t, count, filterCount)
		got[resource.ResourceName] = data{
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

	t.Cleanup(func() {
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{programName1}})
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{programName2}})
	})
	local, err := yakgrpc.NewLocalClient()
	if err != nil {
		t.Error(err)
	}

	checkPath := func(path string, want map[string]data) {
		url := &ypb.YakURL{
			Schema: "ssarisk",
			Path:   path,
		}
		got := GetSSARisk(t, local, url)
		require.Equal(t, got, want)
	}

	checkSearch := func(search string, want map[string]data) {
		url := &ypb.YakURL{
			Schema: "ssarisk",
			Query:  []*ypb.KVPair{{Key: "search", Value: search}},
		}
		got := GetSSARisk(t, local, url)
		require.Equal(t, got, want)
	}

	t.Run("check risk action root", func(t *testing.T) {
		checkPath("/", map[string]data{
			programName1: {
				Name:  programName1,
				Type:  "program",
				Count: 5,
			},
			programName2: {
				Name:  programName2,
				Type:  "program",
				Count: 5,
			},
		})
	})

	t.Run("check risk action program", func(t *testing.T) {
		// ssarisk://program

		checkPath("/"+programName1, map[string]data{
			"/a.go": {
				Name:  "/a.go",
				Type:  "source",
				Count: 1,
			},
			"/b/b1.go": {
				Name:  "/b/b1.go",
				Type:  "source",
				Count: 1,
			},
			"/b/b2.go": {
				Name:  "/b/b2.go",
				Type:  "source",
				Count: 1,
			},
			"/c.go": {
				Name:  "/c.go",
				Type:  "source",
				Count: 2,
			},
		})
	})

	t.Run("check risk action path(file)", func(t *testing.T) {
		// ssarisk://program/c.go
		checkPath("/"+programName1+"/c.go", map[string]data{
			"funcC": {
				Name:  "funcC",
				Type:  "function",
				Count: 2,
			},
		})
	})

	t.Run("check risk action search path(file)", func(t *testing.T) {
		// search=/c.go
		checkSearch("/c.go", map[string]data{
			"/c.go": {
				Name:  "/c.go",
				Type:  "source",
				Count: 2,
			},
		})
	})

	t.Run("check risk action search function", func(t *testing.T) {
		// search=funcA
		checkSearch("funcA", map[string]data{
			"funcA": {
				Name:  "funcA",
				Type:  "function",
				Count: 1,
			},
		})
	})

	t.Run("check risk action search function fuzzy", func(t *testing.T) {
		// search=func
		checkSearch("func", map[string]data{
			"funcA": {
				Name:  "funcA",
				Type:  "function",
				Count: 1,
			},
			"funcB1": {
				Name:  "funcB1",
				Type:  "function",
				Count: 1,
			},
			"funcB2": {
				Name:  "funcB2",
				Type:  "function",
				Count: 1,
			},
			"funcC": {
				Name:  "funcC",
				Type:  "function",
				Count: 2,
			},
		})
	})

}
