package yakcmds

import "github.com/urfave/cli"

var PassiveCommands = cli.Command{
	Name:      "passive-scan",
	ShortName: "passive",
	Aliases: []string{
		"webscan",
	},
	Usage:       "yak passive-scan [options]",
	Description: "Passive Proxy(MITM) Scan.",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "listen",
			Value: "0.0.0.0:8084",
			Usage: "MITM on which addr:port?",
		},
	},
	Action: func(c *cli.Context) error {

		return nil
	},
}
