package mustpass

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yak"
)

var files = make(map[string]string)

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

func TestMustPass(t *testing.T) {
	yakit.RegisterLowHTTPSaveCallback()

	var cases [][]string
	for k, v := range files {
		cases = append(cases, []string{k, v})
	}

	sort.SliceStable(cases, func(i, j int) bool {
		return cases[i][0] < cases[j][0]
	})

	if vulinboxAddr == "" {
		panic("VULINBOX START ERROR")
	}

	for _, i := range cases {
		caseName, caseContent := i[0], i[1]
		t.Run(caseName, func(t *testing.T) {
			t.Parallel()

			_, err := yak.Execute(caseContent, map[string]interface{}{
				"VULINBOX":      vulinboxAddr,
				"VULINBOX_HOST": utils.ExtractHostPort(vulinboxAddr),
			})
			if err != nil {
				t.Fatalf("run script[%s] error: %v", caseName, err)
			}
		})
	}
}
