package buildin_rule

import (
	"embed"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"io/fs"
	"path"
	"testing"
)

//go:embed sample
var samples embed.FS

type BuildinRuleTestCase struct {
	Name           string
	Rule           string
	FS             map[string]string
	ContainsAll    []string
	NotContainsAny []string
}

func run(t *testing.T, name string, c BuildinRuleTestCase) {
	t.Run(name, func(t *testing.T) {
		rules, err := sfdb.GetRules(c.Rule)
		if err != nil {
			t.Fatal(err)
		}
		if len(rules) <= 0 {
			t.Fatal("no rule found")
		}
		vfs := filesys.NewVirtualFs()
		for k, v := range c.FS {
			filesys.Recursive(".", filesys.WithEmbedFS(samples), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
				_, name := path.Split(s)
				if utils.MatchAllOfGlob(name, v) {
					raw, err := samples.ReadFile(s)
					if err != nil {
						log.Warnf("read file %s error: %s", s, err)
						t.Fatal("load file error: " + v)
					}
					vfs.AddFile(k, string(raw))
				}
				return nil
			}))
		}
		for _, r := range rules {
			ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
				if len(programs) <= 0 {
					t.Fatal("no program found")
				}
				for _, prog := range programs {
					result, err := prog.SyntaxFlowWithError(r.Content)
					if err != nil || result.Errors != nil {
						if err != nil {
							t.Fatal(err)
						} else {
							t.Fatal(result.Errors)
						}
					}
					if len(result.AlertSymbolTable) >= 0 {
						for name, val := range result.AlertSymbolTable {
							msg := fmt.Sprintf("%v: %s: %s", r.Severity, name, val)
							t.Logf(msg)
							if len(c.ContainsAll) > 0 {
								if !utils.MatchAllOfSubString(msg, c.ContainsAll...) {
									t.Fatal("not all contains")
								}
							}
							if len(c.NotContainsAny) > 0 {
								if utils.MatchAnyOfSubString(msg, c.NotContainsAny...) {
									t.Fatal("contain any")
								}
							}
						}
					} else {
						t.Fatal("no alert found no result found")
					}
				}
				return nil
			})
		}
	})
}
