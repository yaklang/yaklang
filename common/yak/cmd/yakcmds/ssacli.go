package yakcmds

import (
	"github.com/segmentio/ksuid"
	"github.com/urfave/cli"
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
			file := utils.GetFirstExistedPath(rawFile)

			dbDebug := c.Bool("database-debug")

			// syntax flow query
			if sf := c.String("syntaxflow"); sf != "" {
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
				result, err := prog.SyntaxFlowWithError(sf)
				if err != nil {
					log.Errorf("syntax flow [%s] query failed: %v", sf, err)
					return nil
				}
				log.Infof("syntax flow query result:")
				result.Show()
				return nil
			}

			// parse project
			if file == "" {
				log.Warnf("file or dir not found: %v", c.String("target"))
				return nil
			}

			opt := make([]ssaapi.Option, 0, 3)
			log.Infof("start to compile file: %v ", file)
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

			proj, err := ssaapi.ParseProjectFromPath(file, opt...)
			if err != nil {
				return utils.Wrapf(err, "parse project [%v] failed", file)
			}

			log.Infof("finished compiling..., results: %v", len(proj))
			return nil
		},
	},
}
