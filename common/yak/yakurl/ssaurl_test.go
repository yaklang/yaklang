package yakurl_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yak/yakurl"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func SendURL(local ypb.YakClient, program, path string, body string) ([]*ypb.YakURLResource, error) {
	url := &ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema:   "syntaxflow",
			Location: program,
			Path:     path,
			Query:    []*ypb.KVPair{},
		},
		Body: []byte(body),
	}
	spew.Dump(url)
	res, err := local.RequestYakURL(context.Background(), url)
	if err != nil {
		return nil, err
	}
	return res.Resources, nil
}

func CheckSSAURL(t *testing.T, local ypb.YakClient, programName, path, sfCode string, checkHandler func([]*ypb.YakURLResource)) {
	var resultID string
	{
		// send memory query
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema:   "syntaxflow",
				Location: programName,
				Path:     path,
			},
			Body: []byte(sfCode),
		}

		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		t.Log("checkHandler in memory query ")
		spew.Dump(res)

		resultIDRes := res.Resources[len(res.Resources)-1]
		require.Equal(t, resultIDRes.ResourceType, "result_id")
		require.Equal(t, resultIDRes.VerboseType, "result_id")
		// got result
		resultID = resultIDRes.ResourceName

		checkHandler(res.Resources[:len(res.Resources)-1])
	}

	{

		// send memory query
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema:   "syntaxflow",
				Location: programName,
				Path:     path,
				Query: []*ypb.KVPair{
					{
						// get from database
						Key:   "result_id",
						Value: resultID,
					},
				},
			},
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		t.Log("checkHandler in database query ")
		spew.Dump(res)

		resultIDRes := res.Resources[len(res.Resources)-1]
		require.Equal(t, resultIDRes.ResourceType, "result_id")
		require.Equal(t, resultIDRes.VerboseType, "result_id")
		// got result
		gotResultID := resultIDRes.ResourceName
		require.Equal(t, resultID, gotResultID)

		checkHandler(res.Resources[:len(res.Resources)-1])
	}

	{
		// send query from database
		prog, err := ssaapi.FromDatabase(programName)
		require.NoError(t, err)
		result := prog.SyntaxFlow(sfCode)
		resultID, err := result.Save()
		require.NoError(t, err)
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema:   "syntaxflow",
				Location: programName,
				Path:     path,
				Query: []*ypb.KVPair{
					{
						// get from database
						Key:   "result_id",
						Value: codec.AnyToString(resultID),
					},
				},
			},
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		t.Log("checkHandler in database query ")
		spew.Dump(res)

		resultIDRes := res.Resources[len(res.Resources)-1]
		require.Equal(t, resultIDRes.ResourceType, "result_id")
		require.Equal(t, resultIDRes.VerboseType, "result_id")
		// got result
		gotResultID := resultIDRes.ResourceName
		require.Equal(t, codec.AnyToString(resultID), gotResultID)

		checkHandler(res.Resources[:len(res.Resources)-1])
	}
}

func checkVariable(t *testing.T, res []*ypb.YakURLResource, want []string) {
	got := lo.FilterMap(res, func(r *ypb.YakURLResource, _ int) (string, bool) {
		return r.ResourceName, r.ResourceType == "variable"
	})
	require.Len(t, got, len(want))
	require.Equal(t, want, got)
}

func TestSFURl(t *testing.T) {
	local, err := yakgrpc.NewLocalClient()
	if err != nil {
		t.Error(err)
	}
	_ = local

	ssadb.DeleteProgram(ssadb.GetDB(), "com.example.apackage")
	ssadb.DeleteProgram(ssadb.GetDB(), "com.example.bpackage.sub")

	vf := filesys.NewVirtualFs()
	vf.AddFile("example/src/main/java/com/example/apackage/a.java", `
		package com.example.apackage; 
		import com.example.bpackage.sub.B;
		class A {
			public static void main(String[] args) {
				B b = new B();
				// for test 1: A->B
				target1(b.get());
				// for test 2: B->A
				b.show(1);
			}
		}
		`)

	vf.AddFile("example/src/main/java/com/example/bpackage/sub/b.java", `
		package com.example.bpackage.sub; 
		class B {
			public  int get() {
				return 	 1;
			}
			public void show(int a) {
				target2(a);
			}
		}
		`)
	progID := uuid.NewString()
	prog, err := ssaapi.ParseProject(vf,
		ssaapi.WithLanguage(consts.JAVA),
		ssaapi.WithProgramPath("example"),
		ssaapi.WithProgramName(progID),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	}()
	require.NoError(t, err)
	require.NotNil(t, prog)

	t.Run("check syntaxflow variable", func(t *testing.T) {
		CheckSSAURL(t, local, progID, "/",
			`target2(* #-> as $a)`,
			func(res []*ypb.YakURLResource) {
				checkVariable(t, res, []string{"a", "_"})
			},
		)
	})

	t.Run("check _", func(t *testing.T) {
		CheckSSAURL(t, local, progID, "/", `target*`, func(res []*ypb.YakURLResource) {
			checkVariable(t, res, []string{"_"})
		})
		CheckSSAURL(t, local, progID, "/_", `target*`, func(res []*ypb.YakURLResource) {
			require.Equal(t, 2, len(res))
		})
	})

	t.Run("check syntaxflow variable no data", func(t *testing.T) {
		res, err := SendURL(local, progID, "/", `
		dddd as $a
		`)
		require.NoError(t, err)
		spew.Dump(res)
		checkVariable(t, res, []string{"a"})
	})

	t.Run("check syntaxflow variable with alert", func(t *testing.T) {
		CheckSSAURL(t, local, progID, "/", `
		target2(* #-> as $a) 
		target1() as $target1
		alert $target1 for "alert information"
		`, func(res []*ypb.YakURLResource) {
			spew.Dump(res)
			checkVariable(t, res, []string{"target1", "a", "_"})
			target1 := res[0]
			require.Equal(t, target1.VerboseName, "alert information")
			require.Equal(t, target1.ResourceType, "variable")
			require.Equal(t, target1.VerboseType, "alert")
			matchRisk := false
			for _, extra := range target1.Extra {
				if extra.Key == "risk_hash" && extra.Value != "" {
					matchRisk = true
				}
			}
			require.True(t, matchRisk, "should have risk hash")
		})
	})

	t.Run("check syntaxflow variable with check params", func(t *testing.T) {
		CheckSSAURL(t, local, progID, "/", `
		target2(* #-> as $a) 
		$a?{!(opcode: const)} as $not_const_parameter 
		$a?{(opcode: const)} as $const_parameter

		check $not_const_parameter then "has not-const parameter" else "no not-const parameter"
		check $const_parameter then "has const parameter" else "no const parameter"
		`, func(res []*ypb.YakURLResource) {
			spew.Dump(res)
			checkVariable(t, res, []string{"a", "const_parameter", "not_const_parameter", "_"})

			errMsg := res[0]
			require.Equal(t, errMsg.ResourceType, "message")
			require.Equal(t, errMsg.VerboseType, "error")
			require.Equal(t, errMsg.VerboseName, "no not-const parameter")

			infoMsg := res[1]
			require.Equal(t, infoMsg.ResourceType, "message")
			require.Equal(t, infoMsg.VerboseType, "info")
			require.Equal(t, infoMsg.VerboseName, "has const parameter")
		})
	})

	t.Run("check syntaxflow value", func(t *testing.T) {
		query := fmt.Sprintf(`
		target* as $target 
		$target #{
			hook: %s
		}->
		`, "`*  as $a`")

		CheckSSAURL(t, local, progID, "/", query, func(yu []*ypb.YakURLResource) {
			spew.Dump(yu)
			checkVariable(t, yu, []string{"a", "target", "_"})
		})

		CheckSSAURL(t, local, progID, "/a", query, func(res []*ypb.YakURLResource) {
			spew.Dump(res)
		})
	})

	t.Run("check syntaxflow information", func(t *testing.T) {
		query := fmt.Sprintf(`
		target* as $target 
		$target (* #{
			hook: %s
		}-> as $para_top_def)
		`, "`*  as $a`")
		CheckSSAURL(t, local, progID, "/a/0", query, func(res []*ypb.YakURLResource) {

			require.NoError(t, err)
			spew.Dump(res)
			check := func(path string) {
				log.Infof("check path: %s", path)
				_, err := ssadb.NewIrSourceFs().Stat(path)
				require.NoError(t, err)
			}

			found := false
			var node string
			graphInfoMap := make(map[string]*yakurl.NodeInfo)
			for _, extra := range res[0].Extra {
				if extra.Key == "node_id" {
					log.Infof("graph: %v", extra.Value)
					node = extra.Value
					continue
				}
				if extra.Key == "graph" {
					log.Infof("graph: %v", extra.Value)
					continue
				}

				if extra.Key == "graph_info" {
					log.Infof("graph info: %v", extra.Value)
					var graphInfo []*yakurl.NodeInfo
					if err := json.Unmarshal([]byte(extra.Value), &graphInfo); err != nil {
						t.Error(err)
					}
					for _, info := range graphInfo {
						log.Infof("graph info item: \n%v", info)
						// spew.Dump(info)
						if info.NodeID == node {
							found = true
						}
						graphInfoMap[info.NodeID] = info

						check(info.CodeRange.URL)
					}
				}
				if extra.Key == "graph_line" {
					log.Infof("graph line: %s", extra.Value)
					var res [][]string
					if err := json.Unmarshal([]byte(extra.Value), &res); err != nil {
						t.Error(err)
					}
					require.Greater(t, len(res), 0)
					for _, resItem := range res {
						for _, item := range resItem {
							if _, ok := graphInfoMap[item]; !ok {
								t.Errorf("not found in graph info: %s", item)
							}
						}
					}
				}
			}
			require.True(t, found)
		})
	})

}
