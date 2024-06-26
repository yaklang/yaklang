package fp

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/fp/fingerprint"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/fp/fingerprint/utils"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/fp/iotdevfp"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"github.com/yaklang/yaklang/common/log"
	utils2 "github.com/yaklang/yaklang/common/utils"
)

func (f *Matcher) webDetector(result *MatchResult, ctx context.Context, config *Config, host string, ip net.IP, port int) (*MatchResult, error) {
	//////////////////////////////////////////////////////////////////////////
	////////////////////////从这里开始进行 Web 指纹识别//////////////////////////
	//////////////////////////////////////////////////////////////////////////
	// 首先执行各种 IoT 设备的配置检测(优先级最高)
	iotDetectCtx, cancel := context.WithTimeout(ctx, config.ProbeTimeout)
	defer cancel()

	//////////////////////////////////////////////////////////////////////////
	////////////////////////////// IoT 设备优化 ///////////////////////////////
	//////////////////////////////////////////////////////////////////////////
	//log.Infof("start to check iotdevfp: %v", utils2.HostPort(ip.String(), port))
	h := host
	if h == "" {
		h = ip.String()
	}
	isOpen, redirectInfos, err := FetchBannerFromHostPort(iotDetectCtx, nil, h, port, int64(config.FingerprintDataSize), config.RuntimeId, config.Proxies...)
	if err != nil {
		if !isOpen {
			return &MatchResult{
				Target: host,
				Port:   port,
				State:  CLOSED,
				Reason: err.Error(),
				Fingerprint: &FingerprintInfo{
					IP:   ip.String(),
					Port: port,
				},
			}, nil
		}
		return &MatchResult{
			Target: host,
			Port:   port,
			State:  OPEN,
			Reason: "",
			Fingerprint: &FingerprintInfo{
				IP:   ip.String(),
				Port: port,
			},
		}, nil
	}
	if redirectInfos == nil {
		// 设置初始化匹配结果
		return &MatchResult{
			Target: host,
			Port:   port,
			State:  OPEN,
			Fingerprint: &FingerprintInfo{
				IP:   ip.String(),
				Port: port,
			},
		}, nil
	}

	// 如果强制启用 Web 指纹检测，则需要 Bypass 指纹检测条件
	// 为 Fingerprint 强制赋予可以执行 Web 指纹识别的值
	if result.Fingerprint == nil {
		result.Fingerprint = &FingerprintInfo{
			IP:   ip.String(),
			Port: port,
		}
	}

	var (
		//wg                      = new(sync.WaitGroup)
		results     = new(sync.Map)
		cpeAnalyzer = utils.NewCPEAnalyzer()
		httpflows   []*HTTPFlow
	)
	if redirectInfos != nil {
		log.Debugf("finished to check iotdevfp: %v fetch response[%v]", utils2.HostPort(ip.String(), port), len(redirectInfos))
		result.State = OPEN
		result.Fingerprint.ServiceName = "http"
		for _, i := range redirectInfos {
			var iotdevResults = iotdevfp.MatchAll(i.Response)
			if result.Fingerprint == nil {
				result.Fingerprint = &FingerprintInfo{
					IP:          ip.String(),
					Port:        port,
					Proto:       TCP,
					ServiceName: "http",
					Banner:      strconv.Quote(string(i.Response)),
				}
			}

			if i.IsHttps {
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

			var currentCPE []*rule.CPE
			for _, iotdevResult := range iotdevResults {
				result.Fingerprint.CPEs = append(result.Fingerprint.CPEs, iotdevResult.GetCPE())
				cpeIns, _ := webfingerprint.ParseToCPE(iotdevResult.GetCPE())

				if cpeIns != nil {
					currentCPE = append(currentCPE, fingerprint.LoadCPEFromWebfingerrintCPE(cpeIns))
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
				ok, flow, err := FetchBannerFromHostPort(iotDetectCtx, packet, h, port, int64(config.FingerprintDataSize), config.RuntimeId, config.Proxies...)
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, fmt.Errorf("fetch path %s banner failed", webPath)
				}
				f := utils2.GetLastElement(flow)
				return f.Response, nil
			}
			fpInfos := f.matcher.Match(iotDetectCtx, info.Response)
			//cpes, err := f.wfMatcher.MatchWithOptions(info, config.GenerateWebFingerprintConfigOptions()...)
			//if err != nil {
			//	if !strings.Contains(err.Error(), "no rules matched") {
			//		continue
			//	}
			//}

			var cpes []*rule.CPE
			for _, fpInfo := range fpInfos {
				cpes = append(cpes, &fpInfo.CPE)
			}
			// 如果检测到指纹信息
			if len(fpInfos) > 0 {
				currentCPE = append(currentCPE, cpes...)
				urlStr := info.RespRecord.Url
				cpeAnalyzer.Feed(urlStr, cpes...)
				results.Store(urlStr, cpes)
			}

			if len(flow.CPEs) < len(currentCPE) {
				flow.CPEs = currentCPE
			}
		}
	}
	urlCpe := map[string][]*rule.CPE{}
	results.Range(func(key, value interface{}) bool {
		log.Debugf("url: %s cpes: %#v", key, value)
		_url := key.(string)
		cpes := value.([]*rule.CPE)
		urlCpe[_url] = cpes
		return true
	})

	// 为 FingerprintResult 完善带 URL 的指纹信息
	result.Fingerprint.CPEFromUrls = urlCpe

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
	return result, nil
}

func FetchBannerFromHostPort(baseCtx context.Context, packet2 []byte, host string, port interface{}, bufferSize int64, runtimeId string, proxy ...string) (bool, []*lowhttp.RedirectFlow, error) {
	ctx, cancel := context.WithTimeout(baseCtx, 10*time.Second)
	defer cancel()

	timeout := 10 * time.Second
	if ddl, ok := ctx.Deadline(); ok {
		timeout = ddl.Sub(time.Now())
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
	}

	portInt, _ := strconv.Atoi(fmt.Sprint(port))
	target := utils2.HostPort(host, port)
	isTls := netx.IsTLSService(target)
	packet := []byte(fmt.Sprintf(`GET / HTTP/1.1
Host: %v
User-Agent: Mozilla/5.0 (Windows NT 10.0; rv:68.0) Gecko/20100101 Firefox/68.0
Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7
`, target))
	if packet2 != nil {
		packet = packet2
	}

	rspDetail, err := lowhttp.HTTP(
		lowhttp.WithRuntimeId(runtimeId),
		lowhttp.WithHttps(isTls),
		lowhttp.WithHost(host),
		lowhttp.WithPort(portInt),
		lowhttp.WithRequest(packet),
		lowhttp.WithRedirectTimes(5),
		lowhttp.WithJsRedirect(true),
		lowhttp.WithProxy(proxy...),
	)
	isOpen := rspDetail.PortIsOpen
	if err != nil {
		return isOpen, nil, utils2.Errorf("lowhttp.HTTP failed: %s", err)
	}
	return isOpen, rspDetail.RedirectRawPackets, nil
}
