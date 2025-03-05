package coreplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func sendSSAURL(t *testing.T, local ypb.YakClient, resultID int, programName, kind string) []*ypb.YakURLResource {
	url := &ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema:   "syntaxflow",
			Location: programName,
			Path:     fmt.Sprintf("/%s", kind),
			Query: []*ypb.KVPair{
				{
					// get from database
					Key:   "result_id",
					Value: strconv.Itoa(resultID),
				},
			},
		},
	}
	res, err := local.RequestYakURL(context.Background(), url)
	require.NoError(t, err)
	t.Log("checkHandler in database query ")
	resultIDRes := res.Resources[len(res.Resources)-1]
	require.Equal(t, resultIDRes.ResourceType, "result_id")
	require.Equal(t, resultIDRes.VerboseType, "result_id")
	// got result
	gotResultID := resultIDRes.ResourceName
	require.Equal(t, strconv.Itoa(resultID), gotResultID)

	return res.Resources[:len(res.Resources)-1]
}

func getRangeText(res *ypb.YakURLResource, client ypb.YakClient) (string, error) {
	var rng ssaapi.CodeRange
	var source string

	for _, res := range res.Extra {
		if res.Key == "code_range" {
			if err := json.Unmarshal([]byte(res.Value), &rng); err != nil {
				return "", err
			}
		}
		if res.Key == "source" {
			source = res.Value
		}
	}

	// check rng file url
	if rng.URL == "" {
		return "", fmt.Errorf("no file url in code range")
	}
	if response, err := client.RequestYakURL(context.Background(), &ypb.RequestYakURLParams{
		Method: "GET",
		Url: &ypb.YakURL{
			Schema: "ssadb",
			Path:   rng.URL,
		},
	}); err != nil {
		return "", utils.Wrapf(err, "request file url %s failed", rng.URL)
	} else {
		spew.Dump(response)
	}

	// get source code by range
	editor := memedit.NewMemEditor(source)
	got := editor.GetTextFromPositionInt(
		int(rng.StartLine-rng.SourceCodeLine), int(rng.StartColumn),
		int(rng.EndLine-rng.SourceCodeLine), int(rng.EndColumn),
	)
	return got, nil
}

type sfSearch struct {
	fs    filesys_interface.FileSystem
	local ypb.YakClient
	code  string

	progName string

	t *testing.T
}

func NewSfSearch(fs filesys_interface.FileSystem, t *testing.T, opt ...ssaapi.Option) *sfSearch {
	progName := uuid.NewString()
	client, err := yakgrpc.NewLocalClient()
	require.NoError(t, err)
	{

		opt = append(opt,
			ssaapi.WithFileSystem(fs),
			ssaapi.WithProgramName(progName),
		)
		_, err := ssaapi.ParseProject(opt...)
		require.NoError(t, err)
		t.Cleanup(func() {
			log.Infof("delete program: %v", progName)
			// ssadb.DeleteProgram(ssadb.GetDB(), progName)
		})
	}
	_, err = ssaapi.FromDatabase(progName)
	require.NoError(t, err)

	pluginName := "SyntaxFlow Searcher"
	initDB.Do(func() {
		yakit.InitialDatabase()
	})
	codeBytes := GetCorePluginData(pluginName)
	require.NotNilf(t, codeBytes, "无法从bindata获取: %v", pluginName)

	return &sfSearch{
		fs:       fs,
		local:    client,
		progName: progName,
		code:     string(codeBytes),
		t:        t,
	}
}

func (s *sfSearch) RunSearch(kind, input string, fuzz bool) int {
	stream, err := s.local.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code:       s.code,
		PluginType: "yak",
		ExecParams: []*ypb.KVPair{
			{
				Key:   "kind",
				Value: kind,
			},
			{
				Key:   "rule",
				Value: input,
			},
			{
				Key:   "progName",
				Value: s.progName,
			},
			{
				Key:   "fuzz",
				Value: strconv.FormatBool(fuzz),
			},
		},
	})
	require.NoError(s.t, err)
	resultId := -1
	result := new(msg)
	for {
		exec, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			require.NoError(s.t, err)
		}
		if exec.IsMessage {
			rawMsg := exec.GetMessage()
			fmt.Println("raw msg: ", string(rawMsg))
			json.Unmarshal(rawMsg, &result)
			if result.Content.Level == "json" && result.Content.Data != "" {
				id, err := strconv.Atoi(result.Content.Data)
				if err != nil {
					log.Errorf("invalid result id: %v", string(rawMsg))
					continue
				}
				resultId = id
				break
			}
		}
	}
	return resultId
}

func (s *sfSearch) Check(t *testing.T, kind string, resultId int, want map[string]string) {
	rets := sendSSAURL(t, s.local, resultId, s.progName, kind)
	spew.Dump(rets)

	got := lo.SliceToMap(rets, func(ret *ypb.YakURLResource) (string, string) {
		if ret.ResourceType != "value" {
			return "", ""
		}
		key := ret.ResourceName
		source, err := getRangeText(ret, s.local)
		require.NoError(t, err)
		return key, source
	})
	spew.Dump("got:", got)
	spew.Dump("want:", want)
	for name, source := range want {
		got, ok := got[name]
		require.True(t, ok, "not found: %v", name)
		require.Equal(t, source, got)
	}
}

func (s *sfSearch) SearchAndCheck(t *testing.T, kind, input string, fuzz bool, want map[string]string) {
	resultId := s.RunSearch(kind, input, fuzz)
	s.Check(t, kind, resultId, want)
}

func TestSSASearch(t *testing.T) {
	fs := filesys.NewVirtualFs()
	code1 := `<?php
$b = "funcA(";
function funcA(){}
funcA(111);

function funcAxxxx() {}
function yyyyfuncAxxxx() {}
`
	fs.AddFile("/var/www/html/1.php", code1)
	code3 := `<?php
funcA(222);
`
	fs.AddFile("/var/www/html/funcA.php", code3)

	s := NewSfSearch(fs, t, ssaapi.WithLanguage(ssaapi.PHP))

	t.Run("check all funcA", func(t *testing.T) {
		s.SearchAndCheck(t, "all", "funcA", false, map[string]string{
			"Function-funcA":           "function funcA(){}",
			"Undefined-funcA":          "funcA",
			`"funcA("`:                 "funcA(",
			"Function-funcA(111)":      "funcA(111)",
			"Undefined-funcA(222)":     "funcA(222)",
			`"var/www/html/funcA.php"`: code3,
		})
	})

	t.Run("check symbol funcA", func(t *testing.T) {
		s.SearchAndCheck(t, "symbol", "funcA", false, map[string]string{
			"Function-funcA":  "function funcA(){}",
			"Undefined-funcA": "funcA",
		})
	})

	t.Run("check function funcA", func(t *testing.T) {
		s.SearchAndCheck(t, "function", "funcA", false, map[string]string{
			"Function-funcA": "function funcA(){}",
		})
	})

	t.Run("check function funcA with fuzz", func(t *testing.T) {
		s.SearchAndCheck(t, "function", "funcA", true, map[string]string{
			"Function-funcA":         "function funcA(){}",
			"Function-funcAxxxx":     "function funcAxxxx() {}",
			"Function-yyyyfuncAxxxx": "function yyyyfuncAxxxx() {}",
		})
	})

	t.Run("check call funcA", func(t *testing.T) {
		s.SearchAndCheck(t, "call", "funcA", false, map[string]string{
			"Function-funcA(111)":  "funcA(111)",
			"Undefined-funcA(222)": "funcA(222)",
		})
	})

	t.Run("check file funcA", func(t *testing.T) {
		s.SearchAndCheck(t, "file", "funcA", false, map[string]string{
			`"var/www/html/funcA.php"`: code3,
		})
	})

	t.Run("check const funcA", func(t *testing.T) {
		s.SearchAndCheck(t, "const", "funcA", false, map[string]string{
			`"funcA("`: "funcA(",
		})
	})

	t.Run("check auto select to call", func(t *testing.T) {
		s.SearchAndCheck(t, "all", "funcA(111)", false, map[string]string{
			"Function-funcA(111)": "funcA(111)",
		})
	})
}

func TestSSASearch_OnceSearch_MultipleQueryKind(t *testing.T) {
	fs := filesys.NewVirtualFs()
	code1 := `<?php
$b = "funcA(";
function funcA(){}
funcA(111);

function funcAxxxx() {}
function yyyyfuncAxxxx() {}
`
	fs.AddFile("/var/www/html/1.php", code1)
	code3 := `<?php
funcA(222);
`
	fs.AddFile("/var/www/html/funcA.php", code3)

	s := NewSfSearch(fs, t, ssaapi.WithLanguage(ssaapi.PHP))

	// search
	_ = s
	result := s.RunSearch("all", "funcA", true)

	// check all
	s.Check(t, "all", result, map[string]string{
		"Function-funcA":           "function funcA(){}",
		"Undefined-funcA":          "funcA",
		`"funcA("`:                 "funcA(",
		"Function-funcA(111)":      "funcA(111)",
		"Undefined-funcA(222)":     "funcA(222)",
		`"var/www/html/funcA.php"`: code3,
	})

	// check file
	s.Check(t, "file", result, map[string]string{
		`"var/www/html/funcA.php"`: code3,
	})

	// check function
	s.Check(t, "function", result, map[string]string{
		"Function-funcA": "function funcA(){}",
	})

	// check symbol
	s.Check(t, "symbol", result, map[string]string{
		"Function-funcA":  "function funcA(){}",
		"Undefined-funcA": "funcA",
	})

	// check call
	s.Check(t, "call", result, map[string]string{
		"Function-funcA(111)":  "funcA(111)",
		"Undefined-funcA(222)": "funcA(222)",
	})

	// check const
	s.Check(t, "const", result, map[string]string{
		`"funcA("`: "funcA(",
	})

}

func TestSSASearch_MultipleSearch_HitCache(t *testing.T) {
	fs := filesys.NewVirtualFs()
	code1 := `<?php
$b = "funcA(";
function funcA(){}
funcA(111);

function funcAxxxx() {}
function yyyyfuncAxxxx() {}
`
	fs.AddFile("/var/www/html/1.php", code1)
	code3 := `<?php
funcA(222);
`
	fs.AddFile("/var/www/html/funcA.php", code3)

	s := NewSfSearch(fs, t, ssaapi.WithLanguage(ssaapi.PHP))

	// search
	result := s.RunSearch("all", "funcA", true)
	log.Infof("result: %v", result)

	t.Run("check database cache ", func(t *testing.T) {
		// check database cache
		key := fmt.Sprint([]any{s.progName, "all", "funcA"})
		log.Infof("key: %s", key)
		res, err := s.local.GetKey(context.Background(), &ypb.GetKeyRequest{
			Key: key,
		})
		require.NoError(t, err)
		got := res.GetValue()
		log.Infof("got: %v", got)
		gotResult, err := strconv.Atoi(got)
		require.NoError(t, err)
		require.Equal(t, result, gotResult)
	})

	t.Run("check search again", func(t *testing.T) {
		// search again
		resultGot := s.RunSearch("all", "funcA", true)
		require.Equal(t, result, resultGot)
	})

	t.Run("check search again with different kind: should same id", func(t *testing.T) {
		// search
		// other kind can get "all" kind cache
		resultGot := s.RunSearch("file", "funcA", true)
		require.Equal(t, result, resultGot)
	})

	t.Run("all kind con't got other kind", func(t *testing.T) {
		str := strings.ReplaceAll(uuid.NewString(), "-", "")
		resultWant := s.RunSearch("file", str, true)
		resultGot := s.RunSearch("all", str, true)
		require.NotEqual(t, resultWant, resultGot)
	})

	t.Run("negative: delete result but still in cache", func(t *testing.T) {
		str := strings.ReplaceAll(uuid.NewString(), "-", "")
		// search
		result1 := s.RunSearch("all", str, true)
		log.Infof("result: %v", result1)
		ssadb.DeleteResultByID(uint(result1))
		// search again
		resultGot := s.RunSearch("all", str, true)
		require.NotEqual(t, result1, resultGot)
	})

}
