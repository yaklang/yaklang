package fp

import (
	"context"
	"github.com/yaklang/yaklang/common/fp/iotdevfp"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"github.com/yaklang/yaklang/common/log"
	utils2 "github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"net"
	"strconv"
	"strings"
	"sync"
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
	isOpen, httpBanners, err := FetchBannerFromHostPortEx(iotDetectCtx, nil, ip.String(), port, int64(config.FingerprintDataSize), config.Proxies...)
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

	if httpBanners == nil {
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
		cpeAnalyzer = webfingerprint.NewCPEAnalyzer()
		httpflows   []*HTTPFlow
	)
	if httpBanners != nil {
		log.Debugf("finished to check iotdevfp: %v fetch response[%v]", utils2.HostPort(ip.String(), port), len(httpBanners))
		result.State = OPEN
		result.Fingerprint.ServiceName = "http"
		for _, i := range httpBanners {
			var iotdevResults = iotdevfp.MatchAll(i.Bytes())
			if result.Fingerprint == nil {
				result.Fingerprint = &FingerprintInfo{
					IP:          ip.String(),
					Port:        port,
					Proto:       TCP,
					ServiceName: "http",
					Banner:      strconv.Quote(string(i.Bytes())),
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

			var currentCPE []*webfingerprint.CPE
			for _, iotdevResult := range iotdevResults {
				result.Fingerprint.CPEs = append(result.Fingerprint.CPEs, iotdevResult.GetCPE())
				cpeIns, _ := webfingerprint.ParseToCPE(iotdevResult.GetCPE())
				if cpeIns != nil {
					currentCPE = append(currentCPE, cpeIns)
				}
			}

			info := i
			cpes, err := f.wfMatcher.MatchWithOptions(info, config.GenerateWebFingerprintConfigOptions()...)
			if err != nil {
				if !strings.Contains(err.Error(), "no rules matched") {
					continue
				}
			}

			// 如果检测到指纹信息
			if len(cpes) > 0 {
				currentCPE = append(currentCPE, cpes...)
				urlStr := info.URL.String()
				cpeAnalyzer.Feed(urlStr, cpes...)
				results.Store(urlStr, cpes)
			}

			requestHeader, requestBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(info.RequestRaw)
			flow := &HTTPFlow{
				StatusCode:     info.StatusCode,
				IsHTTPS:        info.IsHttps,
				RequestHeader:  []byte(requestHeader),
				RequestBody:    requestBody,
				ResponseHeader: info.ResponseHeaderBytes(),
				ResponseBody:   info.Body,
				CPEs:           currentCPE,
			}
			httpflows = append(httpflows, flow)
		}
	}
	urlCpe := map[string][]*webfingerprint.CPE{}
	results.Range(func(key, value interface{}) bool {
		log.Debugf("url: %s cpes: %#v", key, value)
		_url := key.(string)
		cpes := value.([]*webfingerprint.CPE)
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

	if httpBanners != nil {
		result.State = OPEN
		result.Fingerprint.Proto = TCP
		if result.Fingerprint.ServiceName == "" {
			if httpBanners[0].IsHttps {
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
