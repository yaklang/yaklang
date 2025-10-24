package scanfpcmd

import (
	"context"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bruteutils"
	"os"
)

var BruteUtil = cli.Command{
	Name: "brute",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name: "target,t",
		},
		cli.StringFlag{
			Name:  "username,u",
			Value: "dataex/dicts/user.txt",
		},
		cli.StringFlag{
			Name:  "password,p",
			Value: "dataex/dicts/3389.txt",
		},
		cli.IntFlag{
			Name:  "min-delay",
			Value: 1,
		},
		cli.IntFlag{
			Name:  "max-delay",
			Value: 2,
		},
		cli.IntFlag{
			Name:  "target-concurrent",
			Value: 200,
		},
		cli.StringFlag{
			Name: "type,x",
		},
		cli.BoolFlag{
			Name:  "ok-to-stop",
			Usage: "如果一个目标发现了成功的结果，则停止对这个目标的爆破",
		},
		cli.IntFlag{
			Name:  "finished-to-end",
			Usage: "爆破的结果如果多次显示'Finished' 就停止爆破，这个选项控制阈值",
			Value: 10,
		},
		cli.StringFlag{
			Name:  "divider",
			Usage: "用户(username), 密码(password)，输入的分隔符，默认是（,）",
			Value: ",",
		},
	},

	Action: func(c *cli.Context) error {
		bruteFunc, err := bruteutils.GetBruteFuncByType(c.String("type"))
		if err != nil {
			return err
		}

		bruter, err := bruteutils.NewMultiTargetBruteUtil(
			c.Int("target-concurrent"), c.Int("min-delay"), c.Int("max-delay"),
			bruteFunc,
		)
		if err != nil {
			return err
		}

		bruter.OkToStop = c.Bool("ok-to-stop")
		bruter.FinishingThreshold = c.Int("finished-to-end")

		var succeedResult []*bruteutils.BruteItemResult

		userList := bruteutils.FileOrMutateTemplate(c.String("username"), c.String("divider"))
		err = bruter.StreamBruteContext(
			context.Background(), c.String("type"),
			bruteutils.FileOrMutateTemplateForStrings(c.String("divider"), utils.ParseStringToHosts(c.String("target"))...),
			userList,
			bruteutils.FileOrMutateTemplate(c.String("password"), c.String("divider")),
			func(b *bruteutils.BruteItemResult) {
				if b.Ok {
					succeedResult = append(succeedResult, b)
					log.Infof("Success for target: %v user: %v pass: %s", b.Target, b.Username, b.Password)
				} else {
					log.Warningf("failed for target: %v user: %v pass: %s", b.Target, b.Username, b.Password)
				}
			},
		)
		if err != nil {
			return err
		}

		log.Info("------------------------------------------------")
		log.Info("------------------------------------------------")
		log.Info("------------------------------------------------")

		if len(succeedResult) <= 0 {
			log.Info("没有爆破到可用结果")
			return nil
		}

		table := tablewriter.NewWriter(os.Stdout)
		table.Header([]string{
			"服务类型", "目标", "用户名", "密码",
		})
		for _, i := range succeedResult {
			if i.OnlyNeedPassword {
				table.Append([]string{
					c.String("type"),
					i.Target,
					"",
					i.Password,
				})
			} else {
				table.Append([]string{
					c.String("type"),
					i.Target,
					i.Username,
					i.Password,
				})
			}
		}
		table.Render()

		return nil
	},
}
