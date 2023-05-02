package yakgrpc

import (
	"context"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const mitmPluginTestCode = `yakit.AutoInitYakit()
loglevel("info")

target = cli.String("target")
pluginName := cli.String("plugin-name")
yakit.Info("开始独立执行 MITM 插件:%v" % pluginName)

if target == "" {
    die("无法进行插件扫描，当前插件对应目标不存在：%v" % target)
}
yakit.Info("扫描目标: %v" % target)

manager, err = hook.NewMixPluginCaller()
if err != nil {
    yakit.Error("创建插件管理模块失败：%v", err)
    die("插件管理模块无法创建")
}

err = manager.LoadPlugin(pluginName)
if err != nil {
    reason = "无法加载插件：%v 原因是：%v" % [pluginName, err]
    yakit.Error(reason)
    die(reason)
}
manager.SetDividedContext(true)
manager.SetConcurrent(20)
defer manager.Wait()

res, err = crawler.Start(target, crawler.maxRequest(10),crawler.disallowSuffix([]))
if err != nil {
    reason = "无法进行基础爬虫：%v" % err
    yakit.Error(reason)
    die(reason)
}

for req = range res {
    yakit.Info("检查URL:%v", req.Url())
    manager.MirrorHTTPFlow(req.IsHttps(), req.Url(), req.RequestRaw(), req.ResponseRaw(), req.ResponseBody())
}
`

func (s *Server) generateMITMTask(pluginName string, ctx ypb.Yak_ExecServer, params []*ypb.ExecParamItem) error {
	params = append(params, &ypb.ExecParamItem{
		Key:   "plugin-name",
		Value: pluginName,
	})
	return s.Exec(&ypb.ExecRequest{
		Params: params,
		Script: mitmPluginTestCode,
	}, ctx)
}

func execTestCaseMITMHooksCaller(rootCtx context.Context, y *yakit.YakScript, params []*ypb.ExecParamItem, db *gorm.DB, streamFeedback func(r *ypb.ExecResult) error) error {
	ctx, cancel := context.WithCancel(rootCtx)
	defer cancel()

	manager := yak.NewYakToCallerManager()
	err := manager.AddForYakit(
		ctx, y.ScriptName, params, y.Content,
		yak.YakitCallerIf(func(result *ypb.ExecResult) error {
			return streamFeedback(result)
		}),
		append(enabledHooks, "__test__")...)
	if err != nil {
		log.Errorf("load mitm hooks code failed: %s", err)
		return utils.Errorf("load mitm failed: %s", err)
	}

	go func() {
		select {
		case <-ctx.Done():
			log.Infof("call %v' clear ", y.ScriptName)
			manager.CallByName("clear")
		}
	}()

	log.Infof("call %v' __test__ ", y.ScriptName)
	manager.CallByName("__test__")
	cancel()

	return nil
}
