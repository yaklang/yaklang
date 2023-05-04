package sub

import (
	"github.com/urfave/cli"
	test2 "github.com/yaklang/yaklang/common/simulator/test"
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
		if c.Bool("eval") {
			test2.EvalTest()
		} else if c.Bool("getstr") {
			test2.GetElementString()
		} else if c.Bool("parent") {
			test2.GetElementParent()
		} else if c.Bool("test") {
			test2.Length()
		}
		//examples.BruteForce()
		return nil
	},
}
