package yakcmds

import (
	"fmt"
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
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type languageCtx struct {
	hit     int64
	matched []string
}

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
		Action: func(c *cli.Context) {
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
			language := c.String("language")
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
					log.Errorf("database file not found: %v", databaseFileRaw)
				}
			}
			consts.SetSSADataBaseName(databaseFileRaw)

			// compile
			if target == "" {
				log.Errorf("target file not found: %v", rawFile)
				return
			}
			opt := make([]ssaapi.Option, 0, 3)
			log.Infof("start to compile file: %v ", target)
			if language != "" {
				log.Infof("start to use language: %v", language)
				opt = append(opt, ssaapi.WithLanguage(ssaapi.Language(language)))
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
				log.Warnf("no-override flag is set, will not delete existed program")
			}

			proj, err := ssaapi.ParseProjectFromPath(target, opt...)
			if err != nil {
				log.Errorf("parse project [%v] failed: %v", target, err)
			}

			log.Infof("finished compiling..., results: %v", len(proj))
			if syntaxFlow != "" {
				SyntaxFlowQuery(programName, databaseFileRaw, syntaxFlow, dbDebug, sfDebug, showDot, withCode)
				log.Warn("Deprecated: syntax flow query language will be removed in ssa sub-command, please use `ssa-query(in short: sf/syntaxFlow)` instead")
			}
		},
	},
	{
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
		},
		Action: func(c *cli.Context) {
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

			if syntaxFlow != "" {
				SyntaxFlowQuery(programName, databaseFileRaw, syntaxFlow, dbDebug, sfDebug, showDot, withCode)
				return
			}

			var dirChecking []string

			handleByFilename := func(filename string) {
				log.Infof("start to use SyntaxFlow rule: %v", filename)
				raw, err := os.ReadFile(filename)
				if err != nil {
					log.Errorf("read file [%v] failed: %v", filename, err)
					return
				}
				syntaxFlow = string(raw)
				SyntaxFlowQuery(programName, databaseFileRaw, syntaxFlow, dbDebug, sfDebug, showDot, withCode)
				fmt.Println()
			}

			for _, originName := range c.Args() {
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
				handleByFilename(name)
			}

			for _, dir := range dirChecking {
				log.Infof("start to read directory: %v", dir)
				err := filesys.Recursive(dir, filesys.WithRecursiveDirectory(true), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
					fileExt := strings.ToLower(filepath.Ext(s))
					if strings.HasSuffix(fileExt, ".sf") {
						handleByFilename(s)
					}
					return nil
				}))
				if err != nil {
					log.Warnf("read directory [%v] failed: %v", dir, err)
				}
			}

		},
	},
}

func SyntaxFlowQuery(
	programName, databaseFileRaw string,
	syntaxFlow string,
	dbDebug, sfDebug, showDot, withCode bool,
) {

	// set database
	if databaseFileRaw != "" {
		// set database path
		if utils.GetFirstExistedFile(databaseFileRaw) == "" {
			// no compile ,database not existed
			log.Errorf("database file not found: %v use default database", databaseFileRaw)
		}
	}
	consts.SetSSADataBaseName(databaseFileRaw)

	if programName == "" {
		log.Errorf("program name is required when using syntax flow query language")
	}
	// program from database
	prog, err := ssaapi.FromDatabase(programName)
	if err != nil {
		log.Errorf("load program [%v] from database failed: %v", programName, err)
	}
	if prog.DBCache != nil && dbDebug {
		prog.DBCache.DB = prog.DBCache.DB.Debug()
	}
	opt := make([]sfvm.Option, 0)
	if sfDebug {
		opt = append(opt, sfvm.WithEnableDebug())
	}
	result, err := prog.SyntaxFlowWithError(syntaxFlow, opt...)
	if err != nil {
		log.Errorf("syntax flow [%s] query failed: %v", syntaxFlow, err)
		return
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
			editor.GetUrl(), codeRange.String(),
			v.GetId(), v.String(),
			ctxText,
		)
	}
}
