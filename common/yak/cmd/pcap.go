package main

import "github.com/urfave/cli"

var pcapCommand = cli.Command{
	Name:  "pcap",
	Usage: "抓包并使用规则过滤",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "device",
			Usage: "网卡",
		},
		cli.StringFlag{
			Name:  "input",
			Usage: "pcap文件路径",
		},
		cli.StringFlag{
			Name:  "output",
			Usage: "过滤后的流量导出路径",
		},
		cli.BoolFlag{
			Name:  "v",
			Usage: "输出详细信息",
		},
		cli.StringFlag{
			Name:  "suricata",
			Usage: "suricata规则文件路径",
		},
	},
	Action: func(c *cli.Context) error {
		return nil
	},
}
