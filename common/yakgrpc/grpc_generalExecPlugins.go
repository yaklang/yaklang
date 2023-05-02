package yakgrpc

import (
	"io/ioutil"
	"os"
	"yaklang.io/yaklang/common/consts"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/yak/yaklib"
	"yaklang.io/yaklang/common/yakgrpc/yakit"
	"yaklang.io/yaklang/common/yakgrpc/ypb"
)

const __CODE_EXEC_YAKIT_PLUGINS_BY_YAK_SCRIPT_FILTER = `loglevel("error")
yakit.AutoInitYakit()

debug = cli.Have("debug")
if debug {
    yakit.StatusCard("调试模式", "True")
}

concurrent = cli.Int("concurrent")
if concurrent <= 0 { concurrent = 10 }

# 综合扫描
yakit.SetProgress(0.1)
plugins = cli.YakitPlugin()
if len(plugins) <= 0 {
    return 
    plugins = [
        "[thinkphp-5023-rce]: ThinkPHP 5.0.23 RCE", 
        "[CVE-2020-17530]: Apache Struts RCE",
        "[thinkphp-2-rce]: ThinkPHP 2 / 3 's' Parameter RCE",
        "[thinkphp-5022-rce]: ThinkPHP 5.0.22 RCE",
        "[thinkphp-501-rce]: ThinkPHP 5.0.1 RCE",
    ]
}
filter = str.NewFilter()

yakit.StatusCard("Plugins", sprint(len(plugins)))


/*
    load plugin
*/
yakit.SetProgress(0.05)
manager, err := hook.NewMixPluginCaller()
if err != nil {
    yakit.Error("create yakit plugin caller failed: %s", err)
    die(err)
}

x.Foreach(plugins, func(name) {
    err = manager.LoadPlugin(name)
    if err != nil {
        yakit.Info("load plugin [%v] failed: %s", name, err)
        return
    }
    if debug {
		yakit.Info("load plugin [%s] finished", name)
    }
})
pluginCount = len(plugins)

/*
    
*/
yakit.SetProgress(0.05)
ports = cli.String("ports")
if ports == "" {
    ports = "22,21,80,443,3389"
}
targetFiles = cli.String("target-file")
if targetFiles == "" {
    yakit.StatusCard("扫描失败", "目标[FILE]")
    return
}
targetRaw, _ = io.ReadFile(targetFiles)
targetRaw = string(targetRaw)
if targetRaw == "" { ## empty
    yakit.StatusCard("扫描失败", "目标为空")
    return
}

targets = make([]string)
urls = make([]string)
for _, line = range str.ParseStringToLines(targetRaw) {
    line = str.TrimSpace(line)
	if str.IsHttpURL(line) {
		urls = append(urls, line)
	}
	targets = append(targets, line)
}

// if len(targets) <= 0 {
//     targets = ["127.0.0.1:8080"]
// }

// 限制并发
swg = sync.NewSizedWaitGroup(concurrent)

handleUrl = func(t) {
    res, err = crawler.Start(t, crawler.maxRequest(20))
    if err != nil {
        yakit.Error("create crawler failed: %s", err)
        return
    }
    for result = range res {
        resIns, err = result.Response()
        if err != nil {
            yakit.Info("cannot fetch result response: %s", err)
            continue
        }
        responseRaw, _ = http.dump(rspIns)
        manager.MirrorHTTPFlowEx(
            false,
            x.If(
                str.HasPrefix(str.ToLower(result.Url()), "https://"), 
                true, false,
            ), result.Url(), result.RequestRaw(), responseRaw,
            result.ResponseBody(),
        )
    }
}

handlePorts = func(t) {
    host, port, _ = str.ParseStringToHostPort(t)
    if port > 0 {
        host = str.HostPort(host, port)
    }
    if host == "" {
        host = t
    }

    if port < 0 {
        res, err = servicescan.Scan(host, ports)
        if err != nil {
            yakit.Error("servicescan %v failed: %s", t)
            return
        }
        for result = range res {
            println(result.String())
            manager.HandleServiceScanResult(result)
        }
    } else {
        manager.GetNativeCaller().CallByName("execNuclei", t)
    }
}

// 设置计数器，用于反馈进度
totalTask = len(urls) + len(targets)
currentTask = 0
counterLock = sync.NewLock()
currentFinished = func() {
    counterLock.Lock()
    defer counterLock.Unlock()
    currentTask++
    if totalTask <= 0 {
        return
    }
    yakit.SetProgress(0.2 + 0.8 * (float(currentTask) / float(totalTask)))
}
// 设置结果处理方案
handleTarget = func(t, isUrl) {
    hash = codec.Sha256(sprint(t, ports))
    if filter.Exist(hash) {
        currentFinished()
        return
    }
    filter.Insert(hash)
    swg.Add()
    go func{
        defer swg.Done()
        defer currentFinished()
        defer func{
            err = recover()
            if err != nil {
                yakit.Error("panic from: %v", err)
                return
            }
        }
        if isUrl {
            handleUrl(t)
            return
        }
        handlePorts(t)
    }
}

for _, u = range urls {
    handleTarget(u, true)
}
for _, target = range targets {
    handleTarget(target, false)
}

// 等待所有插件执行完毕
swg.Wait()
manager.Wait()
`

func (s *Server) ExecYakitPluginsByYakScriptFilter(req *ypb.ExecYakitPluginsByYakScriptFilterRequest, stream ypb.Yak_ExecYakitPluginsByYakScriptFilterServer) error {
	if req.GetFilter() == nil {
		return utils.Error("empty filter")
	}
	if req.GetFilter().GetNoResultReturn() {
		return utils.Error("no poc selected")
	}

	var db = yakit.FilterYakScript(s.GetProfileDatabase(), req.GetFilter())
	rows, err := db.Rows()
	if err != nil {
		stream.Send(yaklib.NewYakitLogExecResult("error", "filter yak scripts failed: %v", err))
		return nil
	}

	fp, err := ioutil.TempFile(consts.GetDefaultYakitBaseTempDir(), "exec-plugins-by-filter-*.txt")
	if err != nil {
		stream.Send(yaklib.NewYakitLogExecResult("error", "create temp file to list plugin failed: %s", err))
		return nil
	}

	var count int
	for rows.Next() {
		var scriptName string
		err := rows.Scan(&scriptName)
		if err != nil {
			continue
		}
		if count == 0 {
			fp.WriteString(scriptName)
		} else {
			fp.WriteString("|" + scriptName)
		}
		count++
	}
	fp.Close()

	stream.Send(yaklib.NewYakitLogExecResult("info", "log yakit plugin total: %v", count))
	//defer os.RemoveAll(fp.Name())

	// 设置插件文件夹
	var params []*ypb.ExecParamItem = req.GetExtraParams()
	if fp.Name() != "" {
		params = append(params, &ypb.ExecParamItem{Key: "--yakit-plugin-file", Value: fp.Name()})
	}

	// 处理扫描目标
	targetfp, err := ioutil.TempFile(consts.GetDefaultYakitBaseTempDir(), "exec-plugins-by-filter-target-*.txt")
	if err != nil {
		return utils.Errorf("cannot create tempfile for targets: %s", err)
	}
	targetFile := req.GetTargetFile()
	if targetFile != "" {
		raw, _ := ioutil.ReadFile(targetFile)
		if raw != nil {
			targetfp.Write(raw)
			targetfp.WriteString("\n")
		}
	}
	targets := req.GetTarget()
	if targets != "" {
		for _, t := range utils.ParseStringToHosts(targets) {
			if t == "" {
				continue
			}
			targetfp.WriteString(t + "\n")
		}
	}
	targetfp.Close()

	stat, _ := os.Stat(fp.Name())
	if stat != nil {
		if stat.Size() <= 0 {
			return utils.Error("target input file empty")
		}
	}

	if targetfp.Name() != "" {
		params = append(params, &ypb.ExecParamItem{
			Key:   "target-file",
			Value: targetfp.Name(),
		})
	}

	if req.GetPorts() != "" {
		params = append(params, &ypb.ExecParamItem{
			Key:   "ports",
			Value: req.GetPorts(),
		})
	}

	return s.Exec(&ypb.ExecRequest{
		Params: params,
		Script: __CODE_EXEC_YAKIT_PLUGINS_BY_YAK_SCRIPT_FILTER,
	}, stream)
}
