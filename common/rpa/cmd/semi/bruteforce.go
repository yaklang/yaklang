package semi

import (
	"yaklang/common/log"
	"yaklang/common/rpa/implement/bruteforce"

	"github.com/urfave/cli"
)

var BruteForce = cli.Command{
	Name:      "bruteforce",
	ShortName: "brute",
	Usage:     "模拟点击暴力密码破解",
	Before:    nil,
	After:     nil,

	OnUsageError: nil,
	Subcommands:  nil,

	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "u,url",
			Usage: "target login url",
		},
		cli.StringFlag{
			Name:  "userfile,user",
			Usage: "username dict filepath",
		},
		cli.StringFlag{
			Name:  "passfile,pass",
			Usage: "password dict filepath",
		},
	},
	Action: func(c *cli.Context) error {
		url := c.String("url")
		if url == "" {
			log.Errorf("get url blank.")
			return nil
		}
		opts := make([]bruteforce.ConfigOpt, 0)
		username := c.String("userfile")
		password := c.String("passfile")
		if username != "" {
			opts = append(opts, bruteforce.WithUserPassPath(username, password))
		}
		// methods := make(bruteforce.ConfigMethods, 0)
		// methods = append(methods, bruteforce.ClickMethod(""))
		// methods = append(methods, bruteforce.InputMethod("", ""))
		// methods = append(methods, bruteforce.SelectMethod("", ""))
		bruteforce.BruteForceStart(url, opts...)
		return nil
	},
}
