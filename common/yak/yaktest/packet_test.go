package yaktest

import "testing"

func TestPacketBatchScan(t *testing.T) {
	var test = `yakit.AutoInitYakit()

loglevel("info")
productMode = cli.Bool("prod")

// 获取 flowsID
flows = make([]int64)
flowsFile = cli.String("flows-file")
if flowsFile != "" {
    ports = str.ParseStringToPorts(string(file.ReadFile(flowsFile)[0]))
    for _, port = range ports {
        flows = append(flows, int64(port))
    }
}

if !productMode { yakit.StatusCard("调试模式", "TRUE") }

if len(flows) <= 0 && !productMode {
    yakit.Info("调试模式补充 FlowID")
    flows = append(flows, 17)
}

caller, err = hook.NewMixPluginCaller()
die(err)

// 加载插件
plugins = cli.YakitPlugin() // yakit-plugin-file
if len(plugins) <= 0 && !productMode {
    // 设置默认插件，一般在这儿用于调试
    plugins = [
        "WebLogic_CVE-2022-21371目录遍历漏洞检测",
    ]
}
loadedPlugins = []
failedPlugins = []
for _, pluginName = range x.If(plugins == undefined, [], plugins) {
    err := caller.LoadPlugin(pluginName)
    if err != nil {
        failedPlugins = append(failedPlugins, pluginName)
        yakit.Error("load plugin[%s] failed: %s", pluginName, err)
    }else{
        yakit.Info("加载插件：%v", pluginName)
        loadedPlugins = append(loadedPlugins, pluginName)
    }
}

if len(failedPlugins)>0 {
    yakit.StatusCard("插件加载失败数", len(failedPlugins))
}

if len(loadedPlugins) <= 0 {
    yakit.StatusCard("执行失败", "没有插件被正确加载", "error")
    die("没有插件加载")
}else{
    yakit.StatusCard("插件加载数", len(loadedPlugins))
}

packetRawFile = cli.String("packet-file")
packetHttps = cli.Bool("packet-https")

// 设置并发参数
pluginConcurrent = 20
packetConcurrent = 20

// 设置异步，设置并发，设置 Wait
caller.SetDividedContext(true)
caller.SetConcurrent(pluginConcurrent)
defer caller.Wait()

handleHTTPFlow = func(flow) {
    defer func{
        err = recover()
        if err != nil {
            yakit.Error("handle httpflow failed: %s", err)
        }
    }

    reqBytes = []byte(codec.StrconvUnquote(flow.Request)[0])
    rspBytes = []byte(codec.StrconvUnquote(flow.Response)[0])
    _, body = poc.Split(rspBytes)
    // 调用镜像流量功能，并控制要不要进行端口扫描插件调用
    caller.MirrorHTTPFlowEx(true, flow.IsHTTPS, flow.Url, reqBytes, rspBytes, body)
}

handleHTTPRequestRaw = func(https, reqBytes) {
    defer func{
        err = recover()
        if err != nil {
            yakit.Error("handle raw bytes failed: %s", err)
        }
    }

    urlstr = ""
    urlIns, _ = str.ExtractURLFromHTTPRequestRaw(reqBytes, https)
    if urlIns != nil {
        urlstr = urlIns.String()
    }
    rsp, req, _ = poc.HTTP(reqBytes, poc.https(packetHttps))
    header, body = poc.Split(rsp)
    caller.MirrorHTTPFlowEx(true, https, urlstr, reqBytes, rsp, body)
}

// 开始获取 HTTPFlow ID
if len(flows) > 0 {
    yakit.StatusCard("扫描历史数据包", len(flows))
	for result = range db.QueryHTTPFlowsByID(flows...) {
		// 提交扫描任务：
		yakit.Info("提交扫描任务:【%v】%v", result.Method, result.Url)
		handleHTTPFlow(result)
	}
}

if packetRawFile != "" {
    raw, err = file.ReadFile(packetRawFile)
    if err != nil {
        yakit.StatusCard("失败原因", "无法接收参数", "error")
    }
    handleHTTPRequestRaw(packetHttps, raw)
}
`
	Run("测试数据包测试", t, []YakTestCase{
		{
			Name: "测试数据包测试",
			Src:  test,
		},
	}...)
}
