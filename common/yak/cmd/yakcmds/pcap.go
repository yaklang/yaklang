package yakcmds

import (
	"bufio"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"io"
	"strings"
	"time"
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

		trafficHandler := func(verbose string, f io.Reader) {
			br := bufio.NewReader(f)
			firstByte, err := br.ReadByte()
			if err != nil && err != io.EOF {
				log.Errorf("read first byte failed: %s", err)
				return
			}

			// SNI
			if firstByte == 0x16 {
				br.UnreadByte()
				raw := utils.StableReader(br, time.Second, 65535)
				clientHello, err := tlsutils.ParseClientHello(raw)
				if err != nil {
					log.Errorf("parse client hello failed: %s", err)
					return
				}
				log.Infof("flow: %v SNI: %s", verbose, clientHello.SNI())
				//} else if firstByte == 'H' {
				//	br.UnreadByte()
				//	log.Infof("checking first byte for http response: %x", firstByte)
				//	rsp, err := lowhttp.ReadHTTPResponseFromBufioReader(br, nil)
				//	if err != nil {
				//		return
				//	}
				//	raw, _ := utils.DumpHTTPResponse(rsp, true)
				//	fmt.Println(string(raw))
				//} else if (firstByte >= 'a' && firstByte <= 'z') ||
				//	(firstByte >= 'A' && firstByte <= 'Z') ||
				//	(firstByte >= '0' && firstByte <= '9') {
				//	br.UnreadByte()
				//	// HTTP
				//	log.Infof("checking first byte for http request: %x", firstByte)
				//	req, err := lowhttp.ReadHTTPRequestFromBufioReader(br)
				//	if err != nil {
				//		return
				//	}
				//	raw, _ := utils.DumpHTTPRequest(req, true)
				//	if req.Header.Get("Host") != "" {
				//		fmt.Println(string(raw))
				//	}
				//} else {
				//	io.Copy(io.Discard, br)
			}
		}
		_ = trafficHandler
		// httpflow
		// sni
		opts = append(opts, pcaputil.WithOnTrafficFlow(func(f *pcaputil.TrafficFlow) {
			go func() {
				for !f.IsClosed() {
					time.Sleep(time.Second)
					clientBuffer := f.ClientConn.GetBuffer()
					// serverBuffer := f.ServerConn.GetBuffer()
					trafficHandler(f.String(), clientBuffer)
				}
			}()
		}))
		return pcaputil.Start(opts...)
	},
}
