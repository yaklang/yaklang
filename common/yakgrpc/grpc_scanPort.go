package yakgrpc

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const scanPort = `yakit.AutoInitYakit()
hostFile = cli.String("target-file", cli.setRequired(true))
ports = cli.String("ports", cli.setDefault("22,443,445,80,8000-8004,3306,3389,5432,8080-8084,7000-7005"))
mode = cli.String("mode", cli.setDefault("fingerprint"))
saveToDB = cli.Bool("save-to-db")
saveClosed = cli.Bool("save-closed-ports") 
proxies = cli.String("proxy", cli.setDefault("no"))
probeTimeoutFloat = cli.Float("probe-timeout", cli.setDefault(5.0), cli.setRequired(false))
probeMax = cli.Int("probe-max", cli.setRequired(false), cli.setDefault(4))

// host alive scan
skippedHostAliveScan = cli.Bool("skipped-host-alive-scan")
hostAliveConcurrent = cli.Int("host-alive-concurrent", cli.setDefault(20), cli.setRequired(false))
hostAliveTimeout = cli.Float("host-alive-timeout", cli.setDefault(5.0), cli.setRequired(false))
hostAliveTCPPorts = cli.String("host-alive-ports", cli.setDefault("80,22,443"), cli.setRequired(false))

if proxies == "no" {
	proxies = ""
}

active = cli.Bool("active")
concurrent = cli.Int("concurrent", cli.setDefault(50))
synConcurrent = cli.Int("syn-concurrent", cli.setDefault(800))
protos = cli.String("proto", cli.setDefault("tcp"))

fpMode = cli.String("fp-mode", cli.setDefault("all"))
scriptNameFile = cli.String("script-name-file", cli.setDefault(""))
cli.check()

hostRaw, _ = file.ReadFile(hostFile)
hosts = string(hostRaw)

excludeHosts = cli.String("exclude-hosts")
excludePorts = cli.String("exclude-ports")
enableCrawler = cli.Have("enable-basic-crawler")
crawlerMaxRequest = cli.Int("basic-crawler-request-max")

if hosts == "" {
	die("target / hosts empty")
}

yakit.SetProgress(0.1)

hostTotal = len(str.ParseStringToHosts(str.Trim(hosts, ",")))
portTotal = len(str.ParseStringToPorts(str.Trim(ports, ",")))
// yakit.StatusCard("扫描主机数", hostTotal)
// yakit.StatusCard("扫描端口数", portTotal)
totalTasks = hostTotal * portTotal
portResultFinalTotal = 0
progressLock = sync.NewLock()
updateProgress = func(delta) {
    if totalTasks <= 0 {
        return
    }
	progressLock.Lock()
    defer progressLock.Unlock()

    portResultFinalTotal = portResultFinalTotal+delta
    if portResultFinalTotal > totalTasks {
        portResultFinalTotal = totalTasks
    }
    // yakit.StatusCard("已出结果", sprintf("%v/%v", portResultFinalTotal, totalTasks))
    yakit.SetProgress(0.1 + (float(portResultFinalTotal) / float(totalTasks) ) * 0.9)
}
defer yakit.SetProgress(1)

opts = []
opts = append(opts, servicescan.active(active))

if concurrent > 0 {
    opts = append(opts, servicescan.concurrent(concurrent))
}

if protos != "" {
    protoList = str.Split(protos, ",")
	printf("PROTO: %#v\n", protos)
    opts = append(opts, servicescan.proto(protoList...))
}

// 使用指纹检测规则条数
if probeMax > 0 {
	opts = append(opts, servicescan.maxProbes(probeMax))
} else {
	opts = append(opts, servicescan.maxProbes(3))
}

if proxies != "" {
    proxyList = str.Split(proxies, ",")
    printf("PROXY: %v\n", proxyList)
    opts = append(opts, servicescan.proxy(proxyList...))
}

if probeTimeoutFloat > 0 {
    opts = append(opts, servicescan.probeTimeout(probeTimeoutFloat))
}

if fpMode == "web" {
	opts = append(opts, servicescan.web())
}

if fpMode == "service" {
	opts = append(opts, servicescan.service())
}

if fpMode == "all" {
	opts = append(opts, servicescan.all())
}

// 设置排除端口和主机
if excludePorts != "" {
    yakit.Info("设置排除端口：%v", excludePorts)
    opts = append(opts, servicescan.excludePorts(excludePorts))
}
if excludeHosts != "" {
    yakit.Info("设置排除主机：%v", excludeHosts)
    opts = append(opts, servicescan.excludeHosts(excludeHosts))
}

if !enableCrawler {
    yakit.Info("禁用基础爬虫")
}

/*
Loading Plugins 
*/
scriptNames = x.If(scriptNameFile != "", x.Filter(
    x.Map(
        str.ParseStringToLines(string(file.ReadFile(scriptNameFile)[0])), 
        func(e){return str.TrimSpace(e)},
    ), func(e){return e!=""}), make([]string))

scriptNameList = str.Join(x.Map(scriptNames, func(i) {
    // 0x60 反引号
    return "1. \x60" + sprint(i) + "\x60"
}), "\n")

yakit.Info("Preparing For Loading Plugins：%v", len(scriptNames))
manager, err = hook.NewMixPluginCaller()
if err != nil {
    yakit.Error("build mix plugin caller failed: %s", err)
    die(err)
}
// 这个有必要设置：独立上下文，避免在大并发的时候出现问题
manager.SetConcurrent(20)
manager.SetDividedContext(true) 
x.Foreach(scriptNames, func(e){
    yakit.Info("Start to Load Plugin: %v", e)
    err = manager.LoadPlugin(e)
    if err != nil {
        yakit.Error("load plugin[%v] error: %v", e, err)
    }
    println(e + " Is Loaded")
})
defer manager.Wait()

// handle Result
handleMITMPluginCaller = func(crawlerReq) {
    go func{
		defer func{
			err = recover()
			if err != nil { yakit.Error("handle plugin result failed: %s", err) }
		}

		rspIns, err = crawlerReq.Response()
		if err != nil {
			yakit.Error("cannot fetch response for %s", url)
			return
		}
		url = crawlerReq.Url()
		body = crawlerReq.ResponseBody()
		req = crawlerReq.RequestRaw()
		isHttps = x.If(str.HasPrefix(url, "https://"), true, false)
		rsp, _ = http.dumphead(rspIns)
		rsp = str.ReplaceHTTPPacketBody(rsp, body, false)
		manager.MirrorHTTPFlowEx(false, isHttps, url, req, rsp, body)
    }
}

handleCrawler = func(result) {
    defer func{
        err = recover()
        if err != nil { yakit.Error("call crawler error: %s", err) }
    }

	if (!enableCrawler) {
        return
    }
    if result.IsOpen() && result.Fingerprint != nil && len(result.Fingerprint.HttpFlows) > 0 {
        addr = str.HostPort(result.Target, result.Port)
        res, err = crawler.Start(
            addr, crawler.maxRequest(crawlerMaxRequest),
            crawler.autoLogin("admin", "password"), crawler.urlRegexpExclude(` + "`" + `(?i).*?\/?(logout|reset|delete|setup).*` + "`" + `),
        )
        if err != nil {
            yakit.Error("basic crawler start failed: %s", err)
            return
        }

        yakit.Info("Start to Exec Basic Crawler for %v", addr)
        for crawlerReq = range res {
            yakit.Info("found url: %s", crawlerReq.Url())
            handleMITMPluginCaller(crawlerReq)
        }
    }
}


// 保存统计数据
startTimestamp = time.Now().Unix()
portTableHeader = ["Host", "Port", "Fingerprint", "HtmlTitle"]
portTableData = make([][]string)
addPortTableData = func(host, port, fp, title) {
    portTableData = append(portTableData, [sprint(host), sprint(port), sprint(fp), sprint(title)])
}
cClassCounter = make(map[string]int)
targetCounter = make(map[string]int)
updateCounter = func(target) {
    ordinaryCount = targetCounter[target]
    if ordinaryCount == undefined {
        targetCounter[target] = 1
    }else{
        targetCounter[target] = targetCounter[target] + 1
    }

    cClass = str.ParseStringToCClassHosts(target)
    if cClass != "" {
        cCount = cClassCounter[cClass]
        if cCount != undefined {
            cClassCounter[cClass] = cClassCounter[cClass] + 1
        }else{
            cClassCounter[cClass] = 1
        }
    }
}

wg = sync.NewWaitGroup()
handleFpResult = func(result) {
	defer func{
		err = recover()
		if err != nil { yakit.Error("call port-scan failed: %s", err) }
	}

    if result.IsOpen() {
        addPortTableData(result.Target, result.Port, result.GetServiceName(), result.GetHtmlTitle())
        updateCounter(result.Target)
	    yakit.Output({
		    "host": result.Target,
		    "port": result.Port,
		    "fingerprint": result.GetServiceName(),
            "htmlTitle": result.GetHtmlTitle(), 
            "isOpen": true,
	    })
        if saveToDB {
            yakit.SavePortFromResult(result)
        }
        println(result.String(protos))
    }else{
        yakit.Output({
		    "host": result.Target,
		    "port": result.Port,
            "isOpen": false,
	    })
        if saveClosed && saveToDB {
            yakit.SavePortFromResult(result)
        }
        println(result.String(protos))   
    }

	go func(){
        defer func{
            err = recover()
            if err != nil { yakit.Error("call port-scan plugin failed: %s", err) }
        }
        manager.GetNativeCaller().CallByName("handle", result)
    }()

    wg.Add(1)
    go func{
        defer wg.Done()
        handleCrawler(result)
    }
}

getPingScan = func() {
	return ping.Scan(
        hosts, ping.proxy(proxies), ping.skip(skippedHostAliveScan), ping.tcpPingPorts(hostAliveTCPPorts), 
        ping.timeout(hostAliveTimeout), ping.concurrent(hostAliveConcurrent), ping.onResult(func(i){
            if (i.Ok) { return }
            // 这里返回
            updateProgress(portTotal)
        }), ping.excludeHosts(excludeHosts),
    ) 
}

if mode == "fingerprint" {
    res, err := servicescan.ScanFromPing(
        getPingScan(), 
        ports, opts...)
    die(err)

    for result = range res {
        updateProgress(1)
        handleFpResult(result)   
    }
}

synOpenPortCount = 0
addSynOpenPortCount = func() {
    synOpenPortCount++
    yakit.StatusCard("SYN 开放端口", synOpenPortCount)
}

// 数据包计数
synPacketCounter = 0
addSynPacketCounter = func(){synPacketCounter++}
startSYNPacketCounter = func(){
    go func{
        for { sleep(1); yakit.StatusCard("SYN 发包数", synPacketCounter) }
    }
}

if mode == "syn" {
    startSYNPacketCounter()
    synResults, err := synscan.ScanFromPing(
        getPingScan(), ports, synscan.initHostFilter(hosts), synscan.initPortFilter(ports),
        synscan.excludeHosts(excludeHosts), synscan.excludePorts(excludePorts),
        synscan.submitTaskCallback(func(i){ addSynPacketCounter() }),
        synscan.concurrent(synConcurrent),
    )
    die(err)

    for result := range synResults {
        updateProgress(int(portTotal/2))
        addSynOpenPortCount()
	    yakit.Output({
		    "host": result.Host,
		    "port": result.Port,
            "isOpen": true,
	    })
        if saveToDB {
            yakit.SavePortFromResult(result)
        }
        result.Show()    
    }
}

if mode == "all" {
    startSYNPacketCounter()
    synResults, err := synscan.ScanFromPing(
        getPingScan(), ports, 
        synscan.initHostFilter(hosts), synscan.initPortFilter(ports),
        synscan.excludeHosts(excludeHosts),
        synscan.excludePorts(excludePorts),
        synscan.callback(func(i){
            addSynOpenPortCount()
            db.SavePortFromResult(i)
        }),
        synscan.submitTaskCallback(func(i){ addSynPacketCounter() }),
        synscan.concurrent(synConcurrent),
    )
    die(err)

    res, err := servicescan.ScanFromSynResult(synResults, opts...)
    die(err)
    for result := range res {
        updateProgress(int(portTotal/2))
        handleFpResult(result)   
    }
}


// 生成报告
reportIns = report.New()
reportIns.From("port-scan")
resultPortCount = len(portTableData)

endTimestamp = time.Now().Unix()

reportIns.Title("端口扫描报告:[%v]台主机/[%v]个开放端口/涉及[%v]个C段", len(targetCounter), resultPortCount, len(cClassCounter))
reportIns.Table(portTableHeader, portTableData...)
reportIns.Markdown(
    sprintf("# 扫描状态统计\n\n"+
    "本次扫描耗时 %v 秒\n\n"+
    "涉及扫描插件: %v 个", 
    endTimestamp - startTimestamp, len(scriptNames),
))
if scriptNameList != "" {
    reportIns.Markdown(scriptNameList)
}
items = make([][]string)
if len(cClassCounter) > 0 {
    reportIns.Markdown("## C 段统计\n\n")
    for name, count = range cClassCounter{
        items = append(items, [sprint(name), sprint(count)])
    }
    reportIns.Table(["C 段", "开放端口数量"], items...)
}

if len(targetCounter) > 0 {
    reportIns.Markdown("## 主机维度端口统计")
    for name, count = range targetCounter{
        items = append(items, [sprint(name), sprint(count)])
    }
    reportIns.Table(["主机 IP", "开放端口数量"], items...)
}
reportIns.Save()


// 等待插件执行结果
yakit.Info("PortScan Finished Waiting for Plugin Results")
println("PortScan Finished... Waiting Plugins")
wg.Wait()
manager.Wait()

/*
type palm/common/yakgrpc/yakit.(Report) struct {
  Fields(可用字段): 
  StructMethods(结构方法/函数): 
  PtrStructMethods(指针结构方法/函数): 
      func Divider() 
      func From(v1: interface {}, v2 ...interface {}) 
      func Markdown(v1: string) 
      func Owner(v1: interface {}, v2 ...interface {}) 
      func Save() 
      func Table(v1: interface {}, v2 ...interface {}) 
      func Title(v1: interface {}, v2 ...interface {}) 
      func ToRecord() return(*yakit.ReportRecord, error) 
}
*/`

func (s *Server) PortScan(req *ypb.PortScanRequest, stream ypb.Yak_PortScanServer) error {

	reqParams := &ypb.ExecRequest{
		Script: scanPort,
	}

	// 把文件写到本地。
	tmpTargetFile, err := ioutil.TempFile("", "yakit-portscan-*.txt")
	if err != nil {
		return utils.Errorf("create temp target file failed: %s", err)
	}
	raw, _ := ioutil.ReadFile(req.GetTargetsFile())
	targetsLineFromFile := utils.PrettifyListFromStringSplited(string(raw), "\n")
	targetsLine := utils.PrettifyListFromStringSplited(req.GetTargets(), "\n")
	targets := append(targetsLine, targetsLineFromFile...)
	var allTargets = strings.Join(targets, ",")
	if req.GetEnableCClassScan() {
		allTargets = utils.ParseStringToCClassHosts(allTargets)
	}
	_, _ = tmpTargetFile.WriteString(allTargets)
	if len(targets) <= 0 {
		return utils.Errorf("empty targets")
	}
	tmpTargetFile.Close()
	defer os.RemoveAll(tmpTargetFile.Name())

	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
		Key:   "target-file",
		Value: tmpTargetFile.Name(),
	})
	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
		Key:   "ports",
		Value: utils.ConcatPorts(utils.ParseStringToPorts(req.Ports)),
	})
	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
		Key:   "mode",
		Value: req.GetMode(),
	})

	if req.GetExcludeHosts() != "" {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "exclude-hosts",
			Value: req.GetExcludeHosts(),
		})
	}

	if req.GetExcludePorts() != "" {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "exclude-ports",
			Value: req.GetExcludePorts(),
		})
	}

	if req.GetSaveToDB() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key: "save-to-db",
		})
	}

	if req.GetSaveClosedPorts() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key: "save-closed-ports",
		})
	}

	// 主动发包
	if req.GetActive() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key: "active",
		})
	}

	// 设置指纹扫描的并发
	if req.GetConcurrent() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "concurrent",
			Value: fmt.Sprint(req.GetConcurrent()),
		})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "concurrent",
			Value: fmt.Sprint(50),
		})
	}

	// 设置 SYN 扫描的并发
	if req.GetSynConcurrent() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "syn-concurrent", Value: fmt.Sprint(req.GetSynConcurrent())})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "syn-concurrent", Value: "1000"})
	}

	if len(req.GetProto()) > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "proto",
			Value: strings.Join(req.GetProto(), ","),
		})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "proto",
			Value: "tcp",
		})
	}

	if len(utils.StringArrayFilterEmpty(req.GetProxy())) > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "proxy",
			Value: strings.Join(req.GetProxy(), ","),
		})
	}

	// 爬虫设置
	if req.GetEnableBasicCrawler() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key: "enable-basic-crawler",
		})
	}
	if req.GetBasicCrawlerRequestMax() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "basic-crawler-request-max",
			Value: fmt.Sprint(req.GetBasicCrawlerRequestMax()),
		})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "basic-crawler-request-max",
			Value: "5",
		})
	}

	if req.GetProbeTimeout() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "probe-timeout", Value: fmt.Sprint(req.GetProbeTimeout())})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "probe-timeout", Value: "5.0"})
	}

	if req.GetProbeMax() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "probe-max", Value: "3"})
	}

	switch req.GetFingerprintMode() {
	case "service":
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "fp-mode",
			Value: "service",
		})
	case "web":
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "fp-mode",
			Value: "web",
		})
	default:
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{
			Key:   "fp-mode",
			Value: "all",
		})
	}

	// handle plugin names
	var callback func()
	reqParams.Params, callback, err = appendPluginNamesEx("script-name-file", "\n", reqParams.Params, req.GetScriptNames()...)
	if callback != nil {
		defer callback()
	}
	if err != nil {
		return utils.Errorf("load plugin names failed: %s", err)
	}

	if req.GetSkippedHostAliveScan() {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "skipped-host-alive-scan"})
	}

	if req.GetHostAliveConcurrent() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-concurrent", Value: fmt.Sprint(req.GetHostAliveConcurrent())})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-concurrent", Value: fmt.Sprint(20)})
	}

	if req.GetHostAliveTimeout() > 0 {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-timeout", Value: fmt.Sprint(req.GetHostAliveTimeout())})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-timeout", Value: fmt.Sprint(5.0)})
	}

	if req.GetHostAlivePorts() != "" {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-ports", Value: fmt.Sprint(req.GetHostAlivePorts())})
	} else {
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "host-alive-ports", Value: "22,80,443"})
	}

	return s.Exec(reqParams, stream)
}

func (s *Server) ViewPortScanCode(ctx context.Context, req *ypb.Empty) (*ypb.SimpleScript, error) {
	return &ypb.SimpleScript{Content: scanPort}, nil
}
