package mustpass

import (
	"embed"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yak"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var (
	files = make(map[string]string)
)

//go:embed files/*
var testFiles embed.FS

func init() {
	dirs, err := testFiles.ReadDir("files")
	if err != nil {
		panic(err)
	}
	for _, i := range dirs {
		if !strings.HasSuffix(i.Name(), ".yak") {
			continue
		}
		raw, err := testFiles.ReadFile(filepath.Join("files", i.Name()))
		if err != nil {
			panic(err)
		}
		files[i.Name()] = string(raw)
	}
	consts.GetGormProfileDatabase()
	consts.GetGormProjectDatabase()
	yak.NewScriptEngine(1)
}

func TestMustPass(t *testing.T) {
	var debugName = "nuclei_network.yak"
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

	for _, i := range debugCases {
		t.Run(i[0], func(t *testing.T) {
			err := yak.NewScriptEngine(1).Execute(i[1])
			if err != nil {
				t.Fatalf("[%v] error: %v", i[0], err)
			}
		})
	}

	for _, i := range cases {
		t.Run(i[0], func(t *testing.T) {
			err := yak.NewScriptEngine(1).Execute(i[1])
			if err != nil {
				t.Fatalf("[%v] error: %v", i[0], err)
			}
		})
	}
}
