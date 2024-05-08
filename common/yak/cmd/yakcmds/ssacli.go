package yakcmds

import (
	"github.com/segmentio/ksuid"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"io/fs"
	"path/filepath"
	"strings"
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
				Name:  "program-name,p",
				Usage: `program name to save in database`,
			},
			cli.StringFlag{
				Name:  "entry",
				Usage: "Program Entry",
			},
			cli.BoolFlag{
				Name: "memory",
			},
		},
		Action: func(c *cli.Context) error {
			file := utils.GetFirstExistedPath(c.String("target"))
			if file == "" {
				log.Warnf("file or dir not found: %v", c.String("target"))
				return nil
			}

			name := c.String("program")
			if name == "" {
				name = "default-" + ksuid.New().String()
			}

			var language = make(map[ssaapi.Language]*languageCtx)
			var autoDetected = 0
			log.Infof("start to detect language in %v", file)
			err := filesys.Recursive(
				file,
				filesys.WithFileStat(func(pathname string, file fs.File, info fs.FileInfo) error {
					log.Infof("checking: %v", pathname)
					ext := strings.TrimLeft(filepath.Ext(pathname), ".")
					autoDetected++
					var current *languageCtx
					var ok bool
					switch strings.ToLower(ext) {
					case "yak", "yaklang":
						if current, ok = language[ssaapi.Yak]; !ok {
							current = &languageCtx{}
							language[ssaapi.Yak] = current
						}
					case "java":
						if current, ok = language[ssaapi.JAVA]; !ok {
							current = &languageCtx{}
							language[ssaapi.JAVA] = current
						}

					case "php":
						if _, ok := language[ssaapi.PHP]; !ok {
							current = &languageCtx{}
							language[ssaapi.PHP] = current
						}
					case "js":
						if _, ok := language[ssaapi.JS]; !ok {
							current = &languageCtx{}
							language[ssaapi.JS] = current
						}
					default:
						autoDetected--
					}

					if current != nil {
						current.hit++
						current.matched = append(current.matched, pathname)
					}

					if autoDetected > 100 {
						return utils.Error("normal exit")
					}
					return nil
				}),
			)
			if err != nil && len(language) <= 0 {
				return err
			}

			if len(language) <= 0 {
				log.Errorf("no language detected in %v", file)
				return utils.Errorf("no language detected in %v", file)
			}

			entry := c.String("entry")
			var selectedLanguage ssaapi.Language
			var hit int64
			for l, lctx := range language {
				if lctx.hit > hit {
					hit = lctx.hit
					selectedLanguage = l
					if entry == "" {
						entry = lctx.matched[0]
					}
				}
			}

			log.Infof("start to compile file: %v with db: %v language: %v", file, name, selectedLanguage)

			if entry != "" {
				log.Infof("start to use entry: %v", entry)
			}

			if c.Bool("memory") {
				name = ""
				log.Infof("compile in memory mode, program-name will be ignored")
			}

			proj, err := ssaapi.ParseProjectFromPath(
				file,
				ssaapi.WithDatabaseProgramName(name),
				ssaapi.WithLanguage(selectedLanguage),
				ssaapi.WithFileSystemEntry(entry),
			)
			if err != nil {
				return utils.Wrapf(err, "parse project [%v] failed", file)
			}

			log.Infof("finished compiling..., results: %v", len(proj))
			return nil
		},
	},
}
