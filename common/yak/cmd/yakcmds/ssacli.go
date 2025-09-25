package yakcmds

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"

	"github.com/gobwas/glob"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"io"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/syntaxflow/sfcompletion"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"golang.org/x/exp/slices"

	"github.com/segmentio/ksuid"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfanalyzer"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssaprofile"
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
			consts.SetGormSSAProjectDatabase(db)
		}
		// if not set dialect, use existed db
		if databaseDialect == "" && databaseFileRaw != "" {
			// set database path
			// if target == "" &&
			// 	utils.GetFirstExistedFile(databaseFileRaw) == "" {
			// 	// no compile ,database not existed
			// 	return utils.Errorf("database file not found: %v", databaseFileRaw)
			// }
			consts.SetSSADatabaseInfo(databaseFileRaw)
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
			Name:  "file,f,input,i",
			Usage: "file path",
		},
		cli.StringFlag{
			Name: "format",
			Usage: `input file format:
	* json (default: json file)
	* raw (syntaxflow file)
		`,
		},
	},
	Action: func(c *cli.Context) error {
		file := c.String("file")
		format := c.String("format")

		if file == "" {
			return utils.Error("file is required")
		}
		if format == "" {
			format = "json"
		}
		path, err := os.Open(file)
		if err != nil {
			return utils.Wrap(err, "open file failed")
		}
		defer path.Close()

		switch format {
		case "json":
			err = sfdb.ImportDatabase(path)
			if err != nil {
				return err
			}
		case "raw":
			rfs := filesys.NewRelLocalFs(file)
			err := sfbuildin.SyncBuildRuleByFileSystem(rfs, false, func(process float64, ruleName string) {
				log.Infof("sync input rule: %s, process: %f", ruleName, process)
			})
			if err != nil {
				return err
			}
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

var syntaxflowFormat = &cli.Command{
	Name:    "syntaxflow-format",
	Aliases: []string{"sf-format", "sf-fmt"},
	Usage:   "format SyntaxFlow rule",
	Flags:   []cli.Flag{},
	Action: func(c *cli.Context) error {
		if len(c.Args()) == 0 {
			log.Errorf("syntaxflow-format: no file provided")
		}

		var errors error
		format := func(fileName string) error {
			// Check if the file has .sf extension
			if !strings.HasSuffix(fileName, ".sf") {
				log.Infof("syntaxflow-format: skipping file %s (not a .sf file)", fileName)
				return nil
			}
			raw, err := os.ReadFile(fileName)
			if err != nil {
				log.Errorf("failed to read file %s: %v", fileName, err)
				return err
			}
			rule, err := sfvm.FormatRule(string(raw))
			if err != nil {
				err = utils.Errorf("failed parse format file %s: %v", fileName, err)
				log.Errorf("%v", err)
				errors = utils.JoinErrors(errors, err)
				return err
			}

			// check format rule
			if _, err := sfvm.CompileRule(rule); err != nil {
				err = utils.Errorf("failed check format file %s: %v\nformat rule: \n%s", fileName, err, rule)
				log.Errorf("%v", err)
				errors = utils.JoinErrors(errors, err)
				return err
			}

			err = os.WriteFile(fileName, []byte(rule), 0o666)
			if err != nil {
				log.Errorf("failed to write file %s: %v", fileName, err)
				return err
			}
			return nil
		}

		for _, path := range c.Args() {
			if utils.IsFile(path) {
				log.Infof("syntaxflow-format: processing file %s", path)
				format(path)
			} else if utils.IsDir(path) {
				log.Infof("syntaxflow-format: processing directory %s", path)
				filesys.Recursive(path, filesys.WithFileSystem(filesys.NewLocalFs()), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
					log.Infof("syntaxflow-format: processing file %s", s)
					return format(s)
				}))
			} else {
				log.Errorf("syntaxflow-format: file %s not found", path)
			}
		}
		return errors
	},
}

var syntaxflowCompletion = &cli.Command{
	Name:    "syntaxflowCompletion",
	Aliases: []string{"sf-complete", "sf-completions"},
	Usage:   "SyntaxFlow Rule Description Completion By AI",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "ai-type,type",
			Usage: "type of AI type",
		},
		cli.StringFlag{
			Name:  "target,t",
			Usage: "the file or directory to process, if it is a directory, all .sf files will be processed",
		},
		cli.StringFlag{
			Name:  "api-key,key,k",
			Usage: "api key of AI",
		},
		cli.StringFlag{
			Name:  "ai-model,model,m",
			Usage: "model of AI",
		},
		cli.StringFlag{
			Name:  "proxy,p",
			Usage: "proxy of AI",
		},
		cli.StringFlag{
			Name:  "domain,ai-domain",
			Usage: "domain of ai",
		},
		cli.StringFlag{
			Name:  "baseUrl,url",
			Usage: "baseUrl of ai",
		},
		cli.StringSliceFlag{
			Name:  "files,fs",
			Usage: "files to process, if it is a directory, all .sf files will be processed",
		},
		cli.IntFlag{
			Name:  "concurrency,c",
			Usage: "concurrency of AI completion, default is 5",
		},
		cli.DurationFlag{
			Name:  "skip-recent,recent",
			Usage: "skip files modified within this duration (e.g., 30m, 1h, 2h30m), default is 0 (no skip)",
		},
	},
	Action: func(c *cli.Context) error {
		target := c.String("target")
		typ := c.String("ai-type")
		key := c.String("api-key")
		model := c.String("ai-model")
		proxy := c.String("proxy")
		concurrency := c.Int("concurrency")
		domain := c.String("domain")
		baseUrl := c.String("url")
		files := c.StringSlice("fs")
		skipRecent := c.Duration("skip-recent")
		if concurrency == 0 {
			concurrency = 5 // default concurrency
		}

		var aiOpts []aispec.AIConfigOption
		if model != "" {
			aiOpts = append(aiOpts, aispec.WithModel(model))
		}
		if domain != "" {
			aiOpts = append(aiOpts, aispec.WithDomain(domain))
		}
		if typ != "" {
			aiOpts = append(aiOpts, aispec.WithType(typ))
		}
		if key != "" {
			aiOpts = append(aiOpts, aispec.WithAPIKey(key))
		}
		if proxy != "" {
			aiOpts = append(aiOpts, aispec.WithProxy(proxy))
		}
		if baseUrl != "" {
			aiOpts = append(aiOpts, aispec.WithBaseURL(baseUrl))
		}

		swg := new(sync.WaitGroup)
		errChan := make(chan error, 1)
		taskChannel := make(chan string, 1)
		var errors error
		errorDone := make(chan struct{}, 1)
		go func() {
			for err := range errChan {
				errors = utils.JoinErrors(errors, err)
			}
			errorDone <- struct{}{}
			close(errorDone)
		}()
		var taskCount atomic.Int64
		var processedCount atomic.Int64 // 成功处理的文件数
		var skippedCount atomic.Int64   // 跳过的文件数
		var errorCount atomic.Int64     // 处理失败的文件数
		for i := 0; i < concurrency; i++ {
			swg.Add(1)
			go func() {
				defer swg.Done()
				for fileName := range taskChannel {
					if !strings.HasSuffix(fileName, ".sf") {
						log.Infof("syntaxflow-completion: skipping file %s (not a .sf file)", fileName)
						continue
					}
					raw, err := os.ReadFile(fileName)
					if err != nil {
						errorCount.Add(1)
						log.Errorf("failed to read file %s: %v", fileName, err)
						continue
					}
					rule, err := sfcompletion.CompleteRuleDesc(fileName, string(raw), aiOpts...)
					if err != nil {
						errorCount.Add(1)
						err = utils.Errorf("failed parse complete file %s: %v", fileName, err)
						errChan <- utils.JoinErrors(err, err)
						log.Errorf("%v", err)
						continue
					}
					// check format rule
					if _, err := sfvm.CompileRule(rule); err != nil {
						errorCount.Add(1)
						err = utils.Errorf("failed completion sf rule %s: %v\nsf rule: \n%s", fileName, err, rule)
						errChan <- utils.JoinErrors(err, err)
						log.Errorf("%v", err)
						continue
					}
					err = os.WriteFile(fileName, []byte(rule), 0o666)
					if err != nil {
						errorCount.Add(1)
						log.Errorf("failed to write file %s: %v", fileName, err)
						errChan <- utils.Errorf("failed to write file %s: %v", fileName, err)
						continue
					}
					processedCount.Add(1)
					sleepTime := rand.Intn(5)
					log.Infof("syntaxflow-completion: completed file %s, sleep for %d seconds", fileName, sleepTime)
					time.Sleep(time.Second * time.Duration(sleepTime))
				}
			}()
		}
		// 检查文件是否应该被跳过（最近修改过）
		shouldSkipFile := func(filePath string) bool {
			if skipRecent == 0 {
				return false // 如果没有设置跳过时间，不跳过任何文件
			}

			fileInfo, err := os.Stat(filePath)
			if err != nil {
				log.Errorf("syntaxflow-completion: failed to get file info for %s: %v", filePath, err)
				return false // 如果无法获取文件信息，不跳过
			}

			modTime := fileInfo.ModTime()
			timeSinceModification := time.Since(modTime)

			if timeSinceModification < skipRecent {
				skippedCount.Add(1)
				log.Infof("syntaxflow-completion: skipping file %s (modified %v ago, within %v threshold)",
					filePath, timeSinceModification.Round(time.Second), skipRecent)
				return true
			}

			return false
		}

		addTask := func(target string) {
			if utils.IsFile(target) {
				if shouldSkipFile(target) {
					return
				}
				log.Infof("syntaxflow-completion: processing file %s", target)
				taskChannel <- target
				taskCount.Add(1)
			} else if utils.IsDir(target) {
				log.Infof("syntaxflow-completion: processing directory %s", target)
				filesys.Recursive(target, filesys.WithFileSystem(filesys.NewLocalFs()), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
					if shouldSkipFile(s) {
						return nil
					}
					log.Infof("syntaxflow-completion: processing file %s", s)
					taskChannel <- s
					taskCount.Add(1)
					return nil
				}))
			} else {
				log.Errorf("syntaxflow-completion: file %s not found", target)
			}
		}

		if target != "" {
			addTask(target)
		}
		for _, f := range files {
			addTask(f)
		}
		close(taskChannel)
		swg.Wait()
		close(errChan)
		<-errorDone

		// 打印统计报告
		totalFiles := processedCount.Load() + skippedCount.Load() + errorCount.Load()
		log.Infof("==================== SyntaxFlow Completion Report ====================")
		log.Infof("Total files found: %d", totalFiles)
		log.Infof("Successfully processed: %d", processedCount.Load())
		log.Infof("Skipped (recently modified): %d", skippedCount.Load())
		log.Infof("Failed with errors: %d", errorCount.Load())
		if skipRecent > 0 {
			log.Infof("Skip threshold: files modified within %v", skipRecent)
		}
		log.Infof("=====================================================================")

		return errors
	},
}

var syntaxflowTestCasesCompletion = &cli.Command{
	Name:    "syntaxflow-test-cases-completion",
	Aliases: []string{"sf-test-cases", "sf-tc"},
	Usage:   "SyntaxFlow Rule Test Cases Completion By AI (Both Positive and Negative)",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "ai-type,type",
			Usage: "type of AI type",
		},
		cli.StringFlag{
			Name:  "target,t",
			Usage: "the file or directory to process, if it is a directory, all .sf files will be processed",
		},
		cli.StringFlag{
			Name:  "api-key,key,k",
			Usage: "api key of AI",
		},
		cli.StringFlag{
			Name:  "ai-model,model,m",
			Usage: "model of AI",
		},
		cli.StringFlag{
			Name:  "proxy,p",
			Usage: "proxy of AI",
		},
		cli.StringFlag{
			Name:  "domain,ai-domain",
			Usage: "domain of ai",
		},
		cli.StringFlag{
			Name:  "baseUrl,url",
			Usage: "baseUrl of ai",
		},
		cli.StringSliceFlag{
			Name:  "files,fs",
			Usage: "files to process, if it is a directory, all .sf files will be processed",
		},
		cli.IntFlag{
			Name:  "concurrency,c",
			Usage: "concurrency of AI completion, default is 3",
		},
	},
	Action: func(c *cli.Context) error {
		target := c.String("target")
		typ := c.String("ai-type")
		key := c.String("api-key")
		model := c.String("ai-model")
		proxy := c.String("proxy")
		concurrency := c.Int("concurrency")
		domain := c.String("domain")
		baseUrl := c.String("url")
		files := c.StringSlice("fs")
		if concurrency == 0 {
			concurrency = 1 // default concurrency, lower than desc completion
		}

		var aiOpts []aispec.AIConfigOption
		if model != "" {
			aiOpts = append(aiOpts, aispec.WithModel(model))
		}
		if domain != "" {
			aiOpts = append(aiOpts, aispec.WithDomain(domain))
		}
		if typ != "" {
			aiOpts = append(aiOpts, aispec.WithType(typ))
		}
		if key != "" {
			aiOpts = append(aiOpts, aispec.WithAPIKey(key))
		}
		if proxy != "" {
			aiOpts = append(aiOpts, aispec.WithProxy(proxy))
		}
		if baseUrl != "" {
			aiOpts = append(aiOpts, aispec.WithBaseURL(baseUrl))
		}

		swg := new(sync.WaitGroup)
		errChan := make(chan error, 1)
		taskChannel := make(chan string, 1)
		var errors error
		errorDone := make(chan struct{}, 1)
		go func() {
			for err := range errChan {
				errors = utils.JoinErrors(errors, err)
			}
			errorDone <- struct{}{}
			close(errorDone)
		}()
		var taskCount atomic.Int64
		for i := 0; i < concurrency; i++ {
			swg.Add(1)
			go func() {
				defer swg.Done()
				for fileName := range taskChannel {
					if !strings.HasSuffix(fileName, ".sf") {
						log.Infof("syntaxflow-test-cases-completion: skipping file %s (not a .sf file)", fileName)
						continue
					}
					raw, err := os.ReadFile(fileName)
					if err != nil {
						log.Errorf("failed to read file %s: %v", fileName, err)
						continue
					}
					rule, err := sfcompletion.CompleteTestCases(fileName, string(raw), aiOpts...)
					if err != nil {
						err = utils.Errorf("failed to complete test cases for file %s: %v", fileName, err)
						errChan <- err
						log.Errorf("%v", err)
						continue
					}
					// check format rule
					if _, err := sfvm.CompileRule(rule); err != nil {
						err = utils.Errorf("failed to validate completed rule for file %s: %v\nrule content: \n%s", fileName, err, rule)
						errChan <- err
						log.Errorf("%v", err)
						continue
					}

					err = os.WriteFile(fileName, []byte(rule), 0o666)
					if err != nil {
						log.Errorf("failed to write file %s: %v", fileName, err)
						errChan <- utils.Errorf("failed to write file %s: %v", fileName, err)
						continue
					}
					sleepTime := rand.Intn(3) + 2 // 2-4 seconds sleep
					log.Infof("syntaxflow-test-cases-completion: completed file %s, sleep for %d seconds", fileName, sleepTime)
					time.Sleep(time.Second * time.Duration(sleepTime))
				}
			}()
		}
		addTask := func(target string) {
			if utils.IsFile(target) {
				log.Infof("syntaxflow-test-cases-completion: processing file %s", target)
				taskChannel <- target
				taskCount.Add(1)
			} else if utils.IsDir(target) {
				log.Infof("syntaxflow-test-cases-completion: processing directory %s", target)
				filesys.Recursive(target, filesys.WithFileSystem(filesys.NewLocalFs()), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
					log.Infof("syntaxflow-test-cases-completion: processing file %s", s)
					taskChannel <- s
					taskCount.Add(1)
					return nil
				}))
			} else {
				log.Errorf("syntaxflow-test-cases-completion: file %s not found", target)
			}
		}

		if target != "" {
			addTask(target)
		}
		for _, f := range files {
			addTask(f)
		}
		close(taskChannel)
		swg.Wait()
		close(errChan)
		<-errorDone
		return errors
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
	Usage:   "visualize and format risk report from JSON file",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "input,i",
			Usage: "risk report JSON file to be imported",
		},
		cli.StringFlag{
			Name:  "program,p",
			Usage: "program name for ssa compiler in db",
		},
		// 	cli.StringFlag{
		// 		Name: "format",
		// 		Usage: `output format:
		// * sarif - SARIF format output
		// * irify - IRify format (truncated file content)
		// * irify-full - IRify format with full file content
		// * irify-react-report - IRify React report format
		// 	`,
		// 	},
		// TODO: 更详细的过滤器可能要和risk filter对接
		cli.StringFlag{
			Name:  "severity",
			Usage: "filter by severity level (critical, high, middle, low)",
		},
		cli.StringFlag{
			Name:  "rule",
			Usage: "filter by rule name (partial match)",
		},
		cli.BoolFlag{
			Name:  "with-code",
			Usage: "include code fragments in output",
		},
	},
	Action: func(c *cli.Context) (e error) {
		input := c.String("input")
		program := c.String("program")

		showAll := func(content []byte) error {
			var report sfreport.Report
			if err := json.Unmarshal(content, &report); err != nil {
				return utils.Wrap(err, "failed to parse JSON file")
			}

			// format := sfreport.ReportTypeFromString(c.String("format"))
			severityFilter := c.String("severity-filter")
			ruleFilter := c.String("rule-filter")
			withCode := c.Bool("with-code")
			return outputConsole(os.Stdout, &report, severityFilter, ruleFilter, withCode)
		}

		if input != "" {
			if !utils.IsFile(input) {
				return utils.Errorf("input file not found: %v", input)
			}

			content, err := os.ReadFile(input)
			if err != nil {
				return utils.Wrap(err, "failed to read input file")
			}
			return showAll(content)
		} else if program != "" {
			config, err := parseSFScanConfig(c)
			if err != nil {
				log.Errorf("parse config failed: %s", err)
				return err
			}
			defer config.DeferFunc()

			ctx := context.Background()
			prog, err := getProgram(ctx, config)
			if err != nil {
				log.Errorf("get program failed: %s", err)
				return err
			}

			ruleFilter := &ypb.SyntaxFlowRuleFilter{
				Language:          []string{prog.GetLanguage()},
				FilterLibRuleKind: yakit.FilterLibRuleFalse,
			}

			var content []byte
			riskCh, err := scan(ctx, prog.GetProgramName(), ruleFilter, true)
			if err != nil {
				log.Errorf("scan failed: %s", err)
				// log.Infof("you can use `yak ssa-risk -p %s --task-id \"%s\" -o xxx`", prog.GetProgramName(), taskId)
				return err
			}
			opt := []sfreport.Option{}
			if c.Bool("with-file-content") {
				opt = append(opt, sfreport.WithFileContent(true))
			}
			if c.Bool("with-dataflow-path") {
				opt = append(opt, sfreport.WithDataflowPath(true))
			}

			// 创建一个缓冲区来捕获ShowRisk的输出
			var buffer bytes.Buffer
			ShowRisk(config.Format, riskCh, &buffer, opt...)
			content = buffer.Bytes()

			return showAll(content)
		}

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

		cli.BoolFlag{
			Name:  "memory,mem",
			Usage: "enable memory mode",
		},
		cli.StringFlag{
			Name:  "database,db",
			Usage: "database path",
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

		// rule group filter
		cli.StringSliceFlag{
			Name:  "rule-group,rg",
			Usage: `set rule group names for filter (can be used multiple times)`,
		},
		// }}}

		// output {{{
		cli.StringFlag{
			Name: "output,o",
			// Usage: "output file, use --format set output file format, default is sarif",
			Usage: "output file, default format is sarif",
		},
		cli.StringFlag{
			Name: "format",
			Usage: `output file format:
	* sarif (default)
	* irify (can config with --with-file-content and --with-dataflow-path)
	* irify-full (with all info)
	* irify-react-report (save database and generate react report in IRify frontend)
		`,
		},

		cli.BoolFlag{
			Name:  "with-file-content",
			Usage: "include full file content in the output (only for irify format)",
		},
		cli.BoolFlag{
			Name:  "with-dataflow-path",
			Usage: "include dataflow path in the output (only for irify format)",
		},
		// }}}

		cli.StringFlag{
			Name:  "pprof",
			Usage: `enable pprof and save pprof file to the given path`,
		},

		cli.StringFlag{
			Name:  "log-level,loglevel",
			Usage: `set log level, default is info, optional value: debug, info, warn, error`,
		},

		cli.StringSliceFlag{
			Name: "exclude-file",
			Usage: `exclude default file,only support glob mode. eg.
					targets/*, vendor/*`,
		},
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

		if pprofFile := c.String("pprof"); pprofFile != "" {
			ssaprofile.DumpHeapProfileWithInterval(30*time.Second, ssaprofile.WithFileName(pprofFile))
		}

		if logLevel := c.String("log-level"); logLevel != "" {
			level, err := log.ParseLevel(logLevel)
			if err != nil {
				log.Warnf("parse log level %s failed: %v, use info level", logLevel, err)
				level = log.InfoLevel
			}
			log.SetLevel(level)
		}

		log.Infof("============= start to scan code ==============")

		ruleTimeStart := time.Now()
		SyncEmbedRule()
		ruleTime := time.Since(ruleTimeStart)
		_ = ruleTime
		log.Infof("sync rule from embed to database success, cost %v", ruleTime)

		if databaseRaw := c.String("database"); databaseRaw != "" {
			consts.SetGormSSAProjectDatabaseByInfo(databaseRaw)
		}
		// Parse configuration
		config, err := parseSFScanConfig(c)
		if err != nil {
			log.Errorf("parse config failed: %s", err)
			return err
		}
		// Ensure the file is closed after we're done
		defer config.DeferFunc()

		// compileTimeStart := time.Now()
		prog, err := getProgram(ctx, config)
		if err != nil {
			log.Errorf("get program failed: %s", err)
			return err
		}
		// compileTime := time.Since(compileTimeStart)s.
		// log.Infof("get or parse rule success, cost %v", compileTime)

		log.Infof("================= get or parse rule ================")
		// scanTimeStart := time.Now()
		ruleFilter := &ypb.SyntaxFlowRuleFilter{
			Language:          []string{prog.GetLanguage()},
			Keyword:           c.String("rule-keyword"),
			FilterLibRuleKind: yakit.FilterLibRuleFalse,
		}

		// Handle rule group filtering
		if groupNames := c.StringSlice("rule-group"); len(groupNames) > 0 {
			ruleFilter.GroupNames = groupNames
		}

		opt := []sfreport.Option{}
		if c.Bool("with-file-content") {
			opt = append(opt, sfreport.WithFileContent(true))
		}
		if c.Bool("with-dataflow-path") {
			opt = append(opt, sfreport.WithDataflowPath(true))
		}
		reportInstance, err := sfreport.ConvertSyntaxFlowResultToReport(config.Format, opt...)
		err = syntaxflow_scan.StartScan(
			ctx,
			nil,
			syntaxflow_scan.WithProgramNames(prog.GetProgramName()),
			syntaxflow_scan.WithRuleFilter(ruleFilter),
			syntaxflow_scan.WithMemory(c.Bool("memory")),
			syntaxflow_scan.WithReporter(reportInstance),
			syntaxflow_scan.WithReporterWriter(config.OutputWriter),
		)
		if err != nil {
			log.Errorf("scan failed: %s", err)
			return err
		}
		ssaprofile.ShowCacheCost()
		return nil
	},
}

var syntaxFlowEvaluate = &cli.Command{
	Name:    "syntaxflow-evaluate",
	Aliases: []string{"sf-evaluate"},
	Usage:   "evaluate SyntaxFlow rule quality and provide score",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "target,t",
			Usage: "target file or directory to evaluate",
		},
		cli.StringFlag{
			Name:  "output,o",
			Usage: "output file for evaluation result (json format)",
		},
		cli.BoolFlag{
			Name:  "verbose,v",
			Usage: "verbose output with detailed problems",
		},
		cli.BoolFlag{
			Name:  "json",
			Usage: "output in JSON format",
		},
	},
	Action: func(c *cli.Context) error {
		target := c.String("target")
		output := c.String("output")
		verbose := c.Bool("verbose")
		jsonOutput := c.Bool("json")

		if target == "" {
			return utils.Error("target file or directory is required")
		}

		// Check if target exists
		if !utils.IsFile(target) && !utils.IsDir(target) {
			return utils.Errorf("target file or directory not found: %v", target)
		}

		results := make(map[string]*sfanalyzer.SyntaxFlowRuleAnalyzeResult)

		// Process single file or directory
		if utils.IsFile(target) {
			if !strings.HasSuffix(target, ".sf") {
				return utils.Error("target file must be a .sf file")
			}

			content, err := os.ReadFile(target)
			if err != nil {
				return utils.Errorf("failed to read file %s: %v", target, err)
			}
			fileName := filepath.Base(target)

			analyzer := sfanalyzer.NewSyntaxFlowAnalyzer(string(content))
			result := analyzer.Analyze()
			results[fileName] = result
		} else {
			// Process directory
			err := filesys.Recursive(target, filesys.WithFileStat(func(s string, info fs.FileInfo) error {
				if !strings.HasSuffix(s, ".sf") {
					return nil
				}

				content, err := os.ReadFile(s)
				if err != nil {
					log.Errorf("failed to read file %s: %v", s, err)
					return nil
				}
				fileName := filepath.Base(s)
				analyzer := sfanalyzer.NewSyntaxFlowAnalyzer(string(content))
				result := analyzer.Analyze()
				results[fileName] = result
				return nil
			}))
			if err != nil {
				return utils.Errorf("failed to process directory: %v", err)
			}
		}

		if len(results) == 0 {
			return utils.Error("no .sf files found to evaluate")
		}

		// Output results
		if jsonOutput || output != "" {
			// JSON output
			jsonData, err := json.MarshalIndent(results, "", "  ")
			if err != nil {
				return utils.Errorf("failed to marshal results to JSON: %v", err)
			}

			if output != "" {
				err = os.WriteFile(output, jsonData, 0o666)
				if err != nil {
					return utils.Errorf("failed to write output file: %v", err)
				}
				log.Infof("Evaluation results written to %s", output)
			} else {
				fmt.Println(string(jsonData))
			}
		} else {
			// Human-readable output
			for fileName, result := range results {
				fmt.Printf("\n=== %s ===\n", fileName)
				fmt.Printf("得分: %d/%d\n", result.Score, result.MaxScore)
				grade := sfanalyzer.GetGrade(result.Score)
				fmt.Printf("等级: %s (%s)\n", grade, sfanalyzer.GetGradeDescription(grade))

				// 始终显示问题概览
				if len(result.Problems) > 0 {
					errorCount := 0
					warningCount := 0
					infoCount := 0

					for _, problem := range result.Problems {
						switch problem.Severity {
						case sfanalyzer.Error:
							errorCount++
						case sfanalyzer.Warning:
							warningCount++
						case sfanalyzer.Info:
							infoCount++
						}
					}

					fmt.Printf("问题概览: ")
					if errorCount > 0 {
						fmt.Printf("%d个错误 ", errorCount)
					}
					if warningCount > 0 {
						fmt.Printf("%d个警告 ", warningCount)
					}
					if infoCount > 0 {
						fmt.Printf("%d个建议 ", infoCount)
					}
					fmt.Printf("\n")

					// 简要显示主要问题
					fmt.Printf("主要问题:\n")
					for i, problem := range result.Problems {
						if i >= 3 && !verbose { // 非详细模式只显示前3个问题
							fmt.Printf("  ... (还有 %d 个问题，使用 -v 查看详情)\n", len(result.Problems)-3)
							break
						}
						fmt.Printf("  • [%s] %s\n", problem.Severity, problem.Description)
					}
				} else {
					fmt.Printf("✓ 未发现问题\n")
				}

				// 详细模式显示完整信息
				if verbose && len(result.Problems) > 0 {
					fmt.Printf("\n详细问题信息:\n")
					for i, problem := range result.Problems {
						fmt.Printf("  %d. [%s] %s\n", i+1, problem.Severity, problem.Description)
						if problem.Suggestion != "" {
							fmt.Printf("     💡 建议: %s\n", problem.Suggestion)
						}
						if problem.Range != nil {
							fmt.Printf("     📍 位置: 第%d行第%d列 - 第%d行第%d列\n",
								problem.Range.StartLine, problem.Range.StartColumn,
								problem.Range.EndLine, problem.Range.EndColumn)
						}
						fmt.Printf("\n")
					}
				}
			}

			// Summary
			if len(results) > 1 {
				fmt.Printf("\n=== 总结 ===\n")
				total := 0
				for _, result := range results {
					total += result.Score
				}
				average := float64(total) / float64(len(results))
				fmt.Printf("平均得分: %.1f/100\n", average)
				fmt.Printf("评估文件数: %d\n", len(results))
			}
		}

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
			consts.SetGormSSAProjectDatabase(db)
		} else if databaseFileRaw != "" {
			// set database path
			if utils.GetFirstExistedFile(databaseFileRaw) == "" {
				// no compile ,database not existed
				return utils.Errorf("database file not found: %v use default database", databaseFileRaw)
			}
			consts.SetGormSSAProjectDatabaseByInfo(databaseFileRaw)
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
	syntaxFlowCreate,              // create rule template
	syntaxflowFormat,              //  format syntaxflow rule
	syntaxflowCompletion,          // complete syntaxflow rule with AI
	syntaxflowTestCasesCompletion, // complete test cases with AI
	syntaxFlowSave,                // save rule to database
	syntaxFlowEvaluate,            // evaluate rule quality
	syntaxFlowExport,              // export rule to file
	syntaxFlowImport,              // import rule from file
	syncRule,                      // sync rule from embed to database
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

	for _, c := range callbacks {
		c(result)
	}

	log.Infof("syntax flow query result:")
	result.Show(
		sfvm.WithShowAll(sfDebug),
		sfvm.WithShowCode(withCode),
		sfvm.WithShowDot(showDot),
	)
	return execError
}

func outputJSON(writer io.Writer, report *sfreport.Report) error {
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return utils.Wrap(err, "failed to marshal JSON")
	}
	_, err = writer.Write(jsonData)
	return err
}

func outputSARIF(writer io.Writer, report *sfreport.Report) error {
	fmt.Fprintf(writer, "SARIF format output not fully implemented yet.\n")
	fmt.Fprintf(writer, "Please use 'irify' or 'irify-full' format for now.\n")
	return outputJSON(writer, report)
}

func outputConsole(writer io.Writer, report *sfreport.Report, severityFilter, ruleFilter string, withCode bool) error {
	filteredRisks := filterRisks(report.Risks, severityFilter, ruleFilter)

	fmt.Fprintf(writer, "=== Scan Report Summary ===\n")
	fmt.Fprintf(writer, "Scan Time: %s\n", report.ReportTime.Format("2006-01-02T15:04:05-07:00"))
	fmt.Fprintf(writer, "Program: %s\n", report.ProgramName)
	fmt.Fprintf(writer, "Language: %s\n", report.ProgramLang)
	fmt.Fprintf(writer, "Files Scanned: %d\n", report.FileCount)
	fmt.Fprintf(writer, "Lines of Code: %d\n", report.CodeLineCount)
	fmt.Fprintf(writer, "Risks Found: %d\n", len(filteredRisks))
	fmt.Fprintf(writer, "\n")

	fmt.Fprintf(writer, "=== Risk Details ===\n")
	riskCount := 0
	for hash, risk := range filteredRisks {
		riskCount++
		fmt.Fprintf(writer, "Risk ID: %s\n", hash)
		fmt.Fprintf(writer, "Title: %s\n", risk.GetTitleVerbose())
		fmt.Fprintf(writer, "Severity: %s\n", risk.GetSeverity())
		fmt.Fprintf(writer, "Location: %s:%d\n", risk.GetCodeSourceURL(), risk.GetLine())

		description := risk.GetDescription()
		if description != "" {
			cleanDesc := strings.TrimSpace(description)
			fmt.Fprintf(writer, "Description: %s\n", cleanDesc)
		}

		solution := risk.GetSolution()
		if solution != "" {
			cleanSolution := strings.TrimSpace(solution)
			if len(cleanSolution) > 200 {
				cleanSolution = cleanSolution[:200] + "..."
			}
			fmt.Fprintf(writer, "Solution: %s\n", cleanSolution)
		}

		if withCode {
			fmt.Fprintf(writer, "Affected Code:\n")
			codeFragment := risk.GetCodeFragment()
			if codeFragment != "" {
				codeLines := strings.Split(codeFragment, "\n")
				for _, line := range codeLines {
					fmt.Fprintf(writer, "  %s\n", line)
				}
			}
		}

		fmt.Fprintf(writer, "\n")
	}

	fmt.Fprintf(writer, "=== Affected Files ===\n")
	for _, file := range report.File {
		hasFilteredRisks := false
		filteredFileRisks := []string{}
		for _, riskHash := range file.Risks {
			if _, exists := filteredRisks[riskHash]; exists {
				hasFilteredRisks = true
				filteredFileRisks = append(filteredFileRisks, riskHash)
			}
		}

		if hasFilteredRisks {
			fmt.Fprintf(writer, "File: %s\n", file.Path)
			fmt.Fprintf(writer, "Lines: %d\n", file.LineCount)
			fmt.Fprintf(writer, "Risk IDs: %s\n", strings.Join(filteredFileRisks, ", "))
			fmt.Fprintf(writer, "\n")
		}
	}

	return nil
}

func filterRisks(risks map[string]*sfreport.Risk, severityFilter, ruleFilter string) map[string]*sfreport.Risk {
	filtered := make(map[string]*sfreport.Risk)

	for hash, risk := range risks {
		if severityFilter != "" && !strings.EqualFold(risk.GetSeverity(), severityFilter) {
			continue
		}

		if ruleFilter != "" && !strings.Contains(strings.ToLower(risk.GetRuleName()), strings.ToLower(ruleFilter)) {
			continue
		}

		filtered[hash] = risk
	}

	return filtered
}
