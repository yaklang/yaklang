package sub

import (
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/simulator/examples"
)

var Example = cli.Command{
	Name:   "example",
	Usage:  "page simulator example",
	Before: nil,
	After:  nil,

	OnUsageError: nil,
	Subcommands:  nil,

	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "bruteforce",
			Usage: "bruteforce example",
		},
		cli.StringFlag{
			Name:  "url",
			Usage: "bruteforce example url",
		},
	},
	Action: func(c *cli.Context) error {
		url := c.String("url")
		if url == "" {
			//url = "http://192.168.0.80/member.php?c=login"
			url = "http://192.168.0.68/#/login"
		}
		usernameList := []string{"admin"}
		passwordList := []string{"luckyadmin123"}
		if c.Bool("bruteforce") {
			//examples.BruteForceModule(url, usernameList, passwordList)
			userOpt := examples.WithUserNameList(usernameList)
			passOpt := examples.WithPassWordList(passwordList)
			scanMode := examples.WithCaptchaMode("common_arithmetic")
			remoteWs := examples.WithWsAddress("http://192.168.0.115:7317/")
			result, err := examples.BruteForceModuleV2(url, userOpt, passOpt, scanMode, remoteWs)
			log.Info(err)
			log.Info(result.Username(), result.Password(), result.Log(), result.Cookie())
		}
		return nil
	},
}
