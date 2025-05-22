package yakurl_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yak/yakurl"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func CheckSSADB(local ypb.YakClient, path string) error {
	url := &ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema: "ssadb",
			Path:   path,
		},
	}
	res, err := local.RequestYakURL(context.Background(), url)
	if err != nil {
		return err
	}
	spew.Dump(res)
	return nil
}
func SendURL(local ypb.YakClient, path string, result string) ([]*ypb.YakURLResource, error) {
	url := &ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema:   "syntaxflow",
			Location: "",
			Path:     path,
			Query: []*ypb.KVPair{
				{
					Key: "result_id",
					// check  this value all not empty
					Value: result,
				},
			},
		},
	}
	spew.Dump(url)
	res, err := local.RequestYakURL(context.Background(), url)
	if err != nil {
		return nil, err
	}
	return res.Resources, nil
}

type ssaurlTest struct {
	programName string
	local       ypb.YakClient

	DeferFunc []func()
}

func (s *ssaurlTest) RunDefer() {
	for _, f := range s.DeferFunc {
		f()
	}
}

func NewSSAURLTest(t *testing.T, vf filesys_interface.FileSystem, opt ...ssaapi.Option) *ssaurlTest {
	local, err := yakgrpc.NewLocalClient()
	if err != nil {
		t.Error(err)
	}
	_ = local

	progID := uuid.NewString()
	opt = append(opt, ssaapi.WithProgramName(progID))
	prog, err := ssaapi.ParseProjectWithFS(vf, opt...)

	require.NoError(t, err)
	require.NotNil(t, prog)

	ret := &ssaurlTest{
		programName: progID,
		local:       local,
	}
	ret.DeferFunc = append(ret.DeferFunc, func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	})
	return ret
}

func (s *ssaurlTest) CheckSSAURL(t *testing.T, path, sfCode string, checkHandler func([]*ypb.YakURLResource)) {
	var resultID string
	var memoryQuery, cacheQuery, databaseQuery time.Duration
	{
		// send memory query
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema:   "syntaxflow",
				Location: s.programName,
				Path:     path,
			},
			Body: []byte(sfCode),
		}

		start := time.Now()
		res, err := s.local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		memoryQuery = time.Since(start)
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
				Location: s.programName,
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
		start := time.Now()
		res, err := s.local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		cacheQuery = time.Since(start)
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
		prog, err := ssaapi.FromDatabase(s.programName)
		require.NoError(t, err)
		result := prog.SyntaxFlow(sfCode)
		resultID, err := result.Save(schema.SFResultKindDebug)
		require.NoError(t, err)
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema:   "syntaxflow",
				Location: s.programName,
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
		start := time.Now()
		res, err := s.local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		databaseQuery = time.Since(start)
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

	t.Log("memory query: ", memoryQuery)
	t.Log("cache query: ", cacheQuery)
	t.Log("database query: ", databaseQuery)

	// assert.True(t, databaseQuery < memoryQuery*10)
	// assert.True(t, (databaseQuery-memoryQuery)/memoryQuery < 1)
}

type variableResult struct {
	variable string
	number   int
}

func (s *ssaurlTest) CheckInfo(t *testing.T, sf string, want []string) {
	s.CheckSSAURL(t, "/", sf, func(res []*ypb.YakURLResource) {
		got := lo.FilterMap(res, func(r *ypb.YakURLResource, _ int) (string, bool) {
			if r.ResourceType != "message" {
				return "", false
			}
			return r.VerboseName, true
		})
		require.Equal(t, want, got)
	})
}
func (s *ssaurlTest) CheckVariable(t *testing.T, sf string, want []variableResult) {
	s.CheckSSAURL(t, "/", sf, func(res []*ypb.YakURLResource) {
		got := lo.FilterMap(res, func(r *ypb.YakURLResource, _ int) (variableResult, bool) {
			isVariable := r.ResourceType == "variable"
			ret := variableResult{
				variable: r.ResourceName,
				number:   int(r.Size),
			}
			return ret, isVariable
		})
		require.Equal(t, want, got)
	})
}

type valueResult struct {
	riskHash bool
	url      string
}

func (s *ssaurlTest) CheckValue(t *testing.T, sf string, varaible string, want []valueResult) {
	s.CheckSSAURL(t, fmt.Sprintf("/%s", varaible), sf, func(res []*ypb.YakURLResource) {
		got := lo.FilterMap(res, func(r *ypb.YakURLResource, index int) (valueResult, bool) {
			if r.ResourceType != "value" {
				return valueResult{}, false
			}
			ret := valueResult{}
			for _, extra := range r.Extra {
				if extra.Key == "risk_hash" {
					response, err := s.local.QuerySSARisks(context.Background(), &ypb.QuerySSARisksRequest{
						Filter: &ypb.SSARisksFilter{
							Hash: []string{extra.Value},
						},
					})
					require.NoError(t, err)
					require.Equal(t, 1, len(response.Data))
					require.Equal(t, extra.Value, response.Data[0].Hash)
					ret.riskHash = true
				}
				if extra.Key == "code_range" {
					var codeRange ssaapi.CodeRange
					err := json.Unmarshal([]byte(extra.Value), &codeRange)
					require.NoError(t, err)
					log.Infof("codeRange: %v", codeRange)
					err = CheckSSADB(s.local, codeRange.URL)
					require.NoError(t, err)
					splits := strings.Split(codeRange.URL, s.programName)
					require.Equal(t, 2, len(splits))
					ret.url = fmt.Sprintf("%s:%d", splits[1], codeRange.StartLine)
				}
			}
			return ret, true
		})

		require.Equal(t, want, got)
	})
}

func TestSFURL(t *testing.T) {
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
	s := NewSSAURLTest(t, vf,
		ssaapi.WithLanguage(consts.JAVA),
		ssaapi.WithProgramPath("example"),
	)
	defer s.RunDefer()

	t.Run("check syntaxflow variable", func(t *testing.T) {
		s.CheckVariable(t, `target2(* #-> as $a)`, []variableResult{
			{variable: "a", number: 1},
			{variable: "_", number: 1},
		})
	})

	t.Run("check _", func(t *testing.T) {
		s.CheckVariable(t, `target*`, []variableResult{
			{variable: "_", number: 2},
		})

		s.CheckValue(t, `target*`, "_", []valueResult{
			{riskHash: false, url: "/example/src/main/java/com/example/apackage/a.java:8"},
			{riskHash: false, url: "/example/src/main/java/com/example/bpackage/sub/b.java:8"},
		})
	})

	t.Run("check syntaxflow vairble no data", func(t *testing.T) {
		s.CheckVariable(t, "dddd as $a", []variableResult{
			{variable: "a", number: 0},
		})
	})

	t.Run("check syntaxflow variable with alert", func(t *testing.T) {
		s.CheckVariable(t, `
			target2(* #-> as $a) as $target1
			target1() as $target2
			$target1 + $target2 as $target 
			alert $target for "alert information"
			`, []variableResult{
			{variable: "target", number: 2},
			{variable: "a", number: 1},
			{variable: "target1", number: 1},
			{variable: "target2", number: 1},
		})
	})

	t.Run("check syntaxflow value with alert", func(t *testing.T) {
		s.CheckValue(t, `
			target2(* #-> as $a) as $target1
			target1() as $target2
			$target1 + $target2 as $target 
			alert $target for "alert information"
			`,
			"target", []valueResult{
				{riskHash: true, url: "/example/src/main/java/com/example/apackage/a.java:8"},
				{riskHash: true, url: "/example/src/main/java/com/example/bpackage/sub/b.java:8"},
			})
	})

	t.Run("check syntaxflow variable with check params", func(t *testing.T) {
		rule := `
			target2(* #-> as $a)
			$a?{!(opcode: const)} as $not_const_parameter
			$a?{(opcode: const)} as $const_parameter

			check $const_parameter then "has const parameter" else "no const parameter"
			check $not_const_parameter then "has not-const parameter" else "no not-const parameter"
			`
		s.CheckVariable(t, rule, []variableResult{
			{variable: "a", number: 1},
			{variable: "const_parameter", number: 1},
			{variable: "not_const_parameter", number: 0},
			{variable: "_", number: 1},
		})

		s.CheckInfo(t, rule, []string{
			"no not-const parameter",
			"has const parameter",
		})
	})

	t.Run("check syntaxflow value", func(t *testing.T) {
		rule := `target* as $target
		$target #{
			hook:<<<HOOK
				* as $a
HOOK
		}-> `

		s.CheckVariable(t, rule, []variableResult{
			{variable: "a", number: 2},
			{variable: "target", number: 2},
			{variable: "_", number: 2},
		})

		s.CheckValue(t, rule, "a", []valueResult{
			{riskHash: false, url: "/example/src/main/java/com/example/apackage/a.java:8"},
			{riskHash: false, url: "/example/src/main/java/com/example/bpackage/sub/b.java:8"},
		})
	})

	t.Run("check syntaxflow information", func(t *testing.T) {
		query := fmt.Sprintf(`
			target* as $target
			$target (* #{
				hook: %s
			}-> as $para_top_def)
			`, "`*  as $a`")
		s.CheckSSAURL(t, "/a/0", query, func(res []*ypb.YakURLResource) {
			check := func(path string) {
				err := CheckSSADB(s.local, path)
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

						check(info.CodeRange.GetPath())
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

func TestSFURL_golang(t *testing.T) {
	local, err := yakgrpc.NewLocalClient()
	if err != nil {
		t.Error(err)
	}
	_ = local

	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/go/go.mod", `
	module github.com/yaklang/yaklang

	go 1.20
	`)
	vf.AddFile("src/main/go/A/test1.go", `
package main

import (
        "database/sql"
        "fmt"
        "log"
        "net/http"

        _ "github.com/go-sql-driver/mysql"
)

func login(w http.ResponseWriter, r *http.Request) {
        username := r.FormValue("username")
        password := r.FormValue("password")

        // 不安全的 SQL 查询
        query := fmt.Sprintf("SELECT * FROM users WHERE username='%s' AND password='%s'", username, password)

        db, err := sql.Open("mysql", "user:password@/dbname")
        if err != nil {
                log.Fatal(err)
        }
        defer db.Close()

        var userID int
        err = db.QueryRow(query).Scan(&userID)
        if err != nil {
                http.Error(w, "Invalid login", http.StatusUnauthorized)
                return
        }

        fmt.Fprintf(w, "User ID: %d", userID)
}

func main() {
        http.HandleFunc("/login", login)
        log.Fatal(http.ListenAndServe(":8080", nil))
}


	`)

	s := NewSSAURLTest(t, vf,
		ssaapi.WithLanguage(consts.GO),
	)
	defer s.RunDefer()

	t.Run("check syntaxflow variable", func(t *testing.T) {
		s.CheckVariable(t,
			`	
				.QueryRow(* #-> as $a)
			`,
			[]variableResult{
				{variable: "a", number: 9},
				{variable: "_", number: 1},
			},
		)
	})

	t.Run("check syntaxflow information package with different filename", func(t *testing.T) {
		query := `
				.QueryRow(* #-> as $a)
			`
		s.CheckValue(t, query, "a", []valueResult{
			{riskHash: false, url: "/src/main/go/A/test1.go:5"},
			{riskHash: false, url: "/src/main/go/A/test1.go:6"},
			{riskHash: false, url: "/src/main/go/A/test1.go:13"},
			{riskHash: false, url: "/src/main/go/A/test1.go:14"},
			{riskHash: false, url: "/src/main/go/A/test1.go:14"},
			{riskHash: false, url: "/src/main/go/A/test1.go:15"},
			{riskHash: false, url: "/src/main/go/A/test1.go:18"},
			{riskHash: false, url: "/src/main/go/A/test1.go:20"},
			{riskHash: false, url: "/src/main/go/A/test1.go:20"},
		})
	})

	t.Run("check syntaxflow information", func(t *testing.T) {
		query := ".QueryRow(* #-> as $a) "
		s.CheckSSAURL(t, "/a/0", query, func(res []*ypb.YakURLResource) {
			check := func(path string) {
				err := CheckSSADB(s.local, path)
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

						check(info.CodeRange.GetPath())
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

func TestSSAURLPagination(t *testing.T) {

	local, err := yakgrpc.NewLocalClient()
	if err != nil {
		t.Error(err)
	}
	_ = local

	vf := filesys.NewVirtualFs()
	vf.AddFile("example/src/main/java/com/example/apackage/a.java", `
		package com.example.apackage;
		import com.example.bpackage.sub.B;
		class A {
			public static void main(String[] args) {
				print(1);
				print(2);
				print(3);
				print(4);
				print(5);
				print(6);
			}
		}
		`)

	progID := uuid.NewString()
	prog, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(consts.JAVA),
		ssaapi.WithProgramPath("example"),
		ssaapi.WithProgramName(progID),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	}()
	require.NoError(t, err)
	require.NotNil(t, prog)

	res, err := prog.SyntaxFlowWithError(`print( * as  $para)`, ssaapi.QueryWithSave(schema.SFResultKindDebug))
	res.Show()
	require.NoError(t, err)
	resultID := res.GetResultID()
	require.NotEqual(t, 0, resultID)

	query := func(page, pageSize int64, path string) ([]string, bool) {
		// send memory query
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema:   "syntaxflow",
				Location: progID,
				Path:     path,
				Query: []*ypb.KVPair{
					{
						// get from database
						Key:   "result_id",
						Value: codec.AnyToString(resultID),
					},
				},
			},
			Page:     page,
			PageSize: pageSize,
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		t.Log("checkHandler in database query ")
		spew.Dump(res)
		finish := false
		ret := make([]string, 0, len(res.Resources))
		for _, res := range res.Resources {
			if res.ResourceType == "value" {
				ret = append(ret, res.ResourceName)
			}

			if res.ResourceType == "result_id" {
				finish = true
			}

		}
		return ret, finish
	}

	t.Run("test lign pagination", func(t *testing.T) {
		total := make([]string, 0, 6)

		path := "/para"
		res, finish := query(1, 2, path)
		total = append(total, res...)
		require.Equal(t, false, finish)

		res, finish = query(2, 2, path)
		total = append(total, res...)
		require.Equal(t, false, finish)

		res, finish = query(3, 2, path)
		total = append(total, res...)
		require.Equal(t, true, finish)

		sort.Strings(total)
		require.Equal(t, []string{"1", "2", "3", "4", "5", "6"}, total)
	})

	t.Run("test not align pagination", func(t *testing.T) {
		total := make([]string, 0, 6)

		path := "/para"
		res, finish := query(1, 4, path)
		total = append(total, res...)
		require.Equal(t, false, finish)

		res, finish = query(2, 4, path)
		total = append(total, res...)
		require.Equal(t, true, finish)

		sort.Strings(total)
		require.Equal(t, []string{"1", "2", "3", "4", "5", "6"}, total)
	})

	t.Run("test _ ", func(t *testing.T) {

		// check "_"
		total := make([]string, 0, 6)

		path := "/_"
		res, finish := query(1, 4, path)
		total = append(total, res...)
		require.Equal(t, false, finish)

		res, finish = query(2, 4, path)
		total = append(total, res...)
		require.Equal(t, true, finish)

		sort.Strings(total)
		require.Equal(t, []string{"Undefined-print(1)", "Undefined-print(2)", "Undefined-print(3)", "Undefined-print(4)", "Undefined-print(5)", "Undefined-print(6)"}, total)

	})
}

func TestHaveRange(t *testing.T) {
	local, err := yakgrpc.NewLocalClient()
	if err != nil {
		t.Error(err)
	}
	_ = local

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

				Map<String, String> map = new HashMap<>();
				map.put("domain", domain);
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
		ssaapi.WithLanguage(consts.JAVA),
		ssaapi.WithProgramPath("example"),
		ssaapi.WithProgramName(progID),
	)
	defer func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
	}()
	require.NoError(t, err)
	require.NotNil(t, prog)

	result, err := prog.SyntaxFlowWithError(`__dependency__ as $main`, ssaapi.QueryWithSave(schema.SFResultKindDebug))
	require.NoError(t, err)
	resId := result.GetResultID()
	require.Greater(t, resId, uint(0))

	// check  this value all empty
	{
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema:   "syntaxflow",
				Location: progID,
				Path:     "/main",
				Query: []*ypb.KVPair{
					{
						// get from database
						Key:   "result_id",
						Value: codec.AnyToString(resId),
					},
				},
			},
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		t.Log("checkHandler in database query ")
		spew.Dump(res)

		emptyRange := true
		for _, item := range res.Resources {
			if item.ResourceType == "value" {
				for _, extra := range item.Extra {
					if extra.Key == "code_range" {
						var rng map[string]any
						t.Log(extra.Value)
						err := json.Unmarshal([]byte(extra.Value), &rng)
						require.NoError(t, err)
						if rng["url"] != "" {
							emptyRange = false
						}
					}
				}
			}
		}
		require.True(t, emptyRange)
	}
	// check  no value return
	{
		url := &ypb.RequestYakURLParams{
			Method: "GET",
			Url: &ypb.YakURL{
				Schema:   "syntaxflow",
				Location: progID,
				Path:     "/main",
				Query: []*ypb.KVPair{
					{
						// get from database
						Key:   "result_id",
						Value: codec.AnyToString(resId),
					},
					{
						Key:   "have_range",
						Value: "true",
					},
				},
			},
		}
		res, err := local.RequestYakURL(context.Background(), url)
		require.NoError(t, err)
		t.Log("checkHandler in database query ")
		spew.Dump(res)
		require.Equal(t, len(res.Resources), 1) // just result id item

		haveValue := false
		for _, item := range res.Resources {
			if item.ResourceType == "value" {
				haveValue = true
			}
		}
		require.False(t, haveValue)
	}
}
