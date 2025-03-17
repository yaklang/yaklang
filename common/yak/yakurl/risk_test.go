package yakurl_test

import (
	"context"
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

/*
	Get SSA Risk
		Request :
			url : {
				schema: "ssarisk"
				path: "${program}/${path}/${function}"
			}
		Response:
			1. path : "${program}" :
				return {
					resource: []Resource{
						name: "${file}"
						Extra: {
							Key: "count"
							Value: "${risk_count}"
						}
					}
				}

				// SELECT program AS programName, COUNT(*) AS Count FROM db GROUP BY program;
			2. path: "${program}/${file}"
				return {
					resource: []Resource{
						name: "${function}"
						Extra: {
							Key: "count"
							Value: "${risk_count}"
						}
					}
				}
*/

func initRiskTest(t *testing.T, programName string) map[string]int {
	db := ssadb.GetDB()

	err := yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:   programName,
		CodeSourceUrl: fmt.Sprintf("/%s/a.go", programName),
		FunctionName:  "funcA",
		Title:         "test1",
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:   programName,
		CodeSourceUrl: fmt.Sprintf("/%s/b/b.go", programName),
		FunctionName:  "funcB1",
		Title:         "test2",
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:   programName,
		CodeSourceUrl: fmt.Sprintf("/%s/b/b.go", programName),
		FunctionName:  "funcB2",
		Title:         "test3",
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:   programName,
		CodeSourceUrl: fmt.Sprintf("/%s/c.go", programName),
		FunctionName:  "funcC",
		Title:         "test4",
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:   programName,
		CodeSourceUrl: fmt.Sprintf("/%s/c.go", programName),
		FunctionName:  "funcC",
		Title:         "test5",
	})
	require.NoError(t, err)

	return map[string]int{
		fmt.Sprintf("/%s/a.go/funcA", programName):    1,
		fmt.Sprintf("/%s/b/b.go/funcB1", programName): 1,
		fmt.Sprintf("/%s/b/b.go/funcB2", programName): 1,
		fmt.Sprintf("/%s/c.go/funcC", programName):    2,
	}
}

func TestRiskAction(t *testing.T) {
	programName := uuid.NewString()
	check := initRiskTest(t, programName)

	local, err := yakgrpc.NewLocalClient()
	if err != nil {
		t.Error(err)
	}
	t.Run("check risk action root", func(t *testing.T) {
		// ssarisk://
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema:   "ssarisk",
				Location: "",
				Path:     "",
			},
			Body: []byte(""),
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		require.Equal(t, len(res.GetResources()), 4)

		for _, p := range res.GetResources() {
			require.Equal(t, p.Extra[0].Value, fmt.Sprint(check[p.Path]))
		}
	})
	t.Run("check risk action program", func(t *testing.T) {
		// ssarisk://program
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema:   "ssarisk",
				Location: programName,
				Path:     "",
			},
			Body: []byte(""),
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		require.Equal(t, len(res.GetResources()), 4)

		for _, p := range res.GetResources() {
			require.Equal(t, p.Extra[0].Value, fmt.Sprint(check[p.Path]))
		}
	})
	t.Run("check risk action path(dir)", func(t *testing.T) {
		// ssarisk://program/b
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema:   "ssarisk",
				Location: programName,
				Path:     "/b",
			},
			Body: []byte(""),
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		require.Equal(t, len(res.GetResources()), 2)

		for _, p := range res.GetResources() {
			require.Equal(t, p.Extra[0].Value, fmt.Sprint(check[p.Path]))
		}
	})
	t.Run("check risk action path(file)", func(t *testing.T) {
		// ssarisk://program/c.go
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema:   "ssarisk",
				Location: programName,
				Path:     "c.go",
			},
			Body: []byte(""),
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		require.Equal(t, len(res.GetResources()), 1)

		for _, p := range res.GetResources() {
			require.Equal(t, p.Extra[0].Value, fmt.Sprint(check[p.Path]))
		}
	})
	t.Run("check risk action function", func(t *testing.T) {
		// ssarisk://program/a.go/funcA
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema:   "ssarisk",
				Location: programName,
				Path:     "a.go/funcA",
				// Query: []*ypb.KVPair{
				// 	{
				// 		Key:   "function_name",
				// 		Value: "funcA",
				// 	},
				// },
			},
			Body: []byte(""),
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		require.Equal(t, len(res.GetResources()), 1)

		for _, p := range res.GetResources() {
			require.Equal(t, p.Extra[0].Value, fmt.Sprint(check[p.Path]))
		}
	})

	t.Run("check risk query", func(t *testing.T) {
		db := ssadb.GetDB()

		risks, err := yakit.GetSSARiskByRawpath(db, fmt.Sprintf("/%s", programName))
		require.NoError(t, err)
		require.Equal(t, len(risks), 5)

		risks, err = yakit.GetSSARiskByRawpath(db, fmt.Sprintf("/%s/b/b.go", programName))
		require.NoError(t, err)
		require.Equal(t, len(risks), 2)

		risks, err = yakit.GetSSARiskByRawpath(db, fmt.Sprintf("/%s/b/b.go/funcB1", programName))
		require.NoError(t, err)
		require.Equal(t, len(risks), 1)
	})

	t.Cleanup(func() {
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{programName}})
	})
}
