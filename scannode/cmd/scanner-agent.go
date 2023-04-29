package main

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/google/go-github/v33/github"
	"github.com/urfave/cli"
	"golang.org/x/oauth2"
	"io/ioutil"
	"os"
	"os/signal"
	"yaklang/common/fp"
	"yaklang/common/log"
	"yaklang/common/spec"
	"yaklang/common/utils"
	"yaklang/common/utils/spacengine/go-shodan"
	"yaklang/common/yak"
	"yaklang/common/yak/yaklib"
	"yaklang/common/yakgrpc"
	"yaklang/scannode"
	"yaklang/scannode/scanrpc"
	"yaklang/server/dbm/falcons"
	"yaklang/server/dbm/visualization"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	sigExitOnce = new(sync.Once)
)

func init() {
	os.Setenv("YAKMODE", "vm")
	go sigExitOnce.Do(func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
		defer signal.Stop(c)

		for {
			select {
			case <-c:
				fmt.Printf("exit by signal [SIGTERM/SIGINT/SIGKILL]")
				os.Exit(1)
				return
			}
		}
	})
}

func main() {
	app := cli.NewApp()

	app.Commands = []cli.Command{
		{
			Name: "gitsearch",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name: "k",
				},
				cli.StringFlag{
					Name: "token,t",
				},
			},
			Action: func(c *cli.Context) {
				ctx := context.Background()
				ts := oauth2.StaticTokenSource(
					&oauth2.Token{AccessToken: c.String("token")},
				)
				tc := oauth2.NewClient(ctx, ts)

				client := github.NewClient(tc)
				res, r, err := client.Search.Code(
					utils.TimeoutContext(10*time.Second),
					c.String("k"), &github.SearchOptions{
						Sort:      "",
						Order:     "",
						TextMatch: true,
						ListOptions: github.ListOptions{
							Page: 1, PerPage: 1,
						},
					},
				)
				if err != nil {
					log.Error(err)
					return
				}
				_ = r
				for _, r := range res.CodeResults {
					spew.Dump(falcons.ConvertToFalconGitLeakRecordFromGithubCodeResult(r))
					//spew.Dump(r.Repository)
					//spew.Dump(r.Repository.URL)
					//spew.Dump(r.Repository.GetGitURL())
					//raw, err := json.Marshal(r.Repository)
					//_ = err
					//var reposInfo github.Repository
					//_ = json.Unmarshal(raw, &reposInfo)
					//spew.Dump(reposInfo.Owner.GetLogin())
					//spew.Dump(
					//	r.GetHTMLURL(),
					//)
					//for _, text := range r.TextMatches {
					//	spew.Dump(bizhelper.Str(text.Fragment))
					//}
				}
			},
		},
		{
			Name: "test-dh",
			Action: func(c *cli.Context) error {
				dh, err := visualization.NewDateHeatmap()
				if err != nil {
					return err
				}
				spew.Dump(dh.Elements)
				return nil
			},
		},
		{
			Name: "ip-whois",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name: "ip",
				},
			},
			Action: func(c *cli.Context) {
				res, err := yaklib.QueryIPForISP(c.String("ip"))
				if err != nil {
					log.Error(err)
					return
				}
				spew.Dump(res)
			},
		},
		{
			Name: "quake",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "key"},
				cli.StringFlag{Name: "filter,f"},
			},
			Action: func(c *cli.Context) {
				client := utils.NewQuake360Client(c.String("key"))
				rsp, err := client.QueryNext(c.String("filter"))
				if err != nil {
					log.Errorf("query quake failed: %s", err)
					return
				}
				spew.Dump(rsp)
			},
		},
		{
			Name: "shodan",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name: "key",
				},
				cli.StringFlag{
					Name: "filter,f",
				},
			},
			Action: func(c *cli.Context) error {
				client := shodan.shodan.New(c.String("key"))
				info, err := client.APIInfo()
				if err != nil {
					return err
				}
				spew.Dump(info)
				hosts, err := client.HostSearch(c.String("f"), nil, map[string][]string{
					//"page": {"1"},
					"limit": {"1"},
				})
				if err != nil {
					return err
				}
				data := hosts.Matches
				for index, d := range data {
					var short string
					var maxLength = 1000
					if len(d.Data) > maxLength {
						short = d.Data[0:maxLength]
					} else {
						short = d.Data
					}
					ip, port := utils.Uint32ToIPv4(uint32(d.IP)), d.Port
					log.Infof("fetch: %s op: %v",
						utils.HostPort(ip.String(), port), short,
					)
					spew.Dump(d)
					_ = index
				}

				return nil
			},
		},
		{
			Name: "scan-fp",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name: "hosts,t",
				},
				cli.StringFlag{
					Name:  "ports,p,port",
					Value: "80-82,8080-8082,8088,8888,443,8443,3389,22,21",
				},
				cli.DurationFlag{
					Name:  "timeout",
					Value: 60 * time.Minute,
				},
			},
			Action: func(c *cli.Context) error {
				targets := make(chan *fp.PoolTask)
				pool, err := fp.NewExecutingPool(
					utils.TimeoutContext(c.Duration("timeout")),
					10,
					targets, nil,
				)
				if err != nil {
					return utils.Errorf("create fp pool failed: %s", err)
				}
				pool.AddCallback(func(matcherResult *fp.MatchResult, err error) {
					if err != nil {
						return
					}
					switch matcherResult.State {
					case fp.OPEN:
						log.Info(matcherResult.String())
					}
				})
				go func() {
					defer close(targets)
					for _, host := range utils.ParseStringToHosts(c.String("hosts")) {
						for _, port := range utils.ParseStringToPorts(c.String("ports")) {
							targets <- &fp.PoolTask{
								Host: host,
								Port: port,
							}
						}
					}
				}()
				err = pool.Run()
				if err != nil {
					return err
				}
				return nil
			},
		},
		{
			Name: "script",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name: "target",
				},
				cli.StringFlag{
					Name: "script,f",
				},
			},
			Action: func(c *cli.Context) error {
				log.Infof("use script file: %s", c.String("script"))
				raw, err := ioutil.ReadFile(c.String("script"))
				if err != nil {
					return err
				}

				log.Infof("create script engine")
				engine := yak.NewScriptEngine(2)

				log.Infof("start to execute: \n\n%v\n\n", string(raw))
				err = engine.ExecuteWithTemplate(string(raw), map[string][]string{
					"url": {"http://www.baidu.com"},
				})
				if err != nil {
					return utils.Errorf("execute[\n%v\n] failed: %s", string(raw), err)
				}

				return nil
			},
		},
		{
			Name: "distyak",
			Action: func(c *cli.Context) error {
				var err error
				args := c.Args()
				if len(args) > 0 {
					// args 被解析到了，说明后面跟着文件，去读文件出来吧
					file := args[0]
					if file != "" {
						var absFile = file
						if !filepath.IsAbs(absFile) {
							absFile, err = filepath.Abs(absFile)
							if err != nil {
								return utils.Errorf("fetch abs file path failed: %s", err)
							}
						}
						raw, err := ioutil.ReadFile(file)
						if err != nil {
							return err
						}

						engine := yak.NewScriptEngine(100)
						engine.HookOsExit()
						err = engine.ExecuteMain(string(raw), absFile)
						if err != nil {
							return err
						}

						return nil
					} else {
						return utils.Errorf("empty yak file")
					}
				}

				code := c.String("code")
				engine := yak.NewScriptEngine(100)
				engine.HookOsExit()
				err = engine.Execute(code)
				if err != nil {
					return err
				}
				return nil
			},
			Flags: []cli.Flag{
				cli.StringFlag{
					Name: "code,c",
				},
			},
			SkipFlagParsing: true,
		},
	}

	app.Flags = append(
		[]cli.Flag{
			cli.BoolFlag{
				Name: "debug",
			},
		},
		spec.GetCliBasicConfig("scanner")...,
	)

	app.Before = func(context *cli.Context) error {
		server, err := yakgrpc.NewServer()
		if err != nil {
			return err
		}
		_ = server
		return nil
	}

	app.Action = func(c *cli.Context) error {
		id := c.String("id")
		config := spec.LoadAMQPConfigFromCliContext(c)

		node, err := scannode.NewScanNode(id, config)
		if err != nil {
			return err
		}

		if c.Bool("debug") {
			time.AfterFunc(2*time.Second, func() {
				helper := node.GetServerHelper()
				_ = helper

				_, err = helper.DoSCAN_RadCrawler(
					context.Background(),
					id, &scanrpc.SCAN_RadCrawlerRequest{
						Targets:    []string{"http://172.16.86.132"},
						Proxy:      "",
						EnableXray: true,
						Cookie:     "",
					}, nil,
				)
				if err != nil {
					panic(err)
				}

				//helper.DoSCAN_ScanFingerprint(
				//	context.Background(),
				//	id, &scanrpc.SCAN_ScanFingerprintRequest{
				//		Hosts:          "47.52.100.105/24",
				//		Ports:          "80,8080",
				//		TimeoutSeconds: 5,
				//		Concurrent:     10,
				//	}, nil,
				//)
				//_, err = helper.DoSCAN_BasicCrawler(
				//	context.Background(),
				//	id, &scanrpc.SCAN_BasicCrawlerRequest{
				//		Targets: []string{
				//			"leavesongs.com",
				//		},
				//		EnableXray: true,
				//		Proxy: "",
				//	},
				//	nil,
				//)
				//if err != nil {
				//	log.Error(err)
				//}
				//_, err = helper.DoSCAN_ProxyCollector(
				//	context.Background(),
				//	id, &scanrpc.SCAN_ProxyCollectorRequest{Port: 8088}, nil,
				//)
				//if err != nil {
				//	panic(err)
				//}
			})
		}

		node.Run()
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		return
	}
}
