package yakcmds

import (
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/urfave/cli"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/match"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"net/http"
	"strings"
)

var pcapCommand = cli.Command{
	Name:  "pcap",
	Usage: "Sniff network traffic and output to stdout",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "l,list-devices",
			Usage: `List available devices`,
		},
		cli.StringFlag{
			Name:  "device,d",
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
		if c.Bool("list-devices") {
			ifaces, err := pcap.FindAllDevs()
			if err != nil {
				return err
			}
			for _, i := range ifaces {
				fmt.Printf("%s (%s)\n", i.Name, i.Description)
				for _, addr := range i.Addresses {
					fmt.Printf("  %s\n", addr.IP)
				}
			}
			return nil
		}

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
		mng := yakit.NewTrafficStorageManager(consts.GetGormProjectDatabase())

		opts = append(
			opts,
			pcaputil.WithEveryPacket(func(packet gopacket.Packet) {
				err := mng.SaveRawPacket(packet)
				if err != nil {
					log.Errorf("save traffic failed: %s", err)
				}
			}),
			pcaputil.WithTLSClientHello(func(flow *pcaputil.TrafficFlow, hello *tlsutils.HandshakeClientHello) {
				if group == nil {
					log.Infof("%v SNI: %v", flow.String(), hello.SNI())
					return
				}
			}),
			pcaputil.WithOnTrafficFlowCreated(func(flow *pcaputil.TrafficFlow) {
				err := mng.CreateTCPReassembledFlow(flow)
				if err != nil {
					log.Errorf("create flow failed: %s", err)
				}
			}),
			pcaputil.WithOnTrafficFlowOnDataFrameReassembled(func(flow *pcaputil.TrafficFlow, conn *pcaputil.TrafficConnection, frame *pcaputil.TrafficFrame) {
				err := mng.SaveTCPReassembledFrame(flow, frame)
				if err != nil {
					log.Errorf("save frame failed: %s", err)
				}
			}),
			pcaputil.WithOnTrafficFlowClosed(func(reason pcaputil.TrafficFlowCloseReason, flow *pcaputil.TrafficFlow) {
				var err error
				switch reason {
				case pcaputil.TrafficFlowCloseReason_INACTIVE:
					err = mng.CloseTCPFlow(flow, false)
				case pcaputil.TrafficFlowCloseReason_FIN:
					err = mng.CloseTCPFlow(flow, false)
				case pcaputil.TrafficFlowCloseReason_RST:
					err = mng.CloseTCPFlow(flow, true)
				}
				if err != nil {
					log.Errorf("close flow failed: %s", err)
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

					var urlStr string
					urlIns, _ := lowhttp.ExtractURLFromHTTPRequestRaw(reqBytes, false)
					if urlIns != nil {
						urlStr = urlIns.String()
					}
					yakit.SaveFromHTTPFromRaw(consts.GetGormProjectDatabase(), false, reqBytes, rspBytes, "pcap", urlStr, "")
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
