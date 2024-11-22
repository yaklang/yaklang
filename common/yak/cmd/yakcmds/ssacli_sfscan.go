package yakcmds

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"io/fs"
	"strings"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

var SSACompilerSyntaxFlowCommand = &cli.Command{
	Name:    "code-scan",
	Aliases: []string{"sfscan"},
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "program,p",
			Usage: "program name for ssa compiler in db",
		},
		cli.BoolFlag{
			Name:  "code,show-code",
			Usage: "show code",
		},
		cli.StringFlag{
			Name:  "rule-keyword,rk,kw",
			Usage: `set rule keyword for file`,
		},
		cli.StringFlag{
			Name:  "rule-dir,rdir",
			Usage: `set rule dir for file`,
		},
	},
	Action: func(c *cli.Context) error {
		program := c.String("program")
		if program == "" {
			return utils.Error("program name is required")
		}

		prog, err := ssaapi.FromDatabase(program)
		if err != nil {
			return err
		}

		var results []*ssaapi.SyntaxFlowResult

		if c.String("rule-dir") != "" {
			lfs := filesys.NewRelLocalFs(c.String("rule-dir"))
			var rules []*schema.SyntaxFlowRule
			err := filesys.SimpleRecursive(filesys.WithFileSystem(lfs), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
				if lfs.Ext(s) != ".sf" {
					return nil
				}
				log.Infof("scan rule file: %s", s)
				raw, err := lfs.ReadFile(s)
				if err != nil {
					log.Warnf("read file: %s failed: %s", s, err)
					return nil
				}
				rule, err := sfdb.OnlyCreateSyntaxFlow(s, string(raw), false)
				if err != nil {
					return err
				}
				if rule.IncludedName != "" {
					log.Infof("skip rule: %s included: %s", rule.RuleName, rule.IncludedName)
					return nil
				}
				rules = append(rules, rule)
				return nil
			}))
			if err != nil {
				return err
			}

			for _, _rule := range rules {
				rule := _rule
				ScanWithSFRule(prog, rule, func(result *ssaapi.SyntaxFlowResult) {
					if ret := result.GetAlertValues(); ret.Len() > 0 {
						results = append(results, result)
					}
				})
			}
		} else {
			filterKw := c.String("rule-keyword")
			for rule := range sfdb.YieldSyntaxFlowRulesWithoutLib(consts.GetGormProfileDatabase(), context.Background()) {
				if filterKw != "" {
					if !strings.Contains(strings.ToLower(rule.RuleName), strings.ToLower(filterKw)) {
						continue
					}
				}
				rule := rule
				ScanWithSFRule(prog, rule, func(result *ssaapi.SyntaxFlowResult) {
					if ret := result.GetAlertValues(); ret.Len() > 0 {
						results = append(results, result)
					}
				})
			}
		}
		for _, result := range results {
			fmt.Println("-----------------------------------------")
			fmt.Println(result.Dump(c.Bool("code")))
			_, err := result.Save()
			if err != nil {
				log.Warnf("save result into database failed: %s", err)
			}
		}

		return nil
	},
}

func ScanWithSFRule(prog *ssaapi.Program, i *schema.SyntaxFlowRule, callback func(result *ssaapi.SyntaxFlowResult)) {
	defer func() {
		if err := recover(); err != nil {
			log.Warnf("execute: %v failed(recover): %s", i.Title, err)
		}
	}()
	result, err := prog.SyntaxFlowRule(i)
	if err != nil {
		log.Debugf("execute: %v failed: %s", i.Title, err)
		return
	}
	callback(result)
}
