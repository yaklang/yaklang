package scannode

import (
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"io/ioutil"
	"path/filepath"
)

var DistYakCommand = cli.Command{
	Name: "distyak",
	Action: func(c *cli.Context) error {
		var err error
		args := c.Args()
		if len(args) > 0 {
			// args 被解析到了，说明后面跟着文件，去读文件出来吧
			file := args[0]
			if file != "" {
				var absFile = file
				if !filepath.IsAbs(absFile) {
					absFile, err = filepath.Abs(absFile)
					if err != nil {
						return utils.Errorf("fetch abs file path failed: %s", err)
					}
				}
				raw, err := ioutil.ReadFile(file)
				if err != nil {
					return err
				}

				engine := yak.NewScriptEngine(100)
				engine.HookOsExit()
				engine.RegisterEngineHooks(func(engine yaklang.YaklangEngine) error {
					var e = engine.(*antlr4yak.Engine)
					_ = e
					return nil
				})
				err = engine.ExecuteMain(string(raw), absFile)
				if err != nil {
					return err
				}

				return nil
			} else {
				return utils.Errorf("empty yak file")
			}
		}

		code := c.String("code")
		engine := yak.NewScriptEngine(100)
		engine.HookOsExit()
		err = engine.Execute(code)
		if err != nil {
			return err
		}
		return nil
	},
	Flags: []cli.Flag{
		cli.StringFlag{
			Name: "code,c",
		},
	},
	SkipFlagParsing: true,
}
