// Package cmd
// @Author bcy2007  2023/3/23 10:50
package main

import (
	"encoding/json"
	"fmt"
	"github.com/urfave/cli"
	"os"
	"strings"
	"time"
	"yaklang/common/crawlerx/config"
	"yaklang/common/crawlerx/newcrawlerx"
	"yaklang/common/log"
	"yaklang/common/utils"
)

func run(targetURL, proxy, path string) {
	opts := make([]newcrawlerx.ConfigOpt, 0)
	browserInfo := newcrawlerx.NewBrowserInfo{}
	if proxy != "" {
		browserInfo.ProxyAddress = proxy
	}
	if path != "" {
		browserInfo.ExePath = path
	}
	jsonStr, _ := json.Marshal(browserInfo)
	browserInfoStr := string(jsonStr)
	if browserInfoStr != "{}" {
		opts = append(opts, newcrawlerx.WithNewBrowser(browserInfoStr))
	}
	channel := newcrawlerx.StartCrawler(targetURL, opts...)
	for item := range channel {
		fmt.Println(item.Method() + " " + item.Url())
	}
	time.Sleep(2 * time.Second)
}

func do() {
	app := cli.NewApp()
	app.Name = "CrawlerX"
	app.Usage = ""
	app.Version = "v0.2"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "url,u",
			Usage: "crawler target url",
		},
		cli.StringFlag{
			Name:  "proxy",
			Usage: "crawler proxy address",
		},
		cli.StringFlag{
			Name:  "path",
			Usage: "Google Chrome app path",
		},
		cli.StringFlag{
			Name:  "form",
			Usage: "",
		},
		cli.StringFlag{
			Name:  "file,f",
			Usage: "read crawler from file",
		},
		cli.StringFlag{
			Name:  "out,o",
			Usage: "result export file",
		},
	}
	app.Action = func(c *cli.Context) error {
		url := c.String("url")
		if url == "" {
			return utils.Errorf("\nempty target url.\nplease read help for instruction.")
		}
		file := c.String("file")
		var opts []newcrawlerx.ConfigOpt
		if file != "" {
			opts = loadFromFile(file)
			//log.Errorf("not work now.")
			//return nil
		} else {
			proxy := c.String("proxy")
			path := c.String("path")
			form := c.String("form")
			opts = make([]newcrawlerx.ConfigOpt, 0)
			opts = append(opts,
				generateBrowserOpt(proxy, path),
				generatorFormOpt(form),
				generateBlackList("logout", "captcha"),
			)
		}
		channel := newcrawlerx.StartCrawler(url, opts...)
		exportFile := c.String("out")
		if exportFile == "" {
			for item := range channel {
				fmt.Println(item.Method() + " " + item.Url())
			}
			log.Info("output channel down.")
		} else {
			result := make([]*newcrawlerx.OutputResult, 0)
			for item := range channel {
				fmt.Println(item.Method() + " " + item.Url())
				output := newcrawlerx.GeneratorOutput(item)
				result = append(result, output)
			}
			time.Sleep(2 * time.Second)
			resultBytes, _ := json.MarshalIndent(result, "", "\t")
			newcrawlerx.WriteFile(exportFile, resultBytes)
		}
		return nil
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Errorf("simulator cmd running error: %s", err)
		return
	}
}

func generateBrowserOpt(proxy, path string) newcrawlerx.ConfigOpt {
	browserInfo := newcrawlerx.NewBrowserInfo{}
	if proxy != "" {
		browserInfo.ProxyAddress = proxy
	}
	if path != "" {
		browserInfo.ExePath = path
	}
	jsonStr, _ := json.Marshal(browserInfo)
	browserInfoStr := string(jsonStr)
	return newcrawlerx.WithNewBrowser(browserInfoStr)
}

func generateBlackList(keyword ...string) newcrawlerx.ConfigOpt {
	return newcrawlerx.WithBlackList(keyword...)
}

func generatorFormOpt(form string) newcrawlerx.ConfigOpt {
	return func(config *newcrawlerx.Config) {

	}
}

func main() {
	//fmt.Println(lineA)
	//fmt.Println(lineB)
	//fmt.Println(lineC)
	//fmt.Println(lineD)
	//fmt.Println(lineE)
	//fmt.Println(lineF)
	do()
	//opts := loadFromFile("/Users/chenyangbao/Project/crawlerx/cmd/param.ini")
	//log.Info(opts)
}

func loadFromFile(filePath string) []newcrawlerx.ConfigOpt {
	_, err := os.Stat(filePath)
	if err != nil {
		panic(err)
	}
	opts := make([]newcrawlerx.ConfigOpt, 0)
	conf, err := config.LoadConfigFile(filePath)
	if err != nil {
		panic(err)
	}
	wsAddress, _ := conf.GetValue("base", "wsAddress")
	exePath, _ := conf.GetValue("base", "exePath")
	proxy, _ := conf.GetValue("base", "proxy")
	proxyUsername, _ := conf.GetValue("base", "proxyUsername")
	proxyPassword, _ := conf.GetValue("base", "proxyPassword")
	newBrowserInfo := &newcrawlerx.NewBrowserInfo{
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
	opts = append(opts,
		newcrawlerx.WithNewBrowser(string(browserBytes)),
		newcrawlerx.WithFormFill(getMapFromString(formFill)),
		newcrawlerx.WithFileInput(getMapFromString(fileUpload)),
		newcrawlerx.WithBlackList(getSliceFromString(blackList)...),
		newcrawlerx.WithWhiteList(getSliceFromString(whiteList)...),
		newcrawlerx.WithVueWeb(true),
	)
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

var AcsII = `
###   ## ######## ######## ######## #####  # ######## ########    #   #
##  #  # ######## ######## ######## ####  ## ######## ########   ##  ##
#  ##### # #   ## ###  ### #  ###   ####  ## ###  ### # #   ## #    ###
#  #####    ##  # ## #  ##    # #   ###  ### ## ## ##    ##  # ##  ####
#  ##### #  ##### #  #  ## #        ###  ### #    ### #  ##### #    ###
#  ### # #  ##### #  #  ## ##     # ###  ### #  ### # #  #####   ##  ##
##    ## #  ##### ##  #  # ##  #  # ####  ## ##    ## #  #####    #   #
######## ######## ######## ######## ######## ######## ######## ########
`

var lineA = " _____                             _                __   __ "
var lineB = "/  __ \\                           | |               \\ \\ / / "
var lineC = "| /  \\/  _ __    __ _  __      __ | |   ___   _ __   \\ V / "
var lineD = "| |     | '__|  / _` | \\ \\ /\\ / / | |  / _ \\ | '__|  /   \\ "
var lineE = "| \\__/\\ | |    | (_| |  \\ V  V /  | | |  __/ | |    / /^\\ \\ "
var lineF = "\\_____/ |_|     \\__,_|   \\_/\\_/   |_|  \\___| |_|    \\/   \\/ "
