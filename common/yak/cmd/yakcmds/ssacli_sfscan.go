package yakcmds

import (
	"context"
	"fmt"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"io/fs"
	"path/filepath"
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

		var opt []ssaapi.Option
		prog, err := ssaapi.FromDatabase(program, opt...)
		if err != nil {
			return err
		}

		var results []*ssaapi.SyntaxFlowResult

		filterKw := c.String("rule-keyword")

		isHitByFiltered := func(i string) bool {
			if filterKw != "" {
				kws := utils.PrettifyListFromStringSplited(filterKw, ",")
				if utils.MatchAnyOfSubString(i, kws...) {
					return true
				}
			} else {
				return true
			}
			return false
		}

		if c.String("rule-dir") != "" {
			dir := c.String("rule-dir")
			originDir := dir
			var filterKwExtra string
			if utils.GetFirstExistedPath(dir) == "" {
				// no existed path
				dir, filterKwExtra = filepath.Split(dir)
				if utils.GetFirstExistedPath(dir) == "" {
					return utils.Errorf("rule dir [%v or %v] not existed", dir, originDir)
				}
			} else if utils.GetFirstExistedFile(dir) != "" {
				// is a single file
				dir, filterKwExtra = filepath.Split(dir)
			}

			if filterKwExtra != "" {
				filterKw += "," + filterKwExtra
			}

			log.Infof("start to create rel local fs: %s", dir)
			lfs := filesys.NewRelLocalFs(dir)
			var rules []*schema.SyntaxFlowRule
			err := filesys.SimpleRecursive(filesys.WithFileSystem(lfs), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
				if !isHitByFiltered(s) {
					return nil
				}

				log.Infof("checking file: %s", s)
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

			defer func() {
				for _, r := range rules {
					log.Infof("handled local fs rule: %s", r.RuleName)
				}
			}()
		} else {
			for rule := range sfdb.YieldSyntaxFlowRulesWithoutLib(consts.GetGormProfileDatabase(), context.Background()) {
				if !isHitByFiltered(rule.RuleName) {
					log.Infof("skip rule: %v", rule.RuleName)
					continue
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
