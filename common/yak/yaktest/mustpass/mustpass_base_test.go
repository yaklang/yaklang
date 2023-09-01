package mustpass

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yak"
)

var (
	files = make(map[string]string)
)

//go:embed files/*
var testFiles embed.FS

var vulinboxAddr string

func init() {
	dirs, err := testFiles.ReadDir("files")
	if err != nil {
		panic(err)
	}
	for _, i := range dirs {
		if !strings.HasSuffix(i.Name(), ".yak") {
			continue
		}
		// https://github.com/golang/go/issues/45230
		raw, err := testFiles.ReadFile(fmt.Sprintf("files/%s", i.Name()))
		if err != nil {
			panic(err)
		}
		files[i.Name()] = string(raw)
	}
	consts.GetGormProfileDatabase()
	consts.GetGormProjectDatabase()
	yak.NewScriptEngine(1)

	vulinboxAddr, err = vulinbox.NewVulinServer(context.Background())
	if err != nil {
		panic("VULINBOX START ERROR")
	}
}

func TestMustPassDebug(t *testing.T) {
	var debugName = "git_to_sca.yak"
	var debugCases [][]string
	for k, v := range files {
		if k == debugName {
			debugCases = append(debugCases, []string{k, v})
		}
	}

	sort.SliceStable(debugCases, func(i, j int) bool {
		return debugCases[i][0] < debugCases[j][0]
	})

	if vulinboxAddr == "" {
		panic("VULINBOX START ERROR")
	}

	totalTest := t
	for _, i := range debugCases {
		t.Run(i[0], func(t *testing.T) {
			_, err := yak.Execute(i[1], map[string]any{
				"VULINBOX": vulinboxAddr,
			})
			if err != nil {
				t.Fatalf("[%v] error: %v", i[0], err)
				totalTest.FailNow()
			}
		})
	}
}

func TestMustPass(t *testing.T) {
	var debugName = "nuclei_scan.yak"
	var debugCases [][]string
	var cases [][]string
	for k, v := range files {
		if k == debugName {
			debugCases = append(debugCases, []string{k, v})
		} else {
			cases = append(cases, []string{k, v})
		}
	}

	sort.SliceStable(debugCases, func(i, j int) bool {
		return debugCases[i][0] < debugCases[j][0]
	})
	sort.SliceStable(cases, func(i, j int) bool {
		return cases[i][0] < cases[j][0]
	})

	if vulinboxAddr == "" {
		panic("VULINBOX START ERROR")
	}

	totalTest := t
	for _, i := range debugCases {
		t.Run(i[0], func(t *testing.T) {
			_, err := yak.NewScriptEngine(1).ExecuteEx(i[1], map[string]any{
				"VULINBOX": vulinboxAddr,
			})
			if err != nil {
				t.Fatalf("[%v] error: %v", i[0], err)
				totalTest.FailNow()
			}
		})
	}

	for _, i := range cases {
		t.Run(i[0], func(t *testing.T) {
			_, err := yak.Execute(i[1], map[string]any{
				"VULINBOX": vulinboxAddr,
			})
			if err != nil {
				t.Fatalf("[%v] error: %v", i[0], err)
			}
		})
	}
}
