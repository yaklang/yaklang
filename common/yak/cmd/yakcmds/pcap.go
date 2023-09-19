package yakcmds

import (
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"strings"
)

var PcapCommand = cli.Command{
	Name:  "pcap",
	Usage: "抓包并使用规则过滤",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "device",
			Usage: "网卡（可选多个,使用逗号分隔）",
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
		var opts []pcaputil.CaptureOption
		if c.Bool("v") {
			opts = append(opts, pcaputil.WithDebug(true))
		}
		if device := c.String("device"); device != "" {
			opts = append(opts, pcaputil.WithDevice(strings.Split(device, ",")...))
		}
		if input := c.String("input"); input != "" {
			opts = append(opts, pcaputil.WithFile(input))
		}
		if output := c.String("output"); output != "" {
			opts = append(opts, pcaputil.WithOutput(output))
		}
		if suricata := c.String("suricata"); suricata != "" {
			opts = append(opts, pcaputil.WithSuricataFilter(suricata))
		}

		opts = append(opts, pcaputil.WithOnTrafficFlowOnDataFrameReassembled(func(flow *pcaputil.TrafficFlow, conn *pcaputil.TrafficConnection, frame *pcaputil.TrafficFrame) {
			if len(frame.Payload) <= 0 {
				return
			}

			if hellospec, err := tlsutils.ParseClientHello(frame.Payload); err == nil {
				log.Infof("%v SNI: %v", flow.String(), hellospec.SNI())
			} else if req, err := utils.ReadHTTPRequestFromBytes(frame.Payload); err == nil {
				u, _ := lowhttp.ExtractURLFromHTTPRequest(req, false)
				if u != nil {
					log.Infof("%v %v %v", flow.String(), req.Method, u.String())
				}
			}
		}))
		return pcaputil.Start(opts...)
	},
}
