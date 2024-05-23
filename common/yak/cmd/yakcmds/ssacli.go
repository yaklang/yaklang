package yakcmds

import (
	"fmt"

	"github.com/segmentio/ksuid"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type languageCtx struct {
	hit     int64
	matched []string
}

var SSACompilerCommands = []*cli.Command{
	{
		Name:    "ssa-compile",
		Aliases: []string{"ssa"},
		Usage:   "Compile to SSA OpCodes from source code",
		Flags: []cli.Flag{
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
		},
		Action: func(c *cli.Context) error {
			programName := c.String("program")
			entry := c.String("entry")
			language := c.String("language")
			inMemory := c.Bool("memory")
			rawFile := c.String("target")
			target := utils.GetFirstExistedPath(rawFile)
			syntaxFlow := c.String("syntaxflow")
			databaseFileRaw := c.String("database")
			dbDebug := c.Bool("database-debug")

			// set database
			if databaseFileRaw != "" {
				// set database path
				if target == "" &&
					utils.GetFirstExistedFile(databaseFileRaw) == "" {
					// no compile ,database not existed
					log.Errorf("database file not found: %v", databaseFileRaw)
					return nil
				}
			}
			consts.SetSSADataBaseName(databaseFileRaw)

			// compile
			if target != "" {
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

				proj, err := ssaapi.ParseProjectFromPath(target, opt...)
				if err != nil {
					log.Errorf("parse project [%v] failed: %v", target, err)
					return nil
				}

				log.Infof("finished compiling..., results: %v", len(proj))
			}

			// syntax flow query
			if syntaxFlow != "" {
				if programName == "" {
					log.Errorf("program name is required when using syntax flow query language")
					return nil
				}
				// program from database
				prog, err := ssaapi.FromDatabase(programName)
				if err != nil {
					log.Errorf("load program [%v] from database failed: %v", programName, err)
					return nil
				}
				if prog.DBCache != nil && dbDebug {
					prog.DBCache.DB = prog.DBCache.DB.Debug()
				}
				result, err := prog.SyntaxFlowWithError(syntaxFlow)
				if err != nil {
					log.Errorf("syntax flow [%s] query failed: %v", syntaxFlow, err)
					return nil
				}
				log.Infof("syntax flow query result:")
				for k, r := range result {
					log.Infof("\nkey:%v", k)
					for _, v := range r {
						codeRange := v.GetRange()
						editor := codeRange.GetEditor()
						ctxText, _ := editor.GetContextAroundRange(
							codeRange.GetStart(),
							codeRange.GetEnd(),
							3,
							func(i int) string {
								return fmt.Sprintf("%5s| ", fmt.Sprint(i))
							},
						)
						log.Infof("%s:%d \nIR: %s\n%s\n",
							editor.GetUrl(), codeRange.GetStart().GetLine(),
							v.String(),
							ctxText,
						)
					}
				}
			}
			return nil
		},
	},
}
