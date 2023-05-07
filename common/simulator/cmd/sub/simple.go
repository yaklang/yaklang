package sub

import (
	"github.com/urfave/cli"
)

var Simple = cli.Command{
	Name:   "simple",
	Usage:  "simple browser simulator action",
	Before: nil,
	After:  nil,

	OnUsageError: nil,
	Subcommands:  nil,

	Action: func(c *cli.Context) error {
		return nil
	},
}
