package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/urfave/cli"
	"gopkg.in/yaml.v3"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
	"yaklang/common/fp"
	"yaklang/common/fp/cmd/scanfpcmd"
	"yaklang/common/fp/webfingerprint"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/netutil"
	"yaklang/common/yak/yaklib/tools"
)

var (
	sigExitOnce = new(sync.Once)
)

func init() {
	go sigExitOnce.Do(func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
		defer signal.Stop(c)

		for {
			select {
			case <-c:
				fmt.Printf("exit by signal [SIGTERM/SIGINT/SIGKILL]")
				os.Exit(1)
			}
		}
	})
}

func main() {
	app := cli.NewApp()

	app.Commands = []cli.Command{
		scanfpcmd.SynScanCmd,
		scanfpcmd.BruteUtil,
		{Name: "nuclei", Action: func(c *cli.Context) error {
			opt := tools.GetDefaultNucleiOptions()
			spew.Dump(opt)
			return nil
		}},
		{
			Name: "md5fp",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name: "url",
				},
				cli.IntFlag{
					Name:  "max-limit",
					Value: 20480,
				},
				cli.StringFlag{
					Name:  "product",
					Value: "product-demo",
				},
				cli.StringFlag{
					Name: "vendor",
				},
				cli.StringFlag{
					Name: "version",
				},
			},
			Action: func(c *cli.Context) {
				rsp, err := http.Get(c.String("url"))
				if err != nil {
					log.Error(err)
					return
				}

				urlObj, err := url.Parse(c.String("url"))
				if err != nil {
					log.Error(err)
					return
				}

				raw, _ := utils.ReadWithLen(rsp.Body, c.Int("max-limit"))
				md5Value := md5.Sum(raw)
				md5Str := hex.EncodeToString(md5Value[:])

				rule := webfingerprint.WebRule{
					Path: urlObj.Path,
					Methods: []*webfingerprint.WebMatcherMethods{
						{MD5s: []*webfingerprint.MD5Matcher{
							{MD5: md5Str, CPE: webfingerprint.CPE{
								Vendor:  c.String("vendor"),
								Product: c.String("product"),
								Version: c.String("version"),
							}},
						}},
					},
				}

				ruleName, err := yaml.Marshal(rule)
				if err != nil {
					log.Error(err)
					return
				}
				log.Infof("根据MD5生成规则如下：\n\n%v\n\n", string(ruleName))
			},
		},
		{
			Name: "arp",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name: "t,target",
				},
			},
			Action: func(c *cli.Context) error {
				targets := c.String("target")
				tList := utils.StringArrayFilterEmpty(utils.ParseStringToHosts(targets))
				if tList == nil {
					return utils.Errorf("empty target...")
				}

				iface, _, _, err := netutil.Route(5*time.Second, tList[0])
				if err != nil {
					return err
				}

				utils.ARPWithPcap(context.Background(), iface.Name, targets)
				res, err := utils.ArpIPAddressesWithContext(
					utils.TimeoutContextSeconds(5),
					iface.Name,
					targets,
				)
				if err != nil {
					return utils.Errorf("arp ip [%v] from iface: %v error: %v", targets, iface.Name, err)
				}

				for ip, mac := range res {
					println(fmt.Sprintf("%25s MAC: %v", ip, mac.String()))
				}
				return nil
			},
		},
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "hosts,target,t",
			Usage: "输入扫描主机，以逗号分隔例如：(192.168.1.1/24,192.168.1.1-23,10.1.1.2)",
		},
		cli.StringFlag{
			Name:  "port,tcp-port,p",
			Usage: "输入想要扫描的端口，支持单个端口和范围，例如（80,443,21-25,8080-8082）",
			Value: "22,80,443,3389,3306,8080-8082,9000-9002,7000-7002",
		},
		cli.StringFlag{
			Name:  "udp-port",
			Usage: "想要扫描的 UDP 端口，支持单个端口和范围",
		},
		cli.StringFlag{
			Name:  "rule-path,rule,r",
			Usage: "手动加载规则文件/文件夹",
		},
		cli.BoolFlag{
			Name:  "only-rule",
			Usage: "只加载这个文件夹中的 Web 指纹",
		},
		cli.IntFlag{
			Name:  "concurrent,thread,c",
			Usage: "并发速度，同时有多少个扫描过程进行？",
			Value: 20,
		},
		//cli.IntFlag{
		//	Name:  "timeout",
		//	Usage: "超时时间(Seconds)",
		//	Value: 3600,
		//},
		cli.BoolFlag{
			Name:  "web",
			Usage: "主动开启 web 扫描模式",
		},
		cli.IntFlag{
			Name:  "request-timeout",
			Usage: "单个请求的超时时间（Seconds）",
			Value: 10,
		},
		cli.StringFlag{
			Name:  "json,o",
			Usage: "详细结果输出 json 到文件",
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		var options []fp.ConfigOption

		// web rule
		webRules, _ := fp.GetDefaultWebFingerprintRules()
		userRule := webfingerprint.FileOrDirToWebRules(c.String("rule-path"))

		if c.Bool("only-rule") {
			webRules = userRule
		} else {
			webRules = append(webRules, userRule...)
		}

		options = append(
			options,

			// 主动探测模式 - 主动发送符合条件的包
			fp.WithActiveMode(true),

			// 每一个指纹探测请求的超时时间
			fp.WithProbeTimeout(time.Second*time.Duration(c.Int("request-timeout"))),

			// web 指纹火力全开
			fp.WithWebFingerprintUseAllRules(true),

			// web 指纹
			fp.WithWebFingerprintRule(webRules),
		)
		options = append(
			options, fp.WithForceEnableWebFingerprint(true),
		)

		config := fp.NewConfig(options...)

		matcher, err := fp.NewDefaultFingerprintMatcher(config)
		if err != nil {
			return err
		}

		// udp/tcp
		portSwg := utils.NewSizedWaitGroup(c.Int("concurrent"))

		// 结果处理的同步锁
		resultLock := new(sync.Mutex)

		var res []*fp.MatchResult

		scanCore := func(tHost string, tPort int, opts ...fp.ConfigOption) {
			defer portSwg.Done()

			log.Infof("start scan %v", utils.HostPort(tHost, tPort))
			result, err := matcher.Match(
				tHost, tPort,
				opts...,
			)
			if err != nil {
				log.Errorf("scan %v failed: %s", utils.HostPort(tHost, tPort))
				return
			}
			resultLock.Lock()
			defer resultLock.Unlock()

			log.Infof("[%6s] %s://%s cpe: %v", result.State, result.GetProto(), utils.HostPort(result.Target, result.Port), result.GetCPEs())
			res = append(res, result)
		}

		for _, host := range utils.ParseStringToHosts(c.String("hosts")) {
			host := host
			for _, tcpPort := range utils.ParseStringToPorts(c.String("port")) {
				tcpPort := tcpPort

				portSwg.Add()
				go scanCore(
					host, tcpPort,
					fp.WithForceEnableWebFingerprint(true),
					fp.WithOnlyEnableWebFingerprint(c.Bool("web")),
					fp.WithTransportProtos(fp.TCP),
				)
			}

			for _, udpPort := range utils.ParseStringToPorts(c.String("udp-port")) {
				udpPort := udpPort

				portSwg.Add()
				go scanCore(host, udpPort, fp.WithDisableWebFingerprint(true),
					fp.WithTransportProtos(fp.UDP))
			}

		}
		portSwg.Wait()

		analysis := fp.MatcherResultsToAnalysis(res)

		analysis.Show()
		analysis.ToJson(c.String("json"))

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		return
	}
}
