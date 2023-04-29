package yakgrpc

var (
	generalBatchExecutor = `loglevel("info")
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
newPlugins = cli.YakitPlugin()
if len(newPlugins) > 0 {
    plugins = newPlugins 
}
filter = str.NewFilter()

// plugins = ["FastJSON 漏洞检测 via DNSLog"]

/*
    load plugin
*/
yakit.SetProgress(0.05)
manager, err := hook.NewMixPluginCaller()
if err != nil {
    yakit.Error("create yakit plugin caller failed: %s", err)
    die(err)
}
manager.SetConcurrent(20)
manager.SetDividedContext(true) 

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
// targetRaw = "http://192.168.101.177:8084/"
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

// 限制并发
swg = sync.NewSizedWaitGroup(1)

handleUrl = func(t) {
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
        rspHeader, _ = http.dumphead(rspIns)
        rspBody = result.ResponseBody()
        responseRaw = str.ReplaceHTTPPacketBody(rspHeader, rspBody, false)
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
        res, err = servicescan.Scan(host, ports, servicescan.proxy(proxy...))
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
    if len(urls) <= 0 {
        handleTarget(target, true)
    }
    handleTarget(target, false)
}

// 等待所有插件执行完毕
swg.Wait()
manager.Wait()`
)

var (
	nucleiExecutor = `target := cli.String("target", cli.setRequired(true))
pocFile := cli.String("pocFile", cli.setRequired(true))
isWorkflow = cli.Bool("isWorkflow")
debug = cli.Bool("debug")
proxy = cli.StringSlice("proxy")
cli.check()

client = yakit.NewClient(cli.String("yakit-webhook"))
log.info("当前默认 yakit-webhook 为：%v", cli.String("yakit-webhook"))
client.OutputLog("info", "参数检查成功。")

opts = [
    nuclei.debug(debug),
    nuclei.verbose(debug),
    nuclei.noColor(true),
    nuclei.severity("info", "low", "medium", "high", "critical"),
]

if len(proxy) > 0 {
    opts = append(opts, nuclei.proxy(proxy...))
}

if (debug) {
    log.setLevel("debug")
}

if debug {
    client.OutputLog("info", "构建基础扫描参数：调试模式")
} else {
    client.OutputLog("info", "未开启调试模式")
}

if !isWorkflow {
    client.OutputLog("info", "当前执行模式为 PoC 模式：%v", pocFile)
    opts = append(opts, nuclei.templates(pocFile))
}else{
    client.OutputLog("info", "当前执行模式为 workflows 模式：%v", pocFile)
    opts = append(opts, nuclei.workflows(pocFile))
}

client.OutputLog("info", "开始针对目标：%v，进行漏洞检测", target)
r, err := nuclei.Scan(target, opts...)
die(err)

for a = range r {
    client.OutputLog("success", "监测到漏洞/风险[%v]：%v from: %v", a.PocName, a.Severity, a.Target)
    client.OutputLog("json", a.RawJson)
	client.Output(nuclei.PocVulToRisk(a))
}

client.OutputLog("end", "进程正常结束")
`
)
