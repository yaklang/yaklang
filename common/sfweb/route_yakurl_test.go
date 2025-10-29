package sfweb_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/sfweb"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func checkSSAURL(t *testing.T, programName, path, sfCode string, checkHandler func([]*sfweb.YakURLResource)) {
	t.Helper()
	var resultID string
	{
		// send memory query
		// url := &ypb.RequestYakURLParams{
		req := &sfweb.YakURLRequest{
			Method: http.MethodGet,
			URL: &sfweb.YakURL{
				Schema:   "syntaxflow",
				Location: programName,
				Path:     path,
				Query: []*sfweb.Query{
					{
						Key:   "save_result",
						Value: "true",
					},
				},
			},
			Body: sfCode,
		}
		body, err := json.Marshal(req)
		require.NoError(t, err)
		var rsp sfweb.YakURLResponse

		rawRsp, err := DoResponse(http.MethodPost, "/yakurl", &rsp, poc.WithReplaceHttpPacketBody(body, false))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rawRsp.GetStatusCode())

		t.Log("checkHandler in memory query")
		resultIDRes := rsp.Resources[len(rsp.Resources)-1]
		resultID = resultIDRes.ResourceName
		require.Equal(t, "result_id", resultIDRes.ResourceType)
		require.Equal(t, "result_id", resultIDRes.VerboseType)

		checkHandler(rsp.Resources[:len(rsp.Resources)-1])
	}

	{
		// send memory query
		req := &sfweb.YakURLRequest{
			Method: http.MethodGet,
			URL: &sfweb.YakURL{
				Schema:   "syntaxflow",
				Location: programName,
				Path:     path,
				Query: []*sfweb.Query{
					{
						Key:   "result_id",
						Value: resultID,
					},
				},
			},
			Body: sfCode,
		}
		body, err := json.Marshal(req)
		require.NoError(t, err)
		var rsp sfweb.YakURLResponse
		rawRsp, err := DoResponse(http.MethodPost, "/yakurl", &rsp, poc.WithReplaceHttpPacketBody(body, false))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rawRsp.GetStatusCode())

		resultIDRes := rsp.Resources[len(rsp.Resources)-1]
		require.Equal(t, resultIDRes.ResourceType, "result_id")
		require.Equal(t, resultIDRes.VerboseType, "result_id")
		// got result
		gotResultID := resultIDRes.ResourceName
		require.Equal(t, resultID, gotResultID)

		checkHandler(rsp.Resources[:len(rsp.Resources)-1])
	}
}

func checkVariable(t *testing.T, res []*sfweb.YakURLResource, want []string) {
	got := lo.FilterMap(res, func(r *sfweb.YakURLResource, _ int) (string, bool) {
		return r.ResourceName, r.ResourceType == "variable"
	})
	require.Len(t, got, len(want))
	require.Equal(t, want, got)
}

func TestYakURL(t *testing.T) {
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
	prog, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramPath("example"),
		ssaapi.WithProgramName(progID),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	}()
	require.NoError(t, err)
	require.NotNil(t, prog)

	t.Run("negative invalid request", func(t *testing.T) {
		var rsp sfweb.ErrorResponse
		rawRsp, err := DoResponse(http.MethodPost, "/yakurl", &rsp, poc.WithReplaceHttpPacketBody([]byte(`{`), false))
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, rawRsp.GetStatusCode())
		require.Contains(t, rsp.Message, "unmarshal request error")
	})

	t.Run("negative invalid schema", func(t *testing.T) {
		var rsp sfweb.ErrorResponse
		scheme := uuid.NewString()
		req := &sfweb.YakURLRequest{
			Method: http.MethodGet,
			URL: &sfweb.YakURL{
				FromRaw: fmt.Sprintf("%s://%s%s?save_result=true", scheme, progID, "/"),
			},
		}
		// FromRaw
		body, err := json.Marshal(req)
		require.NoError(t, err)
		rawRsp, err := DoResponse(http.MethodPost, "/yakurl", &rsp, poc.WithReplaceHttpPacketBody(body, false))
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, rawRsp.GetStatusCode())
		require.Equal(t, sfweb.NewInvalidSchemeError(scheme).Error(), rsp.Message)

		// Schema
		req.URL.FromRaw = ""
		req.URL.Schema = scheme
		body, err = json.Marshal(req)
		require.NoError(t, err)
		rawRsp, err = DoResponse(http.MethodPost, "/yakurl", &rsp, poc.WithReplaceHttpPacketBody(body, false))
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, rawRsp.GetStatusCode())
		require.Equal(t, sfweb.NewInvalidSchemeError(scheme).Error(), rsp.Message)
	})

	t.Run("check syntaxflow variable", func(t *testing.T) {
		sfweb.SfWebLogger.SetLevel("debug")
		checkSSAURL(t, progID, "/",
			`target2(* #-> as $a)`,
			func(res []*sfweb.YakURLResource) {
				checkVariable(t, res, []string{"a", "_"})
			},
		)
	})

	t.Run("check _", func(t *testing.T) {
		checkSSAURL(t, progID, "/", `target*`, func(res []*sfweb.YakURLResource) {
			checkVariable(t, res, []string{"_"})
		})
		checkSSAURL(t, progID, "/_", `target*`, func(res []*sfweb.YakURLResource) {
			require.Equal(t, 2, len(res))
		})
	})

	t.Run("check syntaxflow variable no data", func(t *testing.T) {
		checkSSAURL(t, progID, "/", `
		dddd as $a
		`, func(res []*sfweb.YakURLResource) {
			require.NoError(t, err)
			checkVariable(t, res, []string{"a"})
		})
	})

	t.Run("check syntaxflow variable with alert", func(t *testing.T) {
		checkSSAURL(t, progID, "/", `
		target2(* #-> as $a) 
		target1() as $target1
		alert $target1 for "alert information"
		`, func(res []*sfweb.YakURLResource) {
			checkVariable(t, res, []string{"target1", "a", "_"})
			target1 := res[0]
			require.Equal(t, target1.VerboseName, "alert information")
			require.Equal(t, target1.ResourceType, "variable")
			require.Equal(t, target1.VerboseType, "alert")
		})
	})
}
