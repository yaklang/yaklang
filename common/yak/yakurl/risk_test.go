package yakurl_test

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestRiskAction(t *testing.T) {
	programName1 := uuid.NewString()
	initRiskTest(t, programName1)
	programName2 := uuid.NewString()
	initRiskTest(t, programName2)

	local, err := yakgrpc.NewLocalClient()
	if err != nil {
		t.Error(err)
	}
	t.Run("check risk action root", func(t *testing.T) {
		// ssarisk://
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema: "ssarisk",
				Path:   "/",
			},
			Body: []byte(""),
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		require.Equal(t, len(res.GetResources()), 2)
		check := res.GetResources()
		_ = check

		require.Equal(t, check[0].Extra[0].Value, "5")
		require.Equal(t, check[1].Extra[0].Value, "5")

		require.Equal(t, check[0].ResourceType, "program")
	})
	t.Run("check risk action program", func(t *testing.T) {
		// ssarisk://program
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema: "ssarisk",
				Path:   "/" + programName1,
			},
			Body: []byte(""),
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		require.Equal(t, len(res.GetResources()), 4)
		check := res.GetResources()
		_ = check

		require.Equal(t, check[0].Extra[0].Value, "1")
		require.Equal(t, check[1].Extra[0].Value, "1")
		require.Equal(t, check[2].Extra[0].Value, "1")
		require.Equal(t, check[3].Extra[0].Value, "2")

		require.Equal(t, check[0].ResourceName, "/a.go")
		require.Equal(t, check[0].ResourceType, "source")

		require.Equal(t, check[0].Path, fmt.Sprintf("/%s/a.go", programName1))
		require.Equal(t, check[1].Path, fmt.Sprintf("/%s/b/b1.go", programName1))
		require.Equal(t, check[2].Path, fmt.Sprintf("/%s/b/b2.go", programName1))
		require.Equal(t, check[3].Path, fmt.Sprintf("/%s/c.go", programName1))

		filter := &ypb.SSARisksFilter{}
		err = json.Unmarshal([]byte(check[0].Extra[1].Value), &filter)
		require.NoError(t, err)
		_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), filter, nil)
		require.NoError(t, err)
		require.Equal(t, len(risks), 1)

		err = json.Unmarshal([]byte(check[1].Extra[1].Value), &filter)
		require.NoError(t, err)
		_, risks, err = yakit.QuerySSARisk(ssadb.GetDB(), filter, nil)
		require.NoError(t, err)
		require.Equal(t, len(risks), 1)

		err = json.Unmarshal([]byte(check[2].Extra[1].Value), &filter)
		require.NoError(t, err)
		_, risks, err = yakit.QuerySSARisk(ssadb.GetDB(), filter, nil)
		require.NoError(t, err)
		require.Equal(t, len(risks), 1)

		err = json.Unmarshal([]byte(check[3].Extra[1].Value), &filter)
		require.NoError(t, err)
		_, risks, err = yakit.QuerySSARisk(ssadb.GetDB(), filter, nil)
		require.NoError(t, err)
		require.Equal(t, len(risks), 2)
	})
	t.Run("check risk action path(dir)", func(t *testing.T) {
		// 已弃用：不会出现这种情况
		t.Skip()
		// ssarisk://program/b
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema: "ssarisk",
				Path:   "/" + programName1 + "/b",
			},
			Body: []byte(""),
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		require.Equal(t, len(res.GetResources()), 2)
		check := res.GetResources()
		_ = check

		require.Equal(t, check[0].Extra[0].Value, "1")
		require.Equal(t, check[1].Extra[0].Value, "1")

		// require.Equal(t, check[0].Path, fmt.Sprintf("/%s/b/b1.go/funcB1", programName1))
		// require.Equal(t, check[1].Path, fmt.Sprintf("/%s/b/b2.go/funcB2", programName1))

		filter := &ypb.SSARisksFilter{}
		err = json.Unmarshal([]byte(check[0].Extra[1].Value), &filter)
		require.NoError(t, err)
		_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), filter, nil)
		require.NoError(t, err)
		require.Equal(t, len(risks), 1)

		err = json.Unmarshal([]byte(check[1].Extra[1].Value), &filter)
		require.NoError(t, err)
		_, risks, err = yakit.QuerySSARisk(ssadb.GetDB(), filter, nil)
		require.NoError(t, err)
		require.Equal(t, len(risks), 1)
	})
	t.Run("check risk action path(file)", func(t *testing.T) {
		// ssarisk://program/c.go
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema: "ssarisk",
				Path:   "/" + programName1 + "/c.go",
			},
			Body: []byte(""),
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		require.Equal(t, len(res.GetResources()), 1)
		check := res.GetResources()
		_ = check

		require.Equal(t, check[0].Extra[0].Value, "2")
		require.Equal(t, check[0].Path, fmt.Sprintf("/%s/c.go/funcC", programName1))
		require.Equal(t, check[0].ResourceName, "funcC")
		require.Equal(t, check[0].ResourceType, "function")

		filter := &ypb.SSARisksFilter{}
		err = json.Unmarshal([]byte(check[0].Extra[1].Value), &filter)
		require.NoError(t, err)
		_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), filter, nil)
		require.NoError(t, err)
		require.Equal(t, len(risks), 2)
	})
	t.Run("check risk action function", func(t *testing.T) {
		// ssarisk://program/c.go/funcC
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema: "ssarisk",
				Path:   "/" + programName1 + "/c.go/funcC",
			},
			Body: []byte(""),
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		require.Equal(t, len(res.GetResources()), 1)
		check := res.GetResources()
		_ = check

		require.Equal(t, check[0].Extra[0].Value, "2")

		filter := &ypb.SSARisksFilter{}
		err = json.Unmarshal([]byte(check[0].Extra[1].Value), &filter)
		require.NoError(t, err)
		_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), filter, nil)
		require.NoError(t, err)
		require.Equal(t, len(risks), 2)
	})

	t.Run("check risk action search path(file)", func(t *testing.T) {
		// search=/c.go
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema: "ssarisk",
				Query:  []*ypb.KVPair{{Key: "search", Value: "/c.go"}},
			},
			Body: []byte(""),
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		require.Equal(t, len(res.GetResources()), 2)
		check := res.GetResources()
		_ = check

		require.Equal(t, check[0].Extra[0].Value, "2")
		require.Equal(t, check[0].ResourceName, "/c.go")
		require.Equal(t, check[0].ResourceType, "source")
	})

	t.Run("check risk action search function", func(t *testing.T) {
		// search=funcA
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema: "ssarisk",
				Query:  []*ypb.KVPair{{Key: "search", Value: "funcA"}},
			},
			Body: []byte(""),
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		require.Equal(t, len(res.GetResources()), 2)
		check := res.GetResources()
		_ = check

		require.Equal(t, check[0].Extra[0].Value, "1")
		require.Equal(t, check[0].ResourceName, "funcA")
		require.Equal(t, check[0].ResourceType, "function")
	})

	t.Run("check risk action search function fuzzy", func(t *testing.T) {
		// search=func
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema: "ssarisk",
				Query:  []*ypb.KVPair{{Key: "search", Value: "func"}},
			},
			Body: []byte(""),
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		require.Equal(t, len(res.GetResources()), 8)
	})

	t.Cleanup(func() {
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{programName1}})
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{programName2}})
	})
}
