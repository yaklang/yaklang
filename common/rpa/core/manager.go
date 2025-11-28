package core

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"

	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/rpa/captcha"
	"github.com/yaklang/yaklang/common/rpa/randomforest"
	"github.com/yaklang/yaklang/common/rpa/web"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/gobwas/glob"
)

type Manager struct {
	BrowserPool   rod.Pool[rod.Browser]
	Browser       *rod.Browser
	PagePool      rod.Pool[rod.Page]
	hijackRouters *rod.HijackRouter
	rootContext   context.Context
	rootCancel    context.CancelFunc

	StartUrls []string
	hijacked  *StringFilterwithCount
	visited   filter.Filterable

	// visitedUrl *filter.StringFilter
	mainDomain string
	captchaUrl string

	whiteSubdomainGlob []glob.Glob
	whiteNetwork       []*net.IPNet // C段
	blackSubdomainGlob []glob.Glob
	blackNetwork       []*net.IPNet
	excludedSuffix     []string
	excludedFileName   []string
	includedSuffix     []string

	// 控制页面并发
	concurrent         int
	pageSizedWaitGroup *utils.SizedWaitGroup

	config *Config

	depth int

	channel chan RequestIf

	rfmodel     *randomforest.UrlDetectSys
	rfmodelpath string

	// record data sended to ch
	// sended *filter.StringFilter
	// url count
	urlCount  int
	detailLog bool
}

type Req struct {
	// 当前请求所属深度
	depth int

	request         *http.Request
	requestRaw      []byte
	response        *http.Response
	responseHeaders *http.Header
	responseBody    []byte

	// 如果请求失败了，原因是
	err error

	// 这个请求是不是可能和登录相关？
	baseURL *url.URL
}

func (m *Manager) init() error {
	// proxy_address := ""
	if m.config.browser_proxy != "" {
		lh := launcher.New()
		lh = lh.Set(flags.ProxyServer, m.config.browser_proxy)
		
		// 在 Windows 上防止 Chrome 创建桌面快捷方式
		if strings.Contains(runtime.GOOS, "windows") {
			lh = lh.Set("no-first-run", "")
			lh = lh.Set("no-default-browser-check", "")
			lh = lh.Set("disable-default-apps", "")
		}
		
		controlURL, _ := lh.Launch()
		m.Browser = m.Browser.ControlURL(controlURL)
	}

	// m.Browser = m.Browser.Context(context.Background())
	m.Browser = m.Browser.Context(m.rootContext)
	err := m.Browser.Connect()

	if m.config.browser_proxy != "" {
		if m.config.proxy_username != "" {
			go m.Browser.MustHandleAuth(m.config.proxy_username, m.config.proxy_password)()
		}
		m.Browser.MustIgnoreCertErrors(true)
	}

	if err != nil {
		return utils.Errorf("connect error: %s", err)
	}
	// timeout
	if m.config.timeout == 0 {
		m.config.timeout = 20
	}
	// log.Infof("timeout:%s", m.config.timeout)

	// 设置cookie
	if len(m.config.cookie) > 0 {
		m.Browser.SetCookies(m.config.cookie)
	}

	// 设置并发
	if m.concurrent <= 0 {
		m.concurrent = 10
	}
	m.pageSizedWaitGroup = utils.NewSizedWaitGroup(m.concurrent)

	// 设置爬虫基础的限制：限制域名和 IP
	for _, u := range m.StartUrls {
		host, _, err := utils.ParseStringToHostPort(u)
		if err != nil {
			log.Errorf("url is not valid: %v reason: %v", u, err)
			continue
		}
		if utils.IsIPv4(host) {
			whiteNet := net.IPNet{
				IP:   net.ParseIP(host),
				Mask: []byte{0xff, 0xff, 0xff, 0},
			}
			if whiteNet.IP != nil {
				m.whiteNetwork = append(m.whiteNetwork, &whiteNet)
				globSelf, _ := glob.Compile(host + "*")
				if globSelf != nil {
					m.whiteSubdomainGlob = append(m.whiteSubdomainGlob, globSelf)
				}
			}
		} else {
			globSelf, err := glob.Compile("*" + host)
			if err != nil {
				log.Errorf("compile glob *%v failed: %s", host, err)
				continue
			}
			m.whiteSubdomainGlob = append(m.whiteSubdomainGlob, globSelf)
		}
	}

	// 劫持请求
	log.Infof("start to set hijack requests")
	m.hijackRouters = m.Browser.HijackRequests()
	m.hijackRouters.MustAdd("*", func(hijack *rod.Hijack) {
		// defer hijack.ContinueRequest(&proto.FetchContinueRequest{})
		urlRaw := hijack.Request.URL()
		for k, v := range m.config.headers {
			hijack.Request.Req().Header.Set(k, v)
		}
		err := hijack.LoadResponse(http.DefaultClient, true)
		if err != nil {
			if !strings.Contains(err.Error(), "context canceled") {
				log.Errorf("load response error: %s", err)
			}
			hijack.Response.Payload().ResponseCode = 200
			hijack.Response.SetHeader()
			hijack.Response.SetBody("")
			return
		}
		if !m.checkFileSuffixValid(urlRaw.String()) && !m.checkHostIsValid(urlRaw.String()) {
			return
		}
		// hash := utils.RodHijackToUniqueHash(hijack)
		// if m.hijacked.Exist(hash) {
		// 	return
		// }
		// m.hijacked.Insert(hash)
		urlStr := urlRaw.String()
		hash := codec.Sha256(urlStr)
		if m.hijacked.Exist(hash) {
			return
		}
		m.hijacked.Insert(hash)

		reqIns := hijack.Request.Req()
		if reqIns.Proto == "" {
			reqIns.Proto = "1.1"
			reqIns.ProtoMajor = 1
			reqIns.ProtoMinor = 1
		}

		rawPacket, err := utils.HttpDumpWithBody(reqIns, true)
		if err != nil {
			log.Errorf("dump body failed: %s", err)
			return
		}
		_ = rawPacket

		r := &Req{}
		r.baseURL = urlRaw
		r.request = reqIns
		r.requestRaw = rawPacket

		resHeader := hijack.Response.Headers()
		resBody := hijack.Response.Body()
		r.responseHeaders = &resHeader
		r.responseBody = []byte(resBody)

		if m.config.onRequest != nil {
			m.config.onRequest(r)
		}
		if m.urlCount != 0 && m.hijacked.Count() >= int64(m.urlCount) {
			m.rootCancel()
		}
	})

	go func() {
		m.hijackRouters.Run()
	}()

	// rf model init
	if m.config.strict_url {
		_, err = os.Stat(m.rfmodelpath)
		if err != nil {
			log.Errorf("random forest model not exist.")
			m.rfmodel = nil
		} else {
			m.rfmodel.LoadModel(m.rfmodelpath)
		}
	} else {
		m.rfmodel = nil
	}

	log.Info("start to run rpcCrawler.init()")
	return nil
}

func NewManager(urls string, ch chan RequestIf, opts ...ConfigOpt) (*Manager, error) {
	ctx, cancel := context.WithCancel(context.Background())

	config := &Config{
		formFill:     defaultFillForm,
		spider_depth: 3,
		// to be deleted
		captchaUrl: captcha.CAPTCHA_URL,
	}

	for _, opt := range opts {
		opt(config)
	}
	m := &Manager{
		BrowserPool:      rod.NewBrowserPool(3),
		PagePool:         rod.NewPagePool(50),
		config:           config,
		Browser:          rod.New(),
		StartUrls:        utils.ParseStringToUrlsWith3W(urls),
		rootContext:      ctx,
		rootCancel:       cancel,
		hijacked:         NewFilterwithCount(),
		visited:          filter.NewCuckooFilter(),
		excludedSuffix:   defaultExcludedSuffix,
		excludedFileName: defaultExcludedFileName,

		concurrent: 20,
		depth:      config.spider_depth,

		channel:     ch,
		rfmodel:     &randomforest.UrlDetectSys{},
		rfmodelpath: "D:\\Workspace\\yak\\common\\rpa\\randomforest\\rf.model",
		// captmanager: cap,
		urlCount: config.url_count,

		whiteSubdomainGlob: config.white_subdomain,
		blackSubdomainGlob: config.black_subdomain,

		detailLog: false,

		mainDomain: web.GetMainDomain(urls),
		captchaUrl: config.captchaUrl,
	}
	err := m.init()
	if err != nil {
		return nil, utils.Errorf("initialize crawler(browser) failed: %s", err)
	}

	return m, nil
}

func (m *Manager) Run() error {
	defer m.Release(m.Browser)
	return m.RunContext(m.rootContext)
}

func (m *Manager) RunContext(ctx context.Context) error {
	if len(m.StartUrls) <= 0 {
		return utils.Errorf("empty urls")
	}
	for _, u := range m.StartUrls {
		m.pageSizedWaitGroup.AddWithContext(m.rootContext)
		urlStr := u
		go func() {
			err := m.page(urlStr, 1)
			if err != nil && m.detailLog {
				log.Errorf("visit page error: %s", err)
			}
		}()
	}
	m.pageSizedWaitGroup.Wait()
	return nil
}

func CleanupPage(page *rod.Page) {
	log.Infof("clean page %s", page)
	err := page.Close()
	if err != nil {
		log.Errorf("clean up page error: %s", err)
	}
	log.Infof("clean page %s end.", page)
}

func (m *Manager) Release(browser *rod.Browser) {
	m.visited.Close()
	err := browser.Close()
	if err != nil && m.detailLog {
		log.Errorf("clean up browser error: %s", err)
	}
}
