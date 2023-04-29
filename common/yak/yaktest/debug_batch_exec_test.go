package yaktest

import "testing"

func TestBatchExec(t *testing.T) {
	Run("BatchTestDEBUGGER", t, YakTestCase{
		Name: "debug mix caller",
		Src: `
loglevel("info")
yakit.AutoInitYakit()

debug = cli.Have("debug")
if debug {
    yakit.Info("调试模式已开启")
}

proxyStr = str.TrimSpace(cli.String("proxy"))
proxy = make([]string)
if proxyStr != "" {
    proxy = str.SplitAndTrim(proxyStr, ",")
}

# 综合扫描
yakit.SetProgress(0.1)
plugins = [cli.String("plugin")]
filter = str.NewFilter()

plugins = ["FastJSON 漏洞检测 via DNSLog"]

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

/*
    
*/
yakit.SetProgress(0.05)
ports = cli.String("ports")
if ports == "" {
    ports = "22,21,80,443,3389"
}

targetRaw = cli.String("target")
targetRaw = "http://192.168.101.177:8084/"
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
swg = sync.NewSizedWaitGroup(1)

handleUrl = func(t) {
	yakit.Info("开始针对 %v 进行基础爬虫（10 requests）", t)
    res, err = crawler.Start(t, crawler.maxRequest(10), crawler.proxy(proxy...))
    if err != nil {
        yakit.Error("create crawler failed: %s", err)
        return
    }
    for result = range res {
        rspIns, err = result.Response()
        if err != nil {
            yakit.Info("cannot fetch result response: %s", err)
            continue
        }
		println("fetch response from: %s", rspIns)
        responseHeader, _ = http.dumphead(rspIns)
		
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
	yakit.Info("处理目标：%v 插件：%v", t, plugins)
    host, port, _ = str.ParseStringToHostPort(t)
    originHost = ""
    if port > 0 {
        originHost = host
        host = str.HostPort(host, port)
    }
    if host == "" {
        host = t
    }

    if port > 0 {
        result, err = servicescan.ScanOne(originHost, port, servicescan.probeTimeout(10), servicescan.proxy(proxy...))
        if err != nil {
            yakit.Info("指定端口：%v 不开放", host)
            return
        }
        manager.HandleServiceScanResult(result)
        manager.GetNativeCaller().CallByName("execNuclei", t)
    }

    if port <= 0 {
        yakit.Info("开始扫描端口：%v", t)
        res, err = servicescan.Scan(host, ports)
        if err != nil {
            yakit.Error("servicescan %v failed: %s", t)
            return
        }
        for result = range res {
            println(result.String())
            manager.HandleServiceScanResult(result)
        }
    }
}

// 设置结果处理方案
handleTarget = func(t, isUrl) {
    hash = codec.Sha256(sprint(t, ports, isUrl))
    if filter.Exist(hash) {
        return
    }
    filter.Insert(hash)
    swg.Add()
    go func{
        defer swg.Done()
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
`,
	})
}
