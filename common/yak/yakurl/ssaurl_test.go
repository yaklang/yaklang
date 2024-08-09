package yakurl_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
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
		ssaapi.WithDatabaseProgramName(progID),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	}()
	assert.NoError(t, err)
	assert.NotNil(t, prog)

	checkVariable := func(t *testing.T, res []*ypb.YakURLResource, want []string) {
		got := lo.FilterMap(res, func(r *ypb.YakURLResource, _ int) (string, bool) {
			return r.ResourceName, r.ResourceType == "variable"
		})
		assert.Len(t, got, len(want))
		assert.Equal(t, got, want)
	}

	t.Run("check syntaxflow variable", func(t *testing.T) {
		res, err := SendURL(local, progID, "/", `
		target2(* #-> as $a)
		`)
		assert.NoError(t, err)
		spew.Dump(res)
		checkVariable(t, res, []string{"a", "_"})
	})

	t.Run("check syntaxflow variable with alert", func(t *testing.T) {
		res, err := SendURL(local, progID, "/", `
		target2(* #-> as $a) 
		target1() as $target1
		alert $target1 for "alert information"
		`)
		assert.NoError(t, err)
		spew.Dump(res)
		checkVariable(t, res, []string{"target1", "a", "_"})
		target1 := res[0]
		assert.Equal(t, target1.VerboseName, "alert information")
		assert.Equal(t, target1.ResourceType, "variable")
		assert.Equal(t, target1.VerboseType, "alert")
	})

	t.Run("check syntaxflow variable with check params", func(t *testing.T) {
		res, err := SendURL(local, progID, "/", `
		target2(* #-> as $a) 
		$a?{!(opcode: const)} as $not_const_parameter 
		$a?{(opcode: const)} as $const_parameter

		check $not_const_parameter then "has not-const parameter" else "no not-const parameter"
		check $const_parameter then "has const parameter" else "no const parameter"
		`)
		assert.NoError(t, err)
		spew.Dump(res)
		checkVariable(t, res, []string{"a", "not_const_parameter", "const_parameter", "_"})

		errMsg := res[0]
		assert.Equal(t, errMsg.ResourceType, "message")
		assert.Equal(t, errMsg.VerboseType, "error")
		assert.Equal(t, errMsg.VerboseName, "no not-const parameter")

		infoMsg := res[1]
		assert.Equal(t, infoMsg.ResourceType, "message")
		assert.Equal(t, infoMsg.VerboseType, "info")
		assert.Equal(t, infoMsg.VerboseName, "has const parameter")
	})

	t.Run("check syntaxflow value", func(t *testing.T) {
		query := fmt.Sprintf(`
		target* as $target 
		$target #{
			hook: %s
		}->
		`, "`*  as $a`")

		{

			res, err := SendURL(local, progID, "/", query)
			assert.NoError(t, err)
			spew.Dump(res)
			checkVariable(t, res, []string{"target", "a", "_"})
		}

		{
			res, err := SendURL(local, progID, "/a", query)
			assert.NoError(t, err)
			spew.Dump(res)
		}
	})

	t.Run("check syntaxflow information", func(t *testing.T) {
		query := fmt.Sprintf(`
		target* as $target 
		$target (* #{
			hook: %s
		}-> as $para_top_def)
		`, "`*  as $a`")
		{
			res, err := SendURL(local, progID, "/a/0", query)
			assert.NoError(t, err)
			spew.Dump(res)
			check := func(path string) {
				log.Infof("check path: %s", path)
				_, err := ssadb.NewIrSourceFs().Stat(path)
				assert.NoError(t, err)
			}

			found := false
			var node string
			for _, extra := range res[0].Extra {
				if extra.Key == "node_id" {
					node = extra.Value
					continue
				}

				if extra.Key != "graph_info" {
					continue
				}
				var graphInfo []*yakurl.NodeInfo
				if err := json.Unmarshal([]byte(extra.Value), &graphInfo); err != nil {
					t.Error(err)
				}

				for _, info := range graphInfo {
					if info.NodeID == node {
						found = true
					}

					check(info.CodeRange.URL)
				}
			}
			assert.True(t, found)
		}
	})
}
