package sub

import (
	"github.com/urfave/cli"
)

var TestModule = cli.Command{
	Name:   "test",
	Usage:  "test sth",
	Before: nil,
	After:  nil,

	OnUsageError: nil,
	Subcommands:  nil,

	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "eval",
			Usage: "eval test",
		},
		cli.BoolFlag{
			Name:  "getstr",
			Usage: "GetElementString",
		},
		cli.BoolFlag{
			Name:  "parent",
			Usage: "get parent",
		},
		cli.BoolFlag{
			Name: "test",
		},
	},
	Action: func(c *cli.Context) error {
		return nil
	},
}
