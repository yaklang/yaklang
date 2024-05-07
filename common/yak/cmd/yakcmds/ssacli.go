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

			var language = make(map[ssaapi.Language]int)
			var autoDetected = 0
			log.Infof("start to detect language in %v", file)
			err := filesys.Recursive(
				file,
				filesys.WithFileStat(func(pathname string, file fs.File, info fs.FileInfo) error {
					log.Infof("checking: %v", pathname)
					ext := strings.TrimLeft(filepath.Ext(pathname), ".")
					autoDetected++
					switch strings.ToLower(ext) {
					case "yak", "yaklang":
						if _, ok := language[ssaapi.Yak]; !ok {
							language[ssaapi.Yak] = 0
						}
						language[ssaapi.Yak]++
					case "java":
						if _, ok := language[ssaapi.JAVA]; !ok {
							language[ssaapi.JAVA] = 0
						}
						language[ssaapi.JAVA]++
					case "php":
						if _, ok := language[ssaapi.PHP]; !ok {
							language[ssaapi.PHP] = 0
						}
						language[ssaapi.PHP]++
					case "js":
						if _, ok := language[ssaapi.JS]; !ok {
							language[ssaapi.JS] = 0
						}
						language[ssaapi.JS]++
					default:
						autoDetected--
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

			var selectedLanguage ssaapi.Language
			var hit int
			for l, count := range language {
				if count > hit {
					hit = count
					selectedLanguage = l
				}
			}

			log.Infof("start to compile file: %v with db: %v", file, name)
			proj, err := ssaapi.ParseProjectFromPath(
				file,
				ssaapi.WithDatabaseProgramName(name), ssaapi.WithLanguage(selectedLanguage),
			)
			if err != nil {
				return utils.Wrapf(err, "parse project [%v] failed", file)
			}

			log.Infof("finished compiling..., results: %v", len(proj))
			return nil
		},
	},
}
