package fp

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/fp/fingerprint"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/fp/fingerprint/utils"
	"github.com/yaklang/yaklang/common/fp/iotdevfp"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils/lowhttp"

	utils2 "github.com/yaklang/yaklang/common/utils"
)

func (f *Matcher) webDetector(result *MatchResult, ctx context.Context, config *Config, host string, ip net.IP, port int) (*MatchResult, error) {
	//////////////////////////////////////////////////////////////////////////
	////////////////////////从这里开始进行 Web 指纹识别//////////////////////////
	//////////////////////////////////////////////////////////////////////////
	// 首先执行各种 IoT 设备的配置检测(优先级最高)
	f.log("starting web fingerprint detection")

	//////////////////////////////////////////////////////////////////////////
	////////////////////////////// IoT 设备优化 ///////////////////////////////
	//////////////////////////////////////////////////////////////////////////
	f.log("start to check iotdevfp: %v", utils2.HostPort(ip.String(), port))
	h := host
	if h == "" {
		h = ip.String()
	}
	var inspectResults []*netx.TLSInspectResult
	var tlsChecked bool
	isOpen, tlsChecked, inspectResults, redirectInfos, err := f.fetchBannerFromHostPort(ctx, nil, nil, h, port, int64(config.FingerprintDataSize), config.RuntimeId, config.Proxies...)
	if err != nil {
		if !isOpen {
			f.log("port is closed: %v", err)
			return &MatchResult{
				Target: host,
				Port:   port,
				State:  result.State,
				Reason: err.Error(),
				Fingerprint: &FingerprintInfo{
					IP:                ip.String(),
					Port:              port,
					CheckedTLS:        tlsChecked,
					TLSInspectResults: inspectResults,
				},
			}, nil
		}
		f.log("error occurred but port is open: %v", err)
		fastResult := &MatchResult{
			Target: host,
			Port:   port,
			State:  OPEN,
			Reason: "",
			Fingerprint: &FingerprintInfo{
				IP:                ip.String(),
				Port:              port,
				CheckedTLS:        tlsChecked,
				TLSInspectResults: inspectResults,
			},
		}
		f.reportOpen(fastResult)
		return fastResult, nil
	}
	if redirectInfos == nil {
		f.log("no redirect info found")
		// 设置初始化匹配结果
		fastResult := &MatchResult{
			Target: host,
			Port:   port,
			State:  OPEN,
			Fingerprint: &FingerprintInfo{
				IP:                ip.String(),
				Port:              port,
				CheckedTLS:        tlsChecked,
				TLSInspectResults: inspectResults,
			},
		}
		f.reportOpen(fastResult)
		return fastResult, nil
	}

	// 如果强制启用 Web 指纹检测，则需要 Bypass 指纹检测条件
	// 为 Fingerprint 强制赋予可以执行 Web 指纹识别的值
	if result.Fingerprint == nil {
		f.log("initializing fingerprint info")
		result.Fingerprint = &FingerprintInfo{
			IP:                ip.String(),
			Port:              port,
			CheckedTLS:        tlsChecked,
			TLSInspectResults: inspectResults,
		}
	}

	var (
		// wg                      = new(sync.WaitGroup)
		results     = new(sync.Map)
		cpeAnalyzer = utils.NewCPEAnalyzer()
		httpflows   []*HTTPFlow
	)

	f.log("finished to check iotdevfp: %v fetch response[%v]", utils2.HostPort(ip.String(), port), len(redirectInfos))
	result.State = OPEN
	// notify via callback
	f.reportOpen(result)

	result.Fingerprint.ServiceName = "http"
	if len(redirectInfos) > 1 {
		redirectInfos = append([]*lowhttp.RedirectFlow{utils2.GetLastElement(redirectInfos)}, redirectInfos[1:]...)
	}
	for _, i := range redirectInfos {
		var currentCPE []*schema.CPE
		if !f.Config.DisableDefaultIotFingerprint {
			f.log("matching IoT device fingerprints")
			iotdevResults := iotdevfp.MatchAll(i.Response)
			for _, iotdevResult := range iotdevResults {
				result.Fingerprint.CPEs = append(result.Fingerprint.CPEs, iotdevResult.GetCPE())
				cpeIns, _ := webfingerprint.ParseToCPE(iotdevResult.GetCPE())

				if cpeIns != nil {
					currentCPE = append(currentCPE, fingerprint.LoadCPEFromWebfingerrintCPE(cpeIns))
				}
			}
		}

		if result.Fingerprint == nil {
			f.log("creating new fingerprint info")
			result.Fingerprint = &FingerprintInfo{
				IP:                ip.String(),
				Port:              port,
				Proto:             TCP,
				ServiceName:       "http",
				Banner:            strconv.Quote(string(i.Response)),
				TLSInspectResults: inspectResults,
			}
		}

		if i.IsHttps {
			f.log("detected HTTPS service")
			name := strings.ToLower(result.Fingerprint.ServiceName)
			if !strings.Contains(name, "https") &&
				strings.Contains(name, "http") {
				// 不包含 https 但是包含 http
				result.Fingerprint.ServiceName = strings.ReplaceAll(name, "http", "https")
			}

			if name == "" {
				result.Fingerprint.ServiceName = "https"
			}

		}

		info := i
		requestHeader, requestBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(info.Request)
		responseHeader, responseBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(info.Response)
		flow := &HTTPFlow{
			StatusCode:     lowhttp.GetStatusCodeFromResponse(info.Response),
			IsHTTPS:        info.IsHttps,
			RequestHeader:  []byte(requestHeader),
			RequestBody:    requestBody,
			ResponseHeader: []byte(responseHeader),
			ResponseBody:   responseBody,
			CPEs:           currentCPE,
		}
		httpflows = append(httpflows, flow)
		f.matcher.Route = func(ctx context.Context, webPath string) ([]byte, error) {
			target := utils2.HostPort(host, port)
			packet := []byte(fmt.Sprintf(`GET %s HTTP/1.1
Host: %v
User-Agent: Mozilla/5.0 (Windows NT 10.0; rv:68.0) Gecko/20100101 Firefox/68.0
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7
`, target, webPath))
			f.log("sending request to path: %s", webPath)
			var ok bool
			var flow []*lowhttp.RedirectFlow
			var err error
			ok, tlsChecked, inspectResults, flow, err = f.fetchBannerFromHostPortWithTLSSkipped(tlsChecked, ctx, packet, inspectResults, h, port, int64(config.FingerprintDataSize), config.RuntimeId, config.Proxies...)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("fetch path %s banner failed", webPath)
			}
			f := utils2.GetLastElement(flow)
			return f.Response, nil
		}
		cpes := f.matcher.MatchResource(ctx, f.Config.ProbesMax, f.Config.GetWebFingerprintRules(), func(path string) (*rule.MatchResource, error) {
			res := &rule.MatchResource{
				Protocol: "http",
				Port:     port,
				Path:     path,
			}
			cached := map[string][]byte{}
			if path == "" || path == "/" {
				res.Data = info.Response
				return res, nil
			}
			if v, ok := cached[path]; ok {
				res.Data = v
				return res, nil
			}
			data, err := f.matcher.Route(ctx, path)
			if err != nil {
				return nil, err
			}
			cached[path] = data
			return rule.NewHttpResource(data), nil
		})

		// 如果检测到指纹信息
		if len(cpes) > 0 {
			f.log("found %d CPEs", len(cpes))
			currentCPE = append(currentCPE, cpes...)
			urlStr := info.RespRecord.Url
			cpeAnalyzer.Feed(urlStr, cpes...)
			results.Store(urlStr, cpes)
		}

		if len(flow.CPEs) < len(currentCPE) {
			flow.CPEs = currentCPE
		}
	}
	urlCpe := map[string][]*schema.CPE{}
	results.Range(func(key, value interface{}) bool {
		f.log("url: %s cpes: %#v", key, value)
		_url := key.(string)
		cpes := value.([]*schema.CPE)
		urlCpe[_url] = cpes
		return true
	})

	// 为 FingerprintResult 完善带 URL 的指纹信息
	result.Fingerprint.CPEFromUrls = urlCpe

	// add tls inspect results
	result.Fingerprint.CheckedTLS = tlsChecked
	result.Fingerprint.TLSInspectResults = inspectResults

	// 如果可能的话，需要完善指纹识别的 HTTP 相关请求
	result.Fingerprint.HttpFlows = httpflows

	// 把新的 cpes 更新到原来的 cpe 列表中
	cpes := result.Fingerprint.CPEs
	var cpesStrRaw []string
	for _, c := range cpeAnalyzer.AvailableCPE() {
		cpesStrRaw = append(cpesStrRaw, c.String())
	}
	result.Fingerprint.CPEs = append(cpes, cpesStrRaw...)

	// 返回结果前需要检查 Fingerprint 的必要字段
	if result.Fingerprint.ServiceName == "" {
		f.log("setting default service name")
		result.Fingerprint.ServiceName = strings.ToLower(string(result.Fingerprint.Proto))
	}

	if redirectInfos != nil {
		result.State = OPEN
		result.Fingerprint.Proto = TCP
		if result.Fingerprint.ServiceName == "" {
			if redirectInfos[0].IsHttps {
				result.Fingerprint.ServiceName = "https"
			} else {
				result.Fingerprint.ServiceName = "http"
			}
		}
	}

	switch result.Fingerprint.Proto {
	case TCP, UDP:
	default:
		result.Fingerprint.Proto = TCP
	}
	f.log("web fingerprint detection completed")
	return result, nil
}

func (f *Matcher) fetchBannerFromHostPortWithTLSSkipped(checkedTls bool, baseCtx context.Context, packet2 []byte, tlsInspectResults []*netx.TLSInspectResult, host string, port interface{}, bufferSize int64, runtimeId string, proxy ...string) (
	isPortOpen bool,
	tlsChecked bool,
	tlsResults []*netx.TLSInspectResult,
	flows []*lowhttp.RedirectFlow,
	_ error,
) {
	f.log("start to fetch banner from host: %v port: %v", host, port)
	ctx, cancel := context.WithTimeout(baseCtx, f.Config.ProbeTimeout)
	defer cancel()

	connectTimeout := 10 * time.Second
	if ddl, ok := ctx.Deadline(); ok {
		connectTimeout = ddl.Sub(time.Now())
		if connectTimeout <= 0 {
			connectTimeout = 10 * time.Second
		}
	}
	f.log("fetchBannerFromHostPort timeout set to: %v", connectTimeout)

	portInt, _ := strconv.Atoi(fmt.Sprint(port))
	target := utils2.HostPort(host, port)

	f.log("checking if target is TLS service: %v", target)
	start := time.Now()

	var isTls bool = len(tlsInspectResults) > 0
	if !isTls && !checkedTls {
		tlsInspectResults, _ = netx.TLSInspectContext(ctx, target)
		isTls = len(tlsInspectResults) > 0
		tlsChecked = true
	}

	cost := time.Since(start)
	f.log("target: %v isTls: %v cost: %v", target, tlsInspectResults, cost)
	packet := []byte(fmt.Sprintf(`GET / HTTP/1.1
Host: %v
User-Agent: Mozilla/5.0 (Windows NT 10.0; rv:68.0) Gecko/20100101 Firefox/68.0
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7
`, target))
	if packet2 != nil {
		f.log("using custom packet")
		packet = packet2
	}
	var isOpen bool
	f.log("sending HTTP request to target")
	rspDetail, err := lowhttp.HTTP(
		lowhttp.WithRuntimeId(runtimeId),
		lowhttp.WithHttps(isTls),
		lowhttp.WithHost(host),
		lowhttp.WithPort(portInt),
		lowhttp.WithRequest(packet),
		lowhttp.WithRedirectTimes(5),
		lowhttp.WithJsRedirect(true),
		lowhttp.WithProxy(proxy...),
		lowhttp.WithConnectTimeout(connectTimeout),
		lowhttp.WithConnPool(f.Config.WebScanDisableConnPool),
	)
	if err != nil {
		f.log("HTTP request failed: %v", err)
		return isOpen, tlsChecked, tlsInspectResults, nil, utils2.Errorf("lowhttp.HTTP failed: %s", err)
	}
	isOpen = rspDetail.PortIsOpen
	f.log("port open status: %v", isOpen)
	return isOpen, tlsChecked, tlsInspectResults, rspDetail.RedirectRawPackets, nil
}

func (f *Matcher) fetchBannerFromHostPort(baseCtx context.Context, packet2 []byte, tlsInspectResults []*netx.TLSInspectResult, host string, port interface{}, bufferSize int64, runtimeId string, proxy ...string) (
	isPortOpen bool,
	tlsChecked bool,
	tlsResults []*netx.TLSInspectResult,
	flows []*lowhttp.RedirectFlow,
	_ error,
) {
	return f.fetchBannerFromHostPortWithTLSSkipped(false, baseCtx, packet2, tlsInspectResults, host, port, bufferSize, runtimeId, proxy...)
}
