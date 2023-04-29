package yakgrpc

import (
	"context"
	"io/ioutil"
	"os"
	"yaklang/common/consts"
	"yaklang/common/go-funk"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/yakgrpc/ypb"
)

const execPacketScanCode = `
yakit.AutoInitYakit()

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
    flows = append(flows, 1142)
}

caller, err = hook.NewMixPluginCaller()
die(err)

// 加载插件
plugins = cli.YakitPlugin() // yakit-plugin-file
if len(plugins) <= 0 && !productMode {
    // 设置默认插件，一般在这儿用于调试
    plugins = [
        "Spring Cloud Function SPEL表达式注入漏洞检测", "MySQL CVE 合规检查: 2016-2022",
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

func (s *Server) ExecPacketScan(req *ypb.ExecPacketScanRequest, stream ypb.Yak_ExecPacketScanServer) error {
	reqParams := &ypb.ExecRequest{Script: execPacketScanCode}
	var err error

	reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "prod"})

	if len(req.GetHTTPRequest()) > 0 {
		if req.GetHTTPS() {
			reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "packet-https"})
		}
		fp, err := ioutil.TempFile(consts.GetDefaultYakitBaseTempDir(), "scan-packet-file-*.txt")
		if err != nil {
			return utils.Errorf("创建临时文件失败: %s", err)
		}
		fp.Write(req.GetHTTPRequest())
		fp.Close()
		defer func() {
			os.RemoveAll(fp.Name())
		}()
		reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "packet-file", Value: fp.Name()})
	}

	// yakit-plugin-file
	var pluginCallback func()
	reqParams.Params, pluginCallback, err = appendPluginNames(reqParams.Params, req.GetPluginList()...)
	if err != nil {
		return utils.Errorf("append plugin names failed: %s", err)
	}
	if pluginCallback != nil {
		defer pluginCallback()
	}

	// httpflow 作为输入
	if len(req.GetHTTPFlow()) > 0 {
		var flows []int = funk.Map(req.GetHTTPFlow(), func(i int64) int {
			return int(i)
		}).([]int)
		flowsStr := utils.ConcatPorts(flows)
		if flowsStr != "" {
			fp, err := consts.TempFile("yakit-packet-scan-httpflow-%v.txt")
			if err != nil {
				return utils.Errorf("cannot create tmp file(for packet-scan): %s", err)
			}
			fp.WriteString(flowsStr)
			log.Infof("start to handle flow(ID): %v", flowsStr)
			fp.Close()
			defer os.RemoveAll(fp.Name())

			reqParams.Params = append(reqParams.Params, &ypb.ExecParamItem{Key: "flows-file", Value: fp.Name()})
		}
	} else {
		log.Info("no httpflow scanned")
	}

	ctx, cancelCtx := context.WithTimeout(stream.Context(), utils.FloatSecondDuration(float64(req.GetTotalTimeoutSeconds())))
	defer cancelCtx()
	return s.ExecWithContext(ctx, reqParams, stream)
}
