package yakcmds

import (
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

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

var SSACompilerCommands = []*cli.Command{
	{
		Name:    "ssa-remove",
		Aliases: []string{"ssa-rm"},
		Usage:   "Remove SSA OpCodes from database",
		Action: func(c *cli.Context) {
			for _, name := range c.Args() {
				if name == "*" {
					for _, i := range ssadb.AllPrograms(ssadb.GetDB()) {
						log.Infof("Start to delete program: %v", i)
						ssadb.DeleteProgram(ssadb.GetDB(), i)
					}
					break
				}
				log.Infof("Start to delete program: %v", name)
				ssadb.DeleteProgram(ssadb.GetDB(), name)
			}
		},
	},
	{
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
		},
		Action: func(c *cli.Context) error {
			if ret, err := log.ParseLevel(c.String("log")); err == nil {
				log.SetLevel(ret)
			}

			programName := c.String("program")
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
			noOverride := c.Bool("no-override")
			syntaxFlow := c.String("syntaxflow")
			dbDebug := c.Bool("database-debug")
			sfDebug := c.Bool("syntaxflow-debug")
			showDot := c.Bool("dot")
			withCode := c.Bool("with-code")
			// TODO: re-compile
			// re-compile := c.Bool("re-compile")

			// set database
			if databaseFileRaw != "" {
				// set database path
				if target == "" &&
					utils.GetFirstExistedFile(databaseFileRaw) == "" {
					// no compile ,database not existed
					return utils.Errorf("database file not found: %v", databaseFileRaw)
				}
			}
			consts.SetSSADataBaseName(databaseFileRaw)

			// compile
			if target == "" {
				return utils.Errorf("target file not found: %v", rawFile)
			}
			opt := make([]ssaapi.Option, 0, 3)
			log.Infof("start to compile file: %v ", target)
			if input_language != "" {
				input_language = strings.ToLower(input_language)
				var language consts.Language
				switch strings.ToLower(input_language) {
				case "javascript", "js":
					language = ssaapi.JS
				case "yak", "yaklang":
					language = ssaapi.Yak
				default:
					language = consts.Language(input_language)
				}
				log.Infof("start to use language: %v", language)
				opt = append(opt, ssaapi.WithLanguage(language))
			}
			if entry != "" {
				log.Infof("start to use entry file: %v", entry)
				opt = append(opt, ssaapi.WithFileSystemEntry(entry))
			}

			if inMemory {
				log.Infof("compile in memory mode, program-name will be ignored")
			} else {
				if programName == "" {
					programName = "default-" + ksuid.New().String()
				}
				log.Infof("compile save to database with program name: %v", programName)
				opt = append(opt, ssaapi.WithDatabaseProgramName(programName))
			}

			if !noOverride {
				ssadb.DeleteProgram(ssadb.GetDB(), programName)
			} else {
				log.Warnf("no-override flag is set, will not delete existed program: %v", programName)
			}

			proj, err := ssaapi.ParseProjectFromPath(target, opt...)
			if err != nil {
				return utils.Errorf("parse project [%v] failed: %v", target, err)
			}

			log.Infof("finished compiling..., results: %v", len(proj))
			if syntaxFlow != "" {
				log.Warn("Deprecated: syntax flow query language will be removed in ssa sub-command, please use `ssa-query(in short: sf/syntaxFlow)` instead")
				return SyntaxFlowQuery(programName, databaseFileRaw, syntaxFlow, dbDebug, sfDebug, showDot, withCode)
			}
			return nil
		},
	},
	{
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
	},
	{
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
	},
	{
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
			err = sfdb.ImportDatabase(file)
			if err != nil {
				return err
			}
			return nil
		},
	},
	{
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
	},
	{
		Name:    "ssa-query",
		Aliases: []string{"sf", "syntaxFlow", "sf-scan"},
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
		},
		Action: func(c *cli.Context) error {
			if ret, err := log.ParseLevel(c.String("log")); err == nil {
				log.SetLevel(ret)
			}
			programName := c.String("program")
			databaseFileRaw := c.String("database")
			dbDebug := c.Bool("database-debug")
			sfDebug := c.Bool("syntaxflow-debug")
			syntaxFlow := c.String("syntaxflow")
			showDot := c.Bool("dot")
			withCode := c.Bool("with-code")

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
			var results []*sfvm.SFFrameResult
			var sarifCallback func(result *sfvm.SFFrameResult)
			if haveSarifRequired {
				sarifCallback = func(result *sfvm.SFFrameResult) {
					results = append(results, result)
				}
			} else {
				sarifCallback = func(result *sfvm.SFFrameResult) {

				}
			}

			defer func() {
				if len(results) > 0 && sarifFile != "" {
					log.Infof("fetch result: %v, exports sarif to %v", len(results), sarifFile)
					report, err := ssaapi.ConvertSyntaxFlowResultToSarif(results...)
					if err != nil {
						log.Errorf("convert SARIF failed: %v", err)
						return
					}
					if utils.GetFirstExistedFile(sarifFile) != "" {
						backup := sarifFile + ".bak"
						os.Rename(sarifFile, backup)
						os.RemoveAll(sarifFile)
					}
					err = report.WriteFile(sarifFile)
					if err != nil {
						log.Errorf("write SARIF failed: %v", err)
					}
				}
			}()

			if syntaxFlow != "" {
				return SyntaxFlowQuery(programName, databaseFileRaw, syntaxFlow, dbDebug, sfDebug, showDot, withCode, sarifCallback)
			}

			var dirChecking []string

			handleBySyntaxFlowContent := func(syntaxFlow string) error {
				err := SyntaxFlowQuery(programName, databaseFileRaw, syntaxFlow, dbDebug, sfDebug, showDot, withCode, sarifCallback)
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
	},
}

func SyntaxFlowQuery(
	programName, databaseFileRaw string,
	syntaxFlow string,
	dbDebug, sfDebug, showDot, withCode bool,
	callbacks ...func(*sfvm.SFFrameResult),
) error {
	// set database
	if databaseFileRaw != "" {
		// set database path
		if utils.GetFirstExistedFile(databaseFileRaw) == "" {
			// no compile ,database not existed
			return utils.Errorf("database file not found: %v use default database", databaseFileRaw)
		}
	}
	consts.SetSSADataBaseName(databaseFileRaw)

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
	opt := make([]sfvm.Option, 0)
	if sfDebug {
		opt = append(opt, sfvm.WithEnableDebug())
	}
	var execError error
	result, err := prog.SyntaxFlowWithError(syntaxFlow, opt...)
	if err != nil {
		var otherErrs []string
		if result != nil && len(result.Errors) > 0 {
			otherErrs = utils.StringArrayFilterEmpty(utils.RemoveRepeatStringSlice(result.Errors))
		}
		execError = utils.Wrapf(err, "prompt error: \n%v", strings.Join(otherErrs, "\n  "))
	}
	if result == nil {
		return execError
	}

	if result.SFFrameResult != nil {
		for _, c := range callbacks {
			c(result.SFFrameResult)
		}
	}

	log.Infof("syntax flow query result:")
	if withCode {
		if len(result.AlertSymbolTable) != 0 {
			for name := range result.AlertSymbolTable {
				showValues(name, result.GetValues(name), showDot)
			}
		} else if result.SymbolTable.Len() != 0 {
			for k, r := range result.GetAllValues() {
				if k == "_" {
					continue
				}
				showValues(k, r, showDot)
			}
		} else {
			showValues("_", result.GetValues("_"), showDot)
		}
	} else {
		result.Show()
		if showDot {
			fmt.Println("---------------------")
			fmt.Println(result.GetAllValuesChain().DotGraph())
		}
	}
	return execError
}

func showValues(name string, vs ssaapi.Values, showDot bool) {
	log.Infof("===================== Variable:%v =================== ", name)
	show(vs)
	if showDot {
		log.Infof("===================== DOT =================== ")
		vs.ShowDot()
	}
}
func show(r ssaapi.Values) {
	for _, v := range r {
		codeRange := v.GetRange()
		if codeRange == nil {
			log.Infof("IR: %d: %s", v.GetId(), v.String())
			log.Errorf("IR: %d, code range not found\n", v.GetId())
			continue
		}
		editor := codeRange.GetEditor()
		ctxText, _ := editor.GetContextAroundRange(
			codeRange.GetStart(),
			codeRange.GetStart(),
			// codeRange.GetEnd(),
			3,
			func(i int) string {
				if i == codeRange.GetStart().GetLine() {
					return fmt.Sprintf(">>%5s| ", fmt.Sprint(i))
				}
				return fmt.Sprintf("%7s| ", fmt.Sprint(i))
			},
		)
		log.Infof("%s:%s \nIR: %d: %s\n%s\n",
			editor.GetFilename(), codeRange.String(),
			v.GetId(), v.String(),
			ctxText,
		)
	}
}
