// Package cmd
// @Author bcy2007  2023/7/14 11:11
package main

import (
	"encoding/json"
	"fmt"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/crawlerx"
	"github.com/yaklang/yaklang/common/crawlerx/tools"
	"github.com/yaklang/yaklang/common/crawlerx/tools/config"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	fmt.Println(logo)
	appCreate()
}

func appCreate() {
	app := cli.NewApp()
	app.Name = "CrawlerX"
	app.Usage = `url crawler based on browser simulated click`
	app.Version = "v0.3"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "url,u",
			Usage: "crawler target url",
		},
		cli.StringFlag{
			Name:  "file,f",
			Usage: "read crawler from file",
		},
		cli.BoolFlag{
			Name:  "default-file-path",
			Usage: "default crawler config file path",
		},
		cli.StringFlag{
			Name:  "out,o",
			Usage: "result export file",
		},
	}
	app.Action = func(c *cli.Context) error {
		url := c.String("url")
		defaultFile := c.Bool("default-file-path")
		file := c.String("file")
		output := c.String("out")

		if url == "" {
			return utils.Errorf(`EMPTY target url. Please read help for instruction.`)
		}
		opts := make([]crawlerx.ConfigOpt, 0)
		if defaultFile {
			file = "/Users/chenyangbao/Project/yaklang/common/crawlerx/cmd/param.ini"
		}
		if file != "" {
			opts = append(opts, loadFromFile(file)...)
		}
		ch, err := crawlerx.StartCrawler(url, opts...)
		if err != nil {
			log.Error(err)
			return nil
		}
		if output == "" {
			for item := range ch {
				log.Infof(item.Method() + " " + item.Url() + " from " + item.From())
			}
			log.Infof(`output channel down.`)
		} else {
			result := make([]*crawlerx.OutputResult, 0)
			for item := range ch {
				log.Infof(item.Method() + " " + item.Url())
				result = append(result, crawlerx.GeneratorOutput(item))
			}
			time.Sleep(2 * time.Second)
			resultBytes, _ := json.MarshalIndent(result, "", "\t")
			err := tools.WriteFile(output, resultBytes)
			if err != nil {
				log.Errorf(`Write crawler result to file %s error: %s`, output, err)
			}
		}
		return nil
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Errorf(`CrawlerX cmd start error: %s`, err)
		return
	}
}

func loadFromFile(filePath string) []crawlerx.ConfigOpt {
	_, err := os.Stat(filePath)
	if err != nil {
		panic(err)
	}
	opts := make([]crawlerx.ConfigOpt, 0)
	conf, err := config.LoadConfigFile(filePath)
	if err != nil {
		panic(err)
	}
	wsAddress, _ := conf.GetValue("base", "wsAddress")
	exePath, _ := conf.GetValue("base", "exePath")
	proxy, _ := conf.GetValue("base", "proxy")
	proxyUsername, _ := conf.GetValue("base", "proxyUsername")
	proxyPassword, _ := conf.GetValue("base", "proxyPassword")
	newBrowserInfo := &crawlerx.BrowserInfo{
		WsAddress:     wsAddress,
		ExePath:       exePath,
		ProxyAddress:  proxy,
		ProxyPassword: proxyPassword,
		ProxyUsername: proxyUsername,
	}
	browserBytes, _ := json.Marshal(newBrowserInfo)
	formFill, _ := conf.GetValue("crawler", "formFill")
	fileUpload, _ := conf.GetValue("crawler", "fileUpload")
	blackList, _ := conf.GetValue("crawler", "blackList")
	whiteList, _ := conf.GetValue("crawler", "whiteList")
	sensitiveWord, _ := conf.GetValue("crawler", "sensitiveWord")
	maxDepth, _ := conf.GetValue("crawler", "maxDepth")
	leakless, _ := conf.GetValue("crawler", "leakless")
	//log.Info(vueBool)
	opts = append(opts,
		crawlerx.WithBrowserInfo(string(browserBytes)),
		crawlerx.WithFormFill(getMapFromString(formFill)),
		crawlerx.WithFileInput(getMapFromString(fileUpload)),
		crawlerx.WithBlackList(getSliceFromString(blackList)...),
		crawlerx.WithWhiteList(getSliceFromString(whiteList)...),
		crawlerx.WithExtraWaitLoadTime(1000),
		crawlerx.WithSensitiveWords(getSliceFromString(sensitiveWord)),
		crawlerx.WithLeakless(leakless),
	)
	maxDepthInt, err := strconv.Atoi(maxDepth)
	if err == nil {
		//log.Infof(`Config read max depth %d`, maxDepthInt)
		opts = append(opts, crawlerx.WithMaxDepth(maxDepthInt))
	}
	return opts
}

func getMapFromString(mapStr string) map[string]string {
	result := make(map[string]string)
	if mapStr == "" {
		return result
	}
	if !strings.Contains(mapStr, ":") {
		return result
	}
	items := strings.Split(mapStr, ";")
	for _, item := range items {
		if !strings.Contains(item, ":") {
			continue
		}
		values := strings.Split(item, ":")
		result[values[0]] = values[1]
	}
	return result
}

func getSliceFromString(sliceStr string) []string {
	if sliceStr == "" {
		return make([]string, 0)
	}
	result := strings.Split(sliceStr, ";")
	return result
}

const logo = " _____                             _                __   __ " + "\n" +
	"/  __ \\                           | |               \\ \\ / / " + "\n" +
	"| /  \\/  _ __    __ _  __      __ | |   ___   _ __   \\ V / " + "\n" +
	"| |     | '__|  / _` | \\ \\ /\\ / / | |  / _ \\ | '__|  /   \\ " + "\n" +
	"| \\__/\\ | |    | (_| |  \\ V  V /  | | |  __/ | |    / /^\\ \\ " + "\n" +
	"\\_____/ |_|     \\__,_|   \\_/\\_/   |_|  \\___| |_|    \\/   \\/ "
