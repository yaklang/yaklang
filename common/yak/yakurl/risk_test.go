package yakurl_test

import (
	"context"
	"testing"

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

func initRiskTest(t *testing.T, programName string) {
	db := ssadb.GetDB()

	err := yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:   programName,
		CodeSourceUrl: "/program/a.go",
		FunctionName:  "funcA",
		Title:         "1",
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:   programName,
		CodeSourceUrl: "/program/b/b.go",
		FunctionName:  "funcB1",
		Title:         "2",
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:   programName,
		CodeSourceUrl: "/program/b/b.go",
		FunctionName:  "funcB2",
		Title:         "3",
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:   programName,
		CodeSourceUrl: "/program/c.go",
		FunctionName:  "funcC",
		Title:         "4",
	})
	require.NoError(t, err)

	err = yakit.CreateSSARisk(db, &schema.SSARisk{
		ProgramName:   programName,
		CodeSourceUrl: "/program/c.go",
		FunctionName:  "funcC",
		Title:         "5",
	})
	require.NoError(t, err)
}

func TestRisk(t *testing.T) {
	programName := "program"

	initRiskTest(t, programName)

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
			if p.Path == "/program/c.go/funcC" {
				require.Equal(t, p.Extra[0].Value, "2")
			} else if p.Path == "/program/b/b.go/funcB2" {
				require.Equal(t, p.Extra[0].Value, "1")
			} else if p.Path == "/program/b/b.go/funcB1" {
				require.Equal(t, p.Extra[0].Value, "1")
			} else if p.Path == "/program/a.go/funcA" {
				require.Equal(t, p.Extra[0].Value, "1")
			}
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
			if p.Path == "/program/c.go/funcC" {
				require.Equal(t, p.Extra[0].Value, "2")
			} else if p.Path == "/program/b/b.go/funcB2" {
				require.Equal(t, p.Extra[0].Value, "1")
			} else if p.Path == "/program/b/b.go/funcB1" {
				require.Equal(t, p.Extra[0].Value, "1")
			} else if p.Path == "/program/a.go/funcA" {
				require.Equal(t, p.Extra[0].Value, "1")
			}
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
			if p.Path == "/program/b/b.go/funcB2" {
				require.Equal(t, p.Extra[0].Value, "1")
			} else if p.Path == "/program/b/b.go/funcB1" {
				require.Equal(t, p.Extra[0].Value, "1")
			}
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
			if p.Path == "/program/c.go/funcC" {
				require.Equal(t, p.Extra[0].Value, "2")
			}
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
			if p.Path == "/program/a.go/funcA" {
				require.Equal(t, p.Extra[0].Value, "1")
			}
		}
	})

	t.Cleanup(func() {
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{ProgramName: []string{programName}})
	})
}
