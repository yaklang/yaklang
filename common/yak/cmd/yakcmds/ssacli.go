package yakcmds

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"golang.org/x/exp/slices"

	"github.com/segmentio/ksuid"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

var ssaRemove = &cli.Command{
	Name:    "ssa-remove",
	Aliases: []string{"ssa-rm"},
	Usage:   "Remove SSA OpCodes from database",
	Action: func(c *cli.Context) {
		for _, name := range c.Args() {
			if name == "*" {
				for _, name := range ssadb.AllProgramNames(ssadb.GetDB()) {
					log.Infof("Start to delete program: %v", name)
					ssadb.DeleteProgram(ssadb.GetDB(), name)
				}
				break
			}
			log.Infof("Start to delete program: %v", name)
			ssadb.DeleteProgram(ssadb.GetDB(), name)
		}
	},
}

var staticCheck = &cli.Command{
	Name:    "static-check",
	Aliases: []string{"check"},
	Usage:   "Check Code",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:     "input-file,file",
			Required: true,
		},
		cli.StringFlag{
			Name:     "rules",
			Required: true,
		},
		cli.StringFlag{
			Name: "language",
		},
		cli.StringFlag{
			Name: "exclude-file",
		},
	},
	Action: func(c *cli.Context) error {
		var sfrules []*schema.SyntaxFlowRule
		file := c.String("file")
		rules := c.String("rules")
		language := c.String("language")
		excludeFiles := c.StringSlice("exclude-file")
		var excludeCompile []glob.Glob
		for _, s := range excludeFiles {
			compile, err := glob.Compile(s)
			if err != nil {
				return err
			}
			excludeCompile = append(excludeCompile, compile)
		}
		zipfs, err2 := filesys.NewZipFSFromLocal(file)
		if err2 != nil {
			return err2
		}
		filesys.Recursive(rules, filesys.WithFileStat(func(s string, info fs.FileInfo) error {
			if !strings.HasSuffix(s, ".sf") {
				return nil
			}
			raw, err := os.ReadFile(s)
			if err != nil {
				return err
			}
			sfrule, err := sfdb.CheckSyntaxFlowRuleContent(string(raw))
			if err != nil {
				return err
			}
			if sfrule.RuleName == "" {
				sfrule.RuleName = s
			}
			sfrules = append(sfrules, sfrule)
			return nil
		}))
		programs, err := ssaapi.ParseProjectWithFS(zipfs, ssaapi.WithRawLanguage(language), ssaapi.WithExcludeFile(func(path, filename string) bool {
			for _, g := range excludeCompile {
				if g.Match(file) {
					return true
				}
			}
			return false
		}))
		if err != nil {
			return err
		}
		var ruleError error
		addError := func(err error, ruleName string) {
			ruleError = utils.JoinErrors(ruleError, fmt.Errorf("execute syntaxRule[%s] fail,reason: %s", ruleName, err))
		}
		for _, sfrule := range sfrules {
			result, err := programs.SyntaxFlowRule(sfrule, ssaapi.QueryWithEnableDebug(true))
			if err != nil {
				addError(err, sfrule.RuleName)
				continue
			}
			values := result.GetAlertValues()
			if values.Len() != 0 {
				addError(errors.New("alert number is not null"), sfrule.RuleName)
				result.Show()
			}
		}
		if ruleError != nil {
			return ruleError
		}
		return nil
	},
}

var ssaCompile = &cli.Command{
	Name:    "ssa-compile",
	Aliases: []string{"ssa"},
	Usage:   "Compile to SSA OpCodes from source code",
	Flags: []cli.Flag{
		cli.StringFlag{Name: "log", Usage: "log level"},
		cli.StringFlag{
			Name: "language,l",
		},
		cli.StringFlag{
			Name:  "target,t",
			Usage: `target file or directory`,
		},
		cli.StringFlag{
			Name:  "program,p",
			Usage: `program name to save in database`,
		},
		cli.StringFlag{
			Name:  "entry",
			Usage: "Program Entry",
		},
		cli.BoolFlag{
			Name: "memory",
		},
		cli.StringFlag{
			Name:  "syntaxflow,sf",
			Usage: "syntax flow query language",
		},
		cli.StringFlag{
			Name:  "database,db",
			Usage: "database path",
		},
		cli.StringFlag{
			Name:  "database-dialect,db-dialect",
			Usage: "database dialect for gorm, support: mysql, sqlite3(default)",
		},
		cli.BoolFlag{
			Name:  "database-debug,dbdebug",
			Usage: "enable database debug mode",
		},
		cli.BoolFlag{
			Name:  "syntaxflow-debug,sfdebug",
			Usage: "enable syntax flow debug mode",
		},
		cli.BoolFlag{
			Name: "no-override", Usage: "no override existed database program(no delete)",
		},
		cli.BoolFlag{
			Name: "re-compile", Usage: "re-compile existed database program",
		},
		cli.BoolFlag{
			Name: "dot", Usage: "dot graph text for result",
		},
		cli.BoolFlag{
			Name: "with-code,code", Usage: "show code context",
		},
		cli.BoolFlag{
			Name: "no-frontend",
			Usage: `in default, you can see program that compiled by ssa-cli in Yakit Frontend.
				you can use --no-frontend to disable this function`,
		},
		cli.StringSliceFlag{
			Name: "exclude-file",
			Usage: `exclude default file,only support glob mode. eg.
					targets/*, vendor/*`,
		},
	},
	Action: func(c *cli.Context) error {
		if ret, err := log.ParseLevel(c.String("log")); err == nil {
			log.SetLevel(ret)
		}

		programName := c.String("program")
		reCompile := c.Bool("re-compile")
		if programName != "" {
			defer func() {
				ssa.ShowDatabaseCacheCost()
			}()
		}
		entry := c.String("entry")
		input_language := c.String("language")
		inMemory := c.Bool("memory")
		rawFile := c.String("target")
		target := utils.GetFirstExistedPath(rawFile)
		databaseFileRaw := c.String("database")
		databaseDialect := c.String("database-dialect")
		noOverride := c.Bool("no-override")
		syntaxFlow := c.String("syntaxflow")
		dbDebug := c.Bool("database-debug")
		sfDebug := c.Bool("syntaxflow-debug")
		showDot := c.Bool("dot")
		withCode := c.Bool("with-code")
		excludeFile := c.StringSlice("exclude-file")

		var excludeCompile []glob.Glob
		for _, s := range excludeFile {
			compile, err := glob.Compile(s)
			if err != nil {
				return err
			}
			excludeCompile = append(excludeCompile, compile)
		}
		// check program name duplicate
		if prog, err := ssadb.GetProgram(programName, ssa.Application); prog != nil && err == nil {
			if !reCompile {
				return utils.Errorf(
					"program name %v existed in this database, please use `re-compile` flag to re-compile or change program name",
					programName,
				)
			}
		}

		opt := make([]ssaapi.Option, 0, 3)
		// set database
		if databaseDialect != "" {
			// if set dialect, open gorm and set db
			if databaseFileRaw == "" {
				return utils.Errorf("database path is required when using database dialect")
			}
			db, err := gorm.Open(databaseDialect, databaseFileRaw)
			if err != nil {
				return utils.Errorf("open database failed: %v", err)
			}
			consts.SetGormSSAProjectDatabaseByDB(db)
		}
		// if not set dialect, use existed db
		if databaseDialect == "" && databaseFileRaw != "" {
			// set database path
			if target == "" &&
				utils.GetFirstExistedFile(databaseFileRaw) == "" {
				// no compile ,database not existed
				return utils.Errorf("database file not found: %v", databaseFileRaw)
			}
			consts.SetGormSSAProjectDatabaseByPath(databaseFileRaw)
		}

		if slices.Contains(ssadb.AllProgramNames(ssadb.GetDB()), programName) {
			if !reCompile {
				return utils.Errorf(
					"program name %v existed in other database, please use `re-compile` flag to re-compile or change program name",
					programName,
				)
			}
		}

		// compile
		if target == "" {
			return utils.Errorf("target file not found: %v", rawFile)
		}
		log.Infof("start to compile file: %v ", target)
		opt = append(opt, ssaapi.WithRawLanguage(input_language))
		opt = append(opt, ssaapi.WithReCompile(reCompile))
		opt = append(opt, ssaapi.WithExcludeFile(func(path, filename string) bool {
			for _, g := range excludeCompile {
				if g.Match(filename) {
					return true
				}
			}
			return false
		}))

		if entry != "" {
			log.Infof("start to use entry file: %v", entry)
			opt = append(opt, ssaapi.WithFileSystemEntry(entry))
		}

		if inMemory {
			//纯内存模式，cache将只会保留一个小时
			log.Infof("compile in memory mode, program-name will be ignored")
		} else {
			if programName == "" {
				programName = "default-" + ksuid.New().String()
			}
			log.Infof("compile save to database with program name: %v", programName)
			opt = append(opt, ssaapi.WithProgramName(programName))
		}

		if !noOverride {
			ssadb.DeleteProgram(ssadb.GetDB(), programName)
		} else {
			log.Warnf("no-override flag is set, will not delete existed program: %v", programName)
		}

		var proj ssaapi.Programs
		zipfs, err := filesys.NewZipFSFromLocal(target)
		if err == nil {
			proj, err = ssaapi.ParseProjectWithFS(zipfs, opt...)
			if err != nil {
				return utils.Errorf("parse project [%v] failed: %v", target, err)
			}
		} else {
			proj, err = ssaapi.ParseProjectFromPath(target, opt...)
			if err != nil {
				return utils.Errorf("parse project [%v] failed: %v", target, err)
			}
		}

		log.Infof("finished compiling..., results: %v", len(proj))
		if syntaxFlow != "" {
			log.Warn("Deprecated: syntax flow query language will be removed in ssa sub-command, please use `ssa-query(in short: sf/syntaxFlow)` instead")
			return SyntaxFlowQuery(programName, syntaxFlow, dbDebug, sfDebug, showDot, withCode)
		}
		return nil
	},
}

var syntaxFlowCreate = &cli.Command{
	Name:    "syntaxflow-create",
	Aliases: []string{"create-sf", "csf"},
	Usage:   "create syntaxflow template file",
	Flags: []cli.Flag{
		cli.StringFlag{Name: "language,l"},
		cli.StringFlag{Name: "keyword"},
		cli.BoolFlag{Name: "is-vuln,vuln,v", Usage: "set the current SyntaxFlow Rule is a vuln (in desc)"},
		cli.BoolFlag{Name: "audit-suggestion,audit,a", Usage: "set the current SyntaxFlow Rule is a suggestion"},
		cli.BoolFlag{Name: "sec-config,security-config,s", Usage: "set the current SyntaxFlow Rule is a suggestion"},
		cli.StringFlag{Name: "output,o,f", Usage: `set output filename`},
	},
	Action: func(c *cli.Context) error {
		var buf bytes.Buffer

		var typeStrs []string

		switch {
		case c.Bool("is-vuln"):
			typeStrs = append(typeStrs, "vuln")
		case c.Bool("audit-suggestion"):
			typeStrs = append(typeStrs, "audit")
		case c.Bool("security-config"):
			typeStrs = append(typeStrs, "sec-config")
		}

		if len(typeStrs) <= 0 {
			typeStrs = append(typeStrs, "audit")
		}

		buf.WriteString("desc(\n  ")
		buf.WriteString("title: 'checking []',\n  ")
		buf.WriteString("type: " + strings.Join(typeStrs, "|") + "\n)\n\n")
		buf.WriteString("// write your SyntaxFlow Rule, like:\n")
		buf.WriteString("//     DocumentBuilderFactory.newInstance()...parse(* #-> * as $source) as $sink; // find some call chain for parse\n")
		buf.WriteString("//     check $sink then 'find sink point' else 'No Found' // if not found sink, the rule will stop here and report error\n")
		buf.WriteString("//     alert $source // record $source\n\n\n")
		buf.WriteString("// the template is generate by yak.ssa.syntaxflow command line\n")

		filename := c.String("output")

		if l := c.String("language"); filename != "" && l != "" {
			l = strings.TrimSpace(strings.ToLower(l))
			dirname, filename := filepath.Split(filename)
			if !strings.HasPrefix(filename, l+"-") {
				filename = l + "-" + filename
			}
			filename = filepath.Join(dirname, filename)
		}

		if filename == "" {
			fmt.Println(buf.String())
			return nil
		}
		if !strings.HasSuffix(filename, ".sf") {
			filename += ".sf"
		}
		return os.WriteFile(filename, buf.Bytes(), 0o666)
	},
}

var syntaxFlowTest = &cli.Command{
	Name:    "syntaxflow-test",
	Aliases: []string{"sftest", "sf-test"},
	Action: func(c *cli.Context) error {
		testingTInstance := utils.NewRequireTestT(func(msg string, args ...any) {
			log.Errorf(msg, args...)
		}, func() {})

		fsi := filesys.NewLocalFs()

		checkViaPath := func(pathRaw string) error {
			err := filesys.Recursive(pathRaw, filesys.WithFileSystem(fsi), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
				if fsi.Ext(s) == ".sf" || fsi.Ext(s) == "sf" {
					raw, err := fsi.ReadFile(s)
					if err != nil {
						return err
					}
					err = ssatest.EvaluateVerifyFilesystem(string(raw), testingTInstance)
					if err != nil {
						return err
					}
					return nil
				}
				return nil
			}))
			if err != nil {
				return err
			}
			return nil
		}

		empty := len(c.Args()) <= 0
		if empty {
			return checkViaPath(".")
		}
		for _, i := range c.Args() {
			if utils.IsDir(i) {
				err := checkViaPath(i)
				if err != nil {
					return err
				}
			}
			if ret := utils.GetFirstExistedFile(i); ret != "" {
				raw, err := fsi.ReadFile(ret)
				if err != nil {
					return err
				}
				err = ssatest.EvaluateVerifyFilesystem(string(raw), testingTInstance)
				if err != nil {
					return err
				}
			}

		}
		return nil
	},
}

var syntaxFlowExport = &cli.Command{
	Name:    "syntaxflow-export",
	Aliases: []string{"sf-export", "esf"},
	Usage:   "export SyntaxFlow rule to file system",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "output,o",
			Usage: "output file path",
		},
	},
	Action: func(c *cli.Context) error {
		if c.String("output") == "" {
			return utils.Error("output file is required")
		}
		local := filesys.NewLocalFs()
		results, _ := io.ReadAll(sfdb.ExportDatabase())
		if len(results) <= 0 {
			return utils.Error("no rule found")
		}
		return local.WriteFile(c.String("output"), results, 0o666)
	},
}

var syntaxFlowImport = &cli.Command{
	Name:    "syntaxflow-import",
	Usage:   "import SyntaxFlow rule from file system",
	Aliases: []string{"sf-import", "isf"},
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "file,f,i",
			Usage: "file path",
		},
	},
	Action: func(c *cli.Context) error {
		if c.String("file") == "" {
			return utils.Error("file is required")
		}
		file, err := os.Open(c.String("file"))
		if err != nil {
			return utils.Wrap(err, "open file failed")
		}
		defer file.Close()
		err = sfdb.ImportDatabase(file)
		if err != nil {
			return err
		}
		return nil
	},
}

var syncRule = &cli.Command{
	Name:  "sync-rule",
	Usage: "sync rule from embed to database",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "output,o",
			Usage: "output rule info file path",
		},
	},
	Action: func(c *cli.Context) error {
		SyncEmbedRule(true)
		if output := c.String("output"); output != "" {
			log.Infof("output rule info to %s", output)
			log.Infof("start to parse rule info")
			ruleInfos := sfdb.EmbedRuleVersion()
			jsonData, err := json.MarshalIndent(ruleInfos, "", "  ")
			if err != nil {
				log.Infof("Error marshaling ruleInfos: %v", err)
				return err
			}
			os.WriteFile(output, jsonData, 0o666)
			log.Infof("output rule info to %s done ", output)
		}

		return nil
	},
}

var syntaxFlowSave = &cli.Command{
	Name:    "syntaxflow-save",
	Aliases: []string{"save-syntaxflow", "ssf", "sfs"},
	Usage:   "save SyntaxFlow rule to database",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "filesystem,f",
			Usage: "file system for MVP",
		},
		cli.StringFlag{
			Name: "rule,r",
		},
	},
	Action: func(c *cli.Context) error {
		count := 0
		err := filesys.Recursive(c.String("filesystem"), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
			count++
			if count > 50 {
				return utils.Error("too many files")
			}
			// size > 2M will be ignored
			if info.Size() > 2*1024*1024 {
				return utils.Errorf("file %v size too large", s)
			}
			return nil
		}))
		if err != nil {
			return utils.Wrap(err, "read mvp file system failed")
		}

		memfs := filesys.NewVirtualFs()
		local := filesys.NewLocalFs()
		err = filesys.Recursive(c.String("filesystem"), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
			raw, err := local.ReadFile(s)
			if err != nil {
				return nil
			}
			memfs.AddFile(s, string(raw))
			return nil
		}))
		if err != nil {
			return err
		}

		contentRaw, _ := local.ReadFile(c.String("rule"))
		if len(contentRaw) > 0 {
			err = sfdb.ImportValidRule(memfs, c.String("rule"), string(contentRaw))
			if err != nil {
				log.Warnf("import rule failed: %v", err)
			}
			return nil
		}

		entrys, err := utils.ReadDir(c.String("rule"))
		if err != nil {
			return err
		}
		for _, entry := range entrys {
			contentRaw, _ := local.ReadFile(entry.Path)
			if len(contentRaw) <= 0 {
				continue
			}
			err = sfdb.ImportValidRule(memfs, entry.Path, string(contentRaw))
			if err != nil {
				log.Warnf("import rule failed: %v", err)
				continue
			}
		}
		return nil
	},
}

var ssaRisk = &cli.Command{
	Name:    "ssa-risk",
	Aliases: []string{"ssa-risk", "sr"},
	Usage:   "output risk information from the database",
	Flags: []cli.Flag{

		// Input {{{
		// program name
		cli.StringFlag{
			Name:  "program,p",
			Usage: "program name for ssa compiler in db",
		},
		// }}}

		// output {{{
		cli.StringFlag{
			Name:  "output,o",
			Usage: "output file, use --format set output file format, default is sarif",
		},
		// TODO: json format
		// cli.StringFlag{
		// 	Name:  "format",
		// 	Usage: "output file format, set with json or sarif(default)",
		// },
		// }}}

		// {{ risk
		// TODO: risk query filter
		// cli.StringFlag{
		// 	Name:  "log",
		// 	Usage: "log level",
		// },
		cli.StringSliceFlag{
			Name:  "task-id,tid",
			Usage: "task id for ssa compiler in db",
		},
		// }}
	},
	Action: func(c *cli.Context) (e error) {
		defer func() {
			log.Infof("code scan  done")
			if err := recover(); err != nil {
				log.Errorf("code scan failed: %s", err)
				utils.PrintCurrentGoroutineRuntimeStack()
				log.Infof("please use yak `ssa-risk` can export rest result")
				e = utils.Errorf("code scan failed: %s", err)
			}
		}()
		ctx := context.Background()

		// Parse configuration
		config, err := parseSFScanConfig(c)
		if err != nil {
			log.Errorf("parse config failed: %s", err)
			return err
		}
		// Ensure the file is closed after we're done
		defer config.DeferFunc()

		taskIds := c.StringSlice("task-id")

		prog, err := getProgram(ctx, config)
		if err != nil {
			log.Errorf("get program failed: %s", err)
			return err
		}

		ShowResult(config.Format, &ypb.SyntaxFlowResultFilter{
			ProgramNames: []string{prog.GetProgramName()},
			TaskIDs:      taskIds,
			OnlyRisk:     true,
		}, config.OutputWriter)
		return nil

	},
}

var ssaCodeScan = &cli.Command{
	Name:    "code-scan",
	Aliases: []string{"codescan,sfscan"},
	Flags: []cli.Flag{
		// Input {{{
		// program name
		cli.StringFlag{
			Name:  "program,p",
			Usage: "program name for ssa compiler in db",
		},
		// target path
		cli.StringFlag{
			Name:  "target,t",
			Usage: "target path for ssa compiler",
		},

		cli.StringFlag{
			Name:  "language,l",
			Usage: "language for ssa compiler",
		},

		// }}}

		// result show option
		// cli.BoolFlag{
		// 	Name:  "code,show-code",
		// 	Usage: "show code",
		// },

		// Rule {{{

		// rule filter
		cli.StringFlag{
			Name:  "rule-keyword,rk,kw",
			Usage: `set rule keyword for filter`,
		},

		// rule input // todo
		// cli.StringFlag{
		// 	Name:  "rule-dir,rdir",
		// 	Usage: `set rule dir for file`,
		// },
		// }}}

		// output {{{
		cli.StringFlag{
			Name: "output,o",
			// Usage: "output file, use --format set output file format, default is sarif",
			Usage: "output file, default format is sarif",
		},
		cli.StringFlag{
			Name:  "format",
			Usage: "output file format, set with irify,irify-full or sarif(default)",
		},
		// }}}
	},
	Action: func(c *cli.Context) (e error) {
		defer func() {
			log.Infof("code scan  done")
			if err := recover(); err != nil {
				log.Errorf("code scan failed: %s", err)
				utils.PrintCurrentGoroutineRuntimeStack()
				log.Infof("please use yak `ssa-risk` can export rest result")
				e = utils.Errorf("code scan failed: %s", err)
			}
		}()
		ctx := context.Background()
		log.Infof("============= start to scan code ==============")
		ruleTimeStart := time.Now()
		SyncEmbedRule()
		ruleTime := time.Now().Sub(ruleTimeStart)
		_ = ruleTime
		log.Infof("sync rule from embed to database success, cost %v", ruleTime)

		// Parse configuration
		config, err := parseSFScanConfig(c)
		if err != nil {
			log.Errorf("parse config failed: %s", err)
			return err
		}
		// Ensure the file is closed after we're done
		defer config.DeferFunc()

		compileTimeStart := time.Now()
		prog, err := getProgram(ctx, config)
		if err != nil {
			log.Errorf("get program failed: %s", err)
			return err
		}
		compileTime := time.Now().Sub(compileTimeStart)
		log.Infof("get or parse rule success, cost %v", compileTime)

		log.Infof("================= get or parse rule ================")
		scanTimeStart := time.Now()
		ruleFilter := &ypb.SyntaxFlowRuleFilter{
			Language:          []string{prog.GetLanguage()},
			FilterLibRuleKind: yakit.FilterLibRuleFalse,
		}
		ruleFilter.Keyword = c.String("rule-keyword")

		taskId, err := scan(ctx, prog.GetProgramName(), ruleFilter)
		if err != nil {
			log.Errorf("scan failed: %s", err)
			log.Infof("you can use `yak ssa-risk -p %s --task-id \"%s\" -o xxx`", prog.GetProgramName(), taskId)
			return err
		}
		scanTime := time.Now().Sub(scanTimeStart)
		log.Infof("scan success, task id: %s with program: %s, cost %v", taskId, prog.GetProgramName(), scanTime)

		exportTimeStart := time.Now()
		ShowResult(config.Format, &ypb.SyntaxFlowResultFilter{
			TaskIDs:  []string{taskId},
			OnlyRisk: true,
		}, config.OutputWriter)
		exportTime := time.Now().Sub(exportTimeStart)
		log.Infof("show result success, cost %v", exportTime)
		// show echo  time
		log.Infof("finish all time cost:")
		log.Infof("rule sync: %v", ruleTime)
		log.Infof("compile: %v", compileTime)
		log.Infof("scan: %v", scanTime)
		log.Infof("export: %v", exportTime)
		return nil
	},
}

var ssaQuery = &cli.Command{
	Name:    "ssa-query",
	Aliases: []string{"sf", "syntaxFlow"},
	Usage:   "Use SyntaxFlow query SSA OpCodes from database",
	Flags: []cli.Flag{
		cli.StringFlag{Name: "log", Usage: "log level"},
		cli.StringFlag{
			Name:  "program,p",
			Usage: `program name to save in database`,
		},
		cli.StringFlag{
			Name:  "syntaxflow,sf",
			Usage: "syntax flow query language code",
		},
		cli.StringFlag{
			Name:  "database,db",
			Usage: "database path",
		},
		cli.StringFlag{
			Name:  "database-dialect,db-dialect",
			Usage: "database dialect for gorm, support: mysql, sqlite3(default)",
		},
		cli.BoolFlag{
			Name:  "database-debug,dbdebug",
			Usage: "enable database debug mode",
		},
		cli.BoolFlag{
			Name:  "syntaxflow-debug,sfdebug",
			Usage: "enable syntax flow debug mode",
		},
		cli.BoolFlag{
			Name: "dot", Usage: "dot graph text for result",
		},
		cli.BoolFlag{
			Name: "with-code,code", Usage: "show code context",
		},
		cli.StringFlag{
			Name: "sarif,sarif-export,o", Usage: "export SARIF format to files",
		},
		cli.BoolFlag{
			Name: "save,s", Usage: "save the risk to the database",
		},
	},
	Action: func(c *cli.Context) error {
		if ret, err := log.ParseLevel(c.String("log")); err == nil {
			log.SetLevel(ret)
		}
		programName := c.String("program")
		databaseFileRaw := c.String("database")
		databaseDialect := c.String("database-dialect")
		dbDebug := c.Bool("database-debug")
		sfDebug := c.Bool("syntaxflow-debug")
		syntaxFlow := c.String("syntaxflow")
		showDot := c.Bool("dot")
		withCode := c.Bool("with-code")
		saverisk := c.Bool("save")

		// set database
		if databaseDialect != "" {
			// if set dialect, open gorm and set db
			if databaseFileRaw == "" {
				return utils.Errorf("database path is required when using database dialect")
			}
			db, err := gorm.Open(databaseDialect, databaseFileRaw)
			if err != nil {
				return utils.Errorf("open database failed: %v", err)
			}
			consts.SetGormSSAProjectDatabaseByDB(db)
		} else if databaseFileRaw != "" {
			// set database path
			if utils.GetFirstExistedFile(databaseFileRaw) == "" {
				// no compile ,database not existed
				return utils.Errorf("database file not found: %v use default database", databaseFileRaw)
			}
			consts.SetSSAProjectDatabasePath(databaseFileRaw)
		}

		sarifFile := c.String("sarif")
		if sarifFile != "" {
			if filepath.Ext(sarifFile) != ".sarif" {
				sarifFile += ".sarif"
			}
		}

		haveSarifRequired := false
		if sarifFile != "" {
			haveSarifRequired = true
		}
		var results []*ssaapi.SyntaxFlowResult
		var sarifCallback func(result *ssaapi.SyntaxFlowResult)
		if haveSarifRequired {
			sarifCallback = func(result *ssaapi.SyntaxFlowResult) {
				results = append(results, result)
			}
		} else {
			sarifCallback = func(result *ssaapi.SyntaxFlowResult) {

			}
		}

		defer func() {
			if len(results) > 0 && sarifFile != "" {
				log.Infof("fetch result: %v, exports sarif to %v", len(results), sarifFile)
				sarifReport, err := sfreport.ConvertSyntaxFlowResultsToSarif(results...)
				if err != nil {
					log.Errorf("convert SARIF failed: %v", err)
					return
				}
				if utils.GetFirstExistedFile(sarifFile) != "" {
					backup := sarifFile + ".bak"
					os.Rename(sarifFile, backup)
					os.RemoveAll(sarifFile)
				}
				err = sarifReport.WriteFile(sarifFile)
				if err != nil {
					log.Errorf("write SARIF failed: %v", err)
				}
			}
		}()

		if syntaxFlow != "" {
			return SyntaxFlowQuery(programName, syntaxFlow, dbDebug, sfDebug, showDot, withCode, sarifCallback)
		}

		var dirChecking []string
		handleBySyntaxFlowContent := func(syntaxFlow string) error {
			err := SyntaxFlowQuery(programName, syntaxFlow, dbDebug, sfDebug, showDot, withCode, sarifCallback)
			if err != nil {
				return err
			}
			fmt.Println()
			return nil
		}

		handleByFilename := func(filename string) error {
			log.Infof("start to use SyntaxFlow rule: %v", filename)
			raw, err := os.ReadFile(filename)
			if err != nil {
				return utils.Wrapf(err, "read %v failed", filename)
			}
			return handleBySyntaxFlowContent(string(raw))
		}

		sarifCallback = func(result *ssaapi.SyntaxFlowResult) {
			if result.IsLib() {
				return
			}
			values := result.GetAlertValues()
			if values.Len() != 0 {
				result.Show()
				if saverisk {
					result.Save(schema.SFResultKindQuery)
				}
			}
		}

		var errs []error
		var cmdArgs []string = c.Args()
		for _, originName := range cmdArgs {
			name := utils.GetFirstExistedFile(originName)
			if name == "" {
				infos, _ := utils.ReadDir(originName)
				if len(infos) > 0 {
					dirChecking = append(dirChecking, originName)
					continue
				}

				if filepath.IsAbs(originName) {
					log.Warnf("cannot find rule as %v", originName)
				} else {
					absName, _ := filepath.Abs(originName)
					if absName != "" {
						log.Warnf("cannot find rule as %v(abs: %v)", originName, absName)
					} else {
						log.Warnf("cannot find rule as %v", originName)
					}
				}
				continue
			}
			err := handleByFilename(name)
			if err != nil {
				errs = append(errs, err)
			}
		}

		for _, dir := range dirChecking {
			log.Infof("start to read directory: %v", dir)
			err := filesys.Recursive(dir, filesys.WithRecursiveDirectory(true), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
				fileExt := strings.ToLower(filepath.Ext(s))
				if strings.HasSuffix(fileExt, ".sf") {
					err := handleByFilename(s)
					if err != nil {
						errs = append(errs, err)
					}
				}
				return nil
			}))
			if err != nil {
				log.Warnf("read directory [%v] failed: %v", dir, err)
			}
		}

		if len(cmdArgs) <= 0 {
			prog, err := ssaapi.FromDatabase(programName)
			if err != nil {
				log.Errorf("load program [%v] from database failed: %v", programName, err)
				return err
			}

			// use database
			db := consts.GetGormProfileDatabase()
			expected := []string{""}
			for _, l := range utils.PrettifyListFromStringSplitEx(prog.GetLanguage(), ",") {
				if l == "" {
					continue
				}
				expected = append(expected, l)
			}
			db = bizhelper.ExactQueryStringArrayOr(db, "language", expected)
			for result := range sfdb.YieldSyntaxFlowRules(db, context.Background()) {
				err := handleBySyntaxFlowContent(result.Content)
				if err != nil {
					errs = append(errs, err)
				}
			}
		}

		if len(errs) > 0 {
			var buf bytes.Buffer
			for i, e := range errs {
				buf.WriteString("  ")
				buf.WriteString(fmt.Sprintf("%-2d: ", i+1))
				buf.WriteString(e.Error())
				buf.WriteByte('\n')
			}
			return utils.Errorf("many error happened: \n%v", buf.String())
		}

		return nil
	},
}

var SSACompilerCommands = []*cli.Command{
	// program manage
	ssaRemove,  // remove program from database
	ssaCompile, // compile program

	// rule manage
	syntaxFlowCreate, // create rule template
	syntaxFlowSave,   // save rule to database
	syntaxFlowTest,   // test rule
	syntaxFlowExport, // export rule to file
	syntaxFlowImport, // import rule from file
	syncRule,         // sync rule from embed to database
	// risk manage
	ssaRisk, // export risk report

	staticCheck,
	ssaQuery, // rule scan target from database

	// all in one
	ssaCodeScan, // compile and scan and export report
}

// SyntaxFlowQuery 函数用于执行语法流查询
func SyntaxFlowQuery(
	programName string, // 程序名称
	syntaxFlow string, // 语法流
	dbDebug, sfDebug, showDot, withCode bool, // 是否开启数据库调试、语法流调试、显示dot、显示代码
	callbacks ...func(*ssaapi.SyntaxFlowResult), // 回调函数
) error {
	// 捕获panic
	defer func() {
		if err := recover(); err != nil {
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	// 检查程序名称是否为空
	if programName == "" {
		return utils.Error("program name is required when using syntax flow query language")
	}
	// program from database
	prog, err := ssaapi.FromDatabase(programName)
	if err != nil {
		log.Errorf("load program [%v] from database failed: %v", programName, err)
	}
	if dbDebug {
		prog.DBDebug()
	}
	opt := make([]ssaapi.QueryOption, 0)
	if sfDebug {
		opt = append(opt, ssaapi.QueryWithEnableDebug())
	}
	var execError error
	result, err := prog.SyntaxFlowWithError(syntaxFlow, opt...)
	if err != nil {
		var otherErrs []string
		if result != nil && len(result.GetErrors()) > 0 {
			otherErrs = utils.StringArrayFilterEmpty(utils.RemoveRepeatStringSlice(result.GetErrors()))
		}
		execError = utils.Wrapf(err, "prompt error: \n%v", strings.Join(otherErrs, "\n  "))
	}
	if result == nil {
		return execError
	}

	if result != nil {
		for _, c := range callbacks {
			c(result)
		}
	}

	log.Infof("syntax flow query result:")
	result.Show(
		sfvm.WithShowAll(sfDebug),
		sfvm.WithShowCode(withCode),
		sfvm.WithShowDot(showDot),
	)
	return execError
}
