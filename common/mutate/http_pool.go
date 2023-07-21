package mutate

import (
	"bufio"
	"bytes"
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/crawler"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net"
	"net/http"
	"net/http/httptrace"
	"reflect"
	"strings"
	"sync"
	"time"
)

var poolingList sync.Map

type httpPoolConfig struct {
	Size              int
	PerRequestTimeout time.Duration
	IsHttps           bool
	IsGmTLS           bool
	Host              string
	Port              int
	NoSystemProxy     bool
	Proxies           []string
	UseRawMode        bool
	RedirectTimes     int
	NoFollowRedirect  bool
	// NoFollowMetaRedirect             bool
	FollowJSRedirect                 bool
	PayloadsTable                    *sync.Map
	Ctx                              context.Context
	ForceFuzz                        bool
	FuzzParams                       map[string][]string
	NoFixContentLength               bool
	ExtraRegexpMutateCondition       []*RegexpMutateCondition
	ExtraRegexpMutateConditionGetter func() *RegexpMutateCondition
	DelayMinSeconds                  float64
	DelayMaxSeconds                  float64

	// beforeRequest
	// afterRequest
	HookBeforeRequest func([]byte) []byte
	HookAfterRequest  func([]byte) []byte

	// 请求来源
	Source string

	// 强制使用 h2
	ForceHttp2 bool

	// 重试
	RetryTimes           int
	RetryInStatusCode    []int
	RetryNotInStatusCode []int
	RetryWaitTime        float64
	RetryMaxWaitTime     float64

	// DNSServers
	DNSServers []string
	EtcHosts   map[string]string
}

func _httpPool_NoSystemProxy(b bool) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.NoSystemProxy = b
	}
}

func _httpPool_DNSServers(i []string) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.DNSServers = i
	}
}

func _httpPool_EtcHosts(kv []*ypb.KVPair) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		if config.EtcHosts == nil {
			config.EtcHosts = make(map[string]string)
		}
		for _, i := range kv {
			config.EtcHosts[i.GetKey()] = i.GetValue()
		}
	}
}

func _httpPool_Retry(i int) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.RetryTimes = i
	}
}

func _httpPool_RetryWaitTime(i float64) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.RetryWaitTime = i
	}
}

func _httpPool_RetryMaxWaitTime(i float64) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.RetryMaxWaitTime = i
	}
}

func _httpPool_RetryInStatusCode(codes []int) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.RetryInStatusCode = codes
	}
}

func _httpPool_RetryNotInStatusCode(codes []int) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.RetryNotInStatusCode = codes
	}
}

func _hoopPool_SetHookCaller(before func([]byte) []byte, after func([]byte) []byte) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.HookBeforeRequest = before
		config.HookAfterRequest = after
	}
}

func _httpPool_Source(i string) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.Source = i
	}
}

func _httpPool_SetFuzzParams(i interface{}) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		if i != nil {
			config.FuzzParams = utils.InterfaceToMap(i)
		}
	}
}

func _httpPool_SetForceFuzz(b bool) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.ForceFuzz = b
	}
}

func _httpPool_DelaySeconds(b float64) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.DelayMinSeconds = b
		config.DelayMaxSeconds = b
	}
}

func _httpPool_DelayMinSeconds(b float64) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.DelayMinSeconds = b
	}
}

func _httpPool_DelayMaxSeconds(b float64) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.DelayMaxSeconds = b
	}
}

func _httpPool_SetContext(ctx context.Context) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.Ctx = ctx
	}
}

func _httpPool_SetNoFollowRedirect(i bool) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.NoFollowRedirect = i
	}
}

func _httpPool_SetFollowJSRedirect(i bool) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.FollowJSRedirect = i
	}
}

func _httpPool_SetSize(i int) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.Size = i
	}
}

func _httpPool_RawMode(b bool) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.UseRawMode = b
	}
}

func _httpPool_PerRequestTimeout(f float64) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.PerRequestTimeout = utils.FloatSecondDuration(f)
	}
}

func _httpPool_noFixContentLength(f bool) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.NoFixContentLength = f
	}
}

func _httpPool_redirectTimes(f int) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.RedirectTimes = f
	}
}

func _httpPool_Host(h string, isHttps bool) HttpPoolConfigOption {
	return func(c *httpPoolConfig) {
		var lower = strings.ToLower(h)
		if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
			c.Host, c.Port, _ = utils.ParseStringToHostPort(lower)
		} else {
			c.Host, c.Port, _ = utils.ParseStringToHostPort(lower)
			if c.Port <= 0 {
				if isHttps {
					c.Port = 443
				} else {
					c.Port = 80
				}
			}
		}
		if c.Host == "" {
			c.Host = h
		}
	}
}

func _httpPool_Port(port int) HttpPoolConfigOption {
	return func(c *httpPoolConfig) {
		c.Port = port
	}
}

func _httpPool_IsHttps(f bool) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.IsHttps = f
	}
}

func _httpPool_IsGmTLS(f bool) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.IsGmTLS = f
	}
}

func _httpPool_proxies(proxies ...string) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.Proxies = proxies
	}
}

func _httpPool_extraMutateCondition(codes ...*RegexpMutateCondition) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.ExtraRegexpMutateCondition = codes
	}
}

func _httpPool_extraMutateConditionGetter(getter func() *RegexpMutateCondition) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.ExtraRegexpMutateConditionGetter = getter
	}
}

func _httpPool_inner_payload(m *sync.Map) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.PayloadsTable = m
	}
}

func _httpPool_namingContext(invokerName string) HttpPoolConfigOption {
	return func(config *httpPoolConfig) {
		config.Ctx = context.WithValue(config.Ctx, "invoker", invokerName)
	}
}

type HttpPoolConfigOption func(config *httpPoolConfig)

type _httpResult struct {
	Url         string
	Request     *http.Request
	Error       error
	RequestRaw  []byte
	ResponseRaw []byte
	Response    *http.Response
	Payloads    []string
	params      []interface{}

	DurationMs       int64
	ServerDurationMs int64
	Timestamp        int64
	// 如果有关联插件的话，这就是插件名
	Source string

	LowhttpResponse *lowhttp.LowhttpResponse
}

func NewDefaultHttpPoolConfig(opts ...HttpPoolConfigOption) *httpPoolConfig {
	base := &httpPoolConfig{
		Size:              50,
		PerRequestTimeout: 10 * time.Second,
		IsHttps:           false,
		IsGmTLS:           false,
		UseRawMode:        true,
		RedirectTimes:     5,
		NoFollowRedirect:  true,
		FollowJSRedirect:  false,
		Ctx:               context.Background(),
		ForceFuzz:         true,
	}
	for _, opt := range opts {
		opt(base)
	}
	return base
}

func _httpPool(i interface{}, opts ...HttpPoolConfigOption) (chan *_httpResult, error) {
	config := NewDefaultHttpPoolConfig(opts...)
	if len(config.Proxies) <= 0 && utils.GetProxyFromEnv() != "" && !config.NoSystemProxy {
		WithPoolOpt_Proxy(utils.GetProxyFromEnv())(config)
	}

	switch ret := i.(type) {
	case []*MutateResult:
		var payloadsTable = new(sync.Map) // map[string][]string
		var results [][]byte
		for _, e := range ret {
			res := []byte(e.Result)
			results = append(results, res)
			//utils.Debug(func() {
			//	log.Infof("hash: %v payloads: %v", codec.Sha256(res), e.Payloads)
			//})
			payloadsTable.Store(codec.Sha256(res), e.Payloads)
		}
		opts = append(opts, _httpPool_inner_payload(payloadsTable))
		return _httpPool(results, opts...)
	case []*http.Request:
		if len(ret) <= 0 {
			return nil, utils.Errorf("empty target requests: %v", ret)
		}

		if config.Ctx.Value("invoker") != nil { //caller set NamingContext
			group := utils.NewSizedWaitGroup(config.Size)
			wg, _ := poolingList.LoadOrStore(config.Ctx.Value("invoker"), &group)
			wg.(*utils.SizedWaitGroup).Add()
			defer func() { wg.(*utils.SizedWaitGroup).Done() }()
		}

		if config.UseRawMode || config.Host != "" {
			var results [][]byte
			for _, e := range ret {
				res, err := utils.HttpDumpWithBody(e, true)
				if err != nil {
					return nil, err
				}
				results = append(results, res)
			}
			return _httpPool(results, opts...)
		}

		// 配置 HTTP 客户端
		client := utils.NewDefaultHTTPClient()
		if config.PerRequestTimeout > 0 {
			client.Timeout = config.PerRequestTimeout
		}
		var err error
		if config.Proxies != nil {
			httpTr := client.Transport.(*http.Transport)
			httpTr.Proxy, err = crawler.RoundRobinProxySwitcher(config.Proxies...)
			if err != nil {
				log.Errorf("set proxy[%v] failed: %s", config.Proxies, err)
			}
		}
		results := make(chan *_httpResult, len(ret))

		// 开始进行数据包发送
		go func() {
			defer close(results)

			//delayer := utils.NewDelayWaiter()
			delayer, _ := utils.NewFloatSecondsDelayWaiter(config.DelayMinSeconds, config.DelayMaxSeconds)

			swg := utils.NewSizedWaitGroup(config.Size)
			for _, rawRequest := range ret {
				if config.Ctx != nil {
					select {
					case <-config.Ctx.Done():
						return
					default:
					}
				}
				// 防止对原始请求进行修改，所以这里需要进行深拷贝
				rawRequestRaw, err := utils.HttpDumpWithBody(rawRequest, true)
				if err != nil {
					log.Errorf("dump request failed: %s", err)
					continue
				}
				targetRequest, err := lowhttp.ParseStringToHttpRequest(string(rawRequestRaw))
				if err != nil {
					log.Errorf("parse request failed: %s", err)
					continue
				}
				requestRaw, err := utils.HttpDumpWithBody(targetRequest, true)
				if err != nil {
					log.Errorf("marshal http.Request failed: %s", err)
				}
				targetRequest.URL.Scheme = rawRequest.URL.Scheme
				swg.Add()
				go func() {
					defer func() {
						if delayer != nil {
							delayer.Wait()
						}
						swg.Done()
					}()
					defer func() {
						if err := recover(); err != nil {
							log.Error(err)
						}
					}()

					var (
						https                    bool
						raw                      []byte
						rspIns                   *http.Response
						err                      error
						duration, serverDuration time.Duration
					)

					if config.IsHttps {
						https = config.IsHttps
					}
					if targetRequest.URL.Scheme == "https" {
						https = true
					}
					if config.IsGmTLS {
						https = true
						httpTr := client.Transport.(*http.Transport)
						httpTr.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
							dialer := &net.Dialer{}
							conn, err := gmtls.DialWithDialer(dialer, network, addr, &gmtls.Config{
								GMSupport:          &gmtls.GMSupport{},
								InsecureSkipVerify: true,
							})
							if err != nil {
								return nil, err
							}
							ctx, _ = context.WithTimeout(ctx, 30*time.Second)
							return ctxio.NewConn(ctx, conn), nil
						}
					}

					url, err := lowhttp.ExtractURLFromHTTPRequest(targetRequest, https)
					if err != nil {
						log.Errorf("parse request 'url failed: %s, enable debug loglevel for more info", err)
						return
					}
					targetRequest.URL = url

					log.Infof("start to send to %v (http.client mode)", url.String())
					var ac *http.Request
					ac, err = utils.FixHTTPRequestForHTTPDo(targetRequest)
					if err != nil {
						log.Errorf("%v", err)
						return
					}
					utils.Debug(func() {
						raw, _ := utils.HttpDumpWithBody(targetRequest, true)
						log.Debugf("request old: \n%v", string(raw))

						raw, _ = utils.HttpDumpWithBody(ac, true)
						log.Debugf("request new: \n%v", string(raw))
					})
					// start := time.Now()
					var (
						connDone time.Time
					)
					trace := &httptrace.ClientTrace{
						ConnectDone: func(network, addr string, err error) {
							connDone = time.Now()
						},

						GotFirstResponseByte: func() {
							serverDuration = time.Since(connDone)
						},
					}
					ac = ac.WithContext(httptrace.WithClientTrace(ac.Context(), trace))
					start := time.Now()
					rspIns, err = client.Do(ac)
					duration = time.Since(start)

					if rspIns != nil {
						raw, _ = utils.HttpDumpWithBody(rspIns, true)
					}

					ret := &_httpResult{
						Source:           config.Source,
						Url:              url.String(),
						Request:          targetRequest,
						Error:            err,
						RequestRaw:       requestRaw,
						ResponseRaw:      raw,
						Response:         rspIns,
						DurationMs:       duration.Milliseconds(),
						ServerDurationMs: serverDuration.Milliseconds(),
						Timestamp:        time.Now().Unix(),
					}
					if ret.Response == nil && raw != nil {
						ret.Response, err = http.ReadResponse(bufio.NewReader(bytes.NewBuffer(raw)), targetRequest)
						if err != nil {
							log.Errorf("parse bytes to response failed: %s", err)
						}
					}
					results <- ret
				}()
			}

			swg.Wait()
		}()

		return results, nil
	case FuzzHTTPRequestIf:
		reqs, err := ret.Results()
		if err != nil {
			return nil, utils.Errorf("call FuzzHTTPRequest.Results() failed: %s", err)
		}
		return _httpPool(reqs, opts...)
	case FuzzHTTPRequestBatch:
		reqs, err := ret.Results()
		if err != nil {
			return nil, utils.Errorf("call FuzzHTTPRequestBatch.Results() failed: %s", err)
		}
		return _httpPool(reqs, opts...)
	case FuzzHTTPRequest:
		reqs, err := ret.Results()
		if err != nil {
			return nil, utils.Errorf("call FuzzHTTPRequestBatch.Results() failed: %s", err)
		}
		return _httpPool(reqs, opts...)
	case *http.Request:
		return _httpPool([]*http.Request{ret}, opts...)
	case []interface{}:
		var req []*http.Request
		for _, r := range ret {
			reqIns, ok := r.(*http.Request)
			if !ok {
				log.Errorf("cannot convert %v to *http.Request", reflect.TypeOf(r))
				continue
			}
			req = append(req, reqIns)
		}
		return _httpPool(req, opts...)
	case [][]byte:
		if len(ret) <= 0 {
			return nil, utils.Errorf("empty target requests: %v", ret)
		}

		if config.Size <= 0 {
			config.Size = 50
		}
		results := make(chan *_httpResult, len(ret))

		go func() {
			defer close(results)
			defer func() {
				if e := recover(); e != nil {
					log.Error(e)
				}
			}()
			delayer, _ := utils.NewFloatSecondsDelayWaiter(config.DelayMinSeconds, config.DelayMaxSeconds)

			swg := utils.NewSizedWaitGroup(config.Size)
			submitTask := func(targetRequest []byte, payloads ...string) {
				if config.Ctx != nil {
					select {
					case <-config.Ctx.Done():
						return
					default:
					}
				}

				swg.Add()
				go func() {
					defer func() {
						if delayer != nil {
							delayer.Wait()
						}
						swg.Done()
					}()

					// 处理异常
					defer func() {
						if err := recover(); err != nil {
							log.Errorf("submit fuzzer task failed: %s", err)
						}
					}()

					if config.HookBeforeRequest != nil {
						newRequest := config.HookBeforeRequest(targetRequest)
						if len(newRequest) > 0 {
							targetRequest = newRequest
						}
					}

					var (
						urlStr string
					)
					_urlInsRaw, _ := lowhttp.ExtractURLFromHTTPRequestRaw(targetRequest, config.IsHttps)
					if _urlInsRaw != nil {
						urlStr = _urlInsRaw.String()
					}
					reqIns, err := lowhttp.ParseBytesToHttpRequest(targetRequest)
					if err != nil {
						failedResult := &_httpResult{
							Url:        urlStr,
							Error:      err,
							RequestRaw: targetRequest,
							Timestamp:  time.Now().Unix(),
							Payloads:   payloads,
							Source:     config.Source,
						}
						results <- failedResult
						return
					}

					log.Infof("start to send to %v(%v) (packet mode)", urlStr, utils.HostPort(config.Host, config.Port))

					var host string
					var port int
					if config.Host == "" || config.Port <= 0 {
						hostInUrl, portInUrl, _ := utils.ParseStringToHostPort(urlStr)
						host = hostInUrl
						port = portInUrl
					} else {
						host = config.Host
						port = config.Port
					}

					if config.NoFollowRedirect {
						config.FollowJSRedirect = false
						config.RedirectTimes = 0
					}

					rspInstance, err := lowhttp.HTTP(
						lowhttp.WithHttps(config.IsHttps),
						lowhttp.WithHost(host), lowhttp.WithPort(port),
						lowhttp.WithPacketBytes(targetRequest),
						lowhttp.WithTimeout(config.PerRequestTimeout),
						lowhttp.WithRedirectTimes(config.RedirectTimes),
						lowhttp.WithJsRedirect(config.FollowJSRedirect),
						lowhttp.WithContext(config.Ctx),
						lowhttp.WithNoFixContentLength(config.NoFixContentLength),
						lowhttp.WithHttp2(config.ForceHttp2),
						lowhttp.WithSource(config.Source),
						lowhttp.WithProxy(config.Proxies...),
						lowhttp.WithRetryTimes(config.RetryTimes),
						lowhttp.WithRetryInStatusCode(config.RetryInStatusCode),
						lowhttp.WithRetryNotInStatusCode(config.RetryNotInStatusCode),
						lowhttp.WithRetryWaitTime(utils.FloatSecondDuration(config.RetryWaitTime)),
						lowhttp.WithRetryMaxWaitTime(utils.FloatSecondDuration(config.RetryMaxWaitTime)),
						lowhttp.WithDNSServers(config.DNSServers),
						lowhttp.WithETCHosts(config.EtcHosts),
						lowhttp.WithGmTLS(config.IsGmTLS),
					)
					var rsp []byte
					if rspInstance != nil {
						rsp = rspInstance.RawPacket
					}

					if config.HookAfterRequest != nil {
						newRsp := config.HookAfterRequest(rsp)
						if len(newRsp) > 0 {
							rsp = newRsp
						}
					}

					if err != nil {
						log.Errorf("exec packet raw failed: %s", err)
						failedResult := &_httpResult{
							Url:             urlStr,
							Request:         reqIns,
							Error:           err,
							RequestRaw:      targetRequest,
							ResponseRaw:     nil,
							DurationMs:      0,
							Timestamp:       time.Now().Unix(),
							Payloads:        payloads,
							Source:          config.Source,
							LowhttpResponse: rspInstance,
						}
						results <- failedResult
						return
					}
					ret := &_httpResult{
						Url:              urlStr,
						Request:          reqIns,
						Error:            err,
						RequestRaw:       targetRequest,
						ResponseRaw:      rsp,
						DurationMs:       rspInstance.TraceInfo.GetServerDurationMS(),
						ServerDurationMs: rspInstance.TraceInfo.GetServerDurationMS(),
						Timestamp:        time.Now().Unix(),
						Payloads:         payloads,
						Source:           config.Source,
						LowhttpResponse:  rspInstance,
					}
					utils.Debug(func() {
						println(string(rsp))
					})
					if len(rsp) <= 0 {
						ret.Error = utils.Error("服务端没有任何返回数据: empty response (timeout empty)")
					}
					if ret.Response == nil && rsp != nil && !config.NoFixContentLength {
						ret.Response, err = http.ReadResponse(bufio.NewReader(bytes.NewBuffer(rsp)), reqIns)
						if err != nil {
							log.Errorf("parse bytes to response failed: %s", err)
						}
					}
					results <- ret
				}()
			}

			for _, reqRaw := range ret {
				if config.Ctx != nil {
					select {
					case <-config.Ctx.Done():
						return
					default:
					}
				}

				if config.ForceFuzz {
					var conds []*RegexpMutateCondition
					if len(config.FuzzParams) > 0 {
						conds = append(conds, MutateWithExtraParams(config.FuzzParams))
					}
					if len(config.ExtraRegexpMutateCondition) > 0 {
						conds = append(conds, config.ExtraRegexpMutateCondition...)
					}

					if config.ExtraRegexpMutateConditionGetter != nil {
						paramsGetterHandler := config.ExtraRegexpMutateConditionGetter()
						conds = append(conds, paramsGetterHandler)
					}
					_, err := QuickMutateWithCallbackEx2(
						string(reqRaw),
						consts.GetGormProfileDatabase(),
						[]func(result *MutateResult) bool{
							func(result *MutateResult) bool {
								select {
								case <-config.Ctx.Done():
									return false
								default:
								}
								submitTask([]byte(result.Result), result.Payloads...)
								return true
							},
						}, conds...)
					if err != nil {
						log.Errorf("fuzz with callback failed: %s", err)
					}
				} else {
					submitTask(reqRaw)
				}
			}
			swg.Wait()
		}()
		return results, nil
	case []string:
		var results [][]byte
		for _, req := range ret {
			results = append(results, []byte(req))
		}
		return _httpPool(results, opts...)
	case string:
		return _httpPool([][]byte{
			[]byte(ret),
		}, opts...)
	case []byte:
		return _httpPool([][]byte{ret}, opts...)
	default:
		return nil, utils.Errorf("unsupported param type: %v", reflect.TypeOf(i))
	}
}

var HttpPoolExports = map[string]interface{}{
	"Pool": _httpPool,

	// 选项
	"https":              _httpPool_IsHttps,
	"size":               _httpPool_SetSize,
	"host":               _httpPool_Host,
	"port":               _httpPool_Port,
	"proxy":              _httpPool_proxies,
	"perRequestTimeout":  _httpPool_PerRequestTimeout,
	"rawMode":            _httpPool_RawMode,
	"redirectTimes":      _httpPool_redirectTimes,
	"context":            _httpPool_SetContext,
	"fuzz":               _httpPool_SetForceFuzz,
	"fuzzParams":         _httpPool_SetFuzzParams,
	"noFixContentLength": _httpPool_noFixContentLength,
}

var WithPoolOpt_noFixContentLength = _httpPool_noFixContentLength
var WithPoolOpt_Proxy = _httpPool_proxies
var WithPoolOpt_Timeout = _httpPool_PerRequestTimeout
var WithPoolOpt_Concurrent = _httpPool_SetSize
var WithPoolOpt_Addr = _httpPool_Host
var WithPoolOpt_RedirectTimes = _httpPool_redirectTimes
var WithPoolOpt_RawMode = _httpPool_RawMode
var ExecPool = _httpPool
var WithPoolOpt_Https = _httpPool_IsHttps
var WithPoolOpt_GmTLS = _httpPool_IsGmTLS
var WithPoolOpt_NoFollowRedirect = _httpPool_SetNoFollowRedirect
var WithPoolOpt_FollowJSRedirect = _httpPool_SetFollowJSRedirect
var WithPoolOpt_Context = _httpPool_SetContext
var WithPoolOpt_ForceFuzz = _httpPool_SetForceFuzz
var WithPoolOpt_FuzzParams = _httpPool_SetFuzzParams
var WithPoolOpt_ExtraMutateCondition = _httpPool_extraMutateCondition
var WithPoolOpt_ExtraMutateConditionGetter = _httpPool_extraMutateConditionGetter
var WithPoolOpt_DelayMinSeconds = _httpPool_DelayMinSeconds
var WithPoolOPt_DelayMaxSeconds = _httpPool_DelayMaxSeconds
var WithPoolOPt_DelaySeconds = _httpPool_DelaySeconds
var WithPoolOpt_HookCodeCaller = _hoopPool_SetHookCaller
var WithPoolOpt_Source = _httpPool_Source
var WithPoolOpt_NamingContext = _httpPool_namingContext
var WithPoolOpt_RetryTimes = _httpPool_Retry
var WithPoolOpt_RetryInStatusCode = _httpPool_RetryInStatusCode
var WithPoolOpt_RetryNotInStatusCode = _httpPool_RetryNotInStatusCode
var WithPoolOpt_RetryWaitTime = _httpPool_RetryWaitTime
var WithPoolOpt_RetryMaxWaitTime = _httpPool_RetryMaxWaitTime
var WithPoolOpt_DNSServers = _httpPool_DNSServers
var WithPoolOpt_EtcHosts = _httpPool_EtcHosts
var WithPoolOpt_NoSystemProxy = _httpPool_NoSystemProxy
