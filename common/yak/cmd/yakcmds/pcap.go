package yakcmds

import (
	"fmt"
	"github.com/google/gopacket"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/match"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"net/http"
	"strings"
)

var PcapCommand = cli.Command{
	Name:  "pcap",
	Usage: "Sniff network traffic and output to stdout",
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
		cli.StringFlag{
			Name:  "suricata-rule-keyword,k",
			Usage: `suricata规则关键字，可选多个，使用逗号分隔`,
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
			//opts = append(opts, pcaputil.WithSuricataFilter(suricata))
		}

		var group *match.Group
		if skw := c.String("suricata-rule-keyword"); skw != "" {
			group = match.NewGroup(
				match.WithGroupOnMatchedCallback(func(packet gopacket.Packet, match *rule.Rule) {
					log.Infof("matched rule: %s", match.Message)
				}))
			err := group.LoadRulesWithQuery(skw)
			if err != nil {
				return err
			}
			defer group.Wait()
		}

		opts = append(
			opts,
			pcaputil.WithTLSClientHello(func(flow *pcaputil.TrafficFlow, hello *tlsutils.HandshakeClientHello) {
				if group == nil {
					log.Infof("%v SNI: %v", flow.String(), hello.SNI())
					return
				}
			}),
			pcaputil.WithHTTPFlow(func(flow *pcaputil.TrafficFlow, req *http.Request, rsp *http.Response) {
				if req == nil {
					return
				}

				if group == nil {
					reqBytes, _ := utils.DumpHTTPRequest(req, true)
					fmt.Println(string(reqBytes))
					fmt.Println("-----------------------------------------")
					rspBytes, _ := utils.DumpHTTPResponse(rsp, true)
					fmt.Println(string(rspBytes))
					fmt.Println("-----------------------------------------")
					return
				}
				reqBytes, _ := utils.DumpHTTPRequest(req, true)
				rspBytes, _ := utils.DumpHTTPResponse(rsp, true)
				group.FeedHTTPFlowBytes(reqBytes, rspBytes)
			}),
		)
		return pcaputil.Start(opts...)
	},
}
