package crawlerx

import (
	"yaklang/common/crawlerx/core"
	"yaklang/common/utils"
)

type Crawler struct {
	url  string
	opts []core.ConfigOpt
}

func CreateCrawler(urlStr string) *Crawler {
	return &Crawler{
		url: urlStr,
	}
}

func (crawler *Crawler) SetProxy(proxyAddr string, proxyInfo ...string) {
	crawler.opts = append(crawler.opts, core.WithProxy(proxyAddr, proxyInfo...))
}

func (crawler *Crawler) SetMaxUrl(maxUrl int) {
	crawler.opts = append(crawler.opts, core.WithMaxUrl(maxUrl))
}

func (crawler *Crawler) SetWhiteList(whiteRegStr string) {
	crawler.opts = append(crawler.opts, core.WithWhiteList(whiteRegStr))
}

func (crawler *Crawler) SetBlackList(blackRegStr string) {
	crawler.opts = append(crawler.opts, core.WithBlackList(blackRegStr))
}

func (crawler *Crawler) SetTimeout(timeout int) {
	crawler.opts = append(crawler.opts, core.WithTimeout(timeout))
}

func (crawler *Crawler) SetMaxDepth(depth int) {
	crawler.opts = append(crawler.opts, core.WithMaxDepth(depth))
}

func (crawler *Crawler) SetFormFill(key, value string) {
	crawler.opts = append(crawler.opts, core.WithFormFill(key, value))
}

func (crawler *Crawler) SetHeader(key, value string) {
	crawler.opts = append(crawler.opts, core.WithHeader(key, value))
}

func (crawler *Crawler) SetHeaders(kv map[string]string) {
	crawler.opts = append(crawler.opts, core.WithHeaders(kv))
}

func (crawler *Crawler) SetConcurrent(concurrent int) {
	crawler.opts = append(crawler.opts, core.WithConcurrent(concurrent))
}

func (crawler *Crawler) SetCookie(domain, k, v string) {
	crawler.opts = append(crawler.opts, core.WithCookie(domain, k, v))
}

func (crawler *Crawler) SetCookies(domain string, value map[string]string) {
	crawler.opts = append(crawler.opts, core.WithCookies(domain, value))
}

func (crawler *Crawler) SetScanRange(scanRange int) {
	crawler.opts = append(crawler.opts, core.WithScanRange(scanRange))
}

func (crawler *Crawler) SetScanRepeatLevel(scanRepeat int) {
	crawler.opts = append(crawler.opts, core.WithScanRepeat(scanRepeat))
}

func (crawler *Crawler) GetChannel() chan core.ReqInfo {
	ch := make(chan core.ReqInfo)
	crawler.opts = append(crawler.opts, core.WithChannel(ch))
	return ch
}

func (crawler *Crawler) SetOnRequest(f func(core.ReqInfo)) {
	crawler.opts = append(crawler.opts, core.WithOnRequest(f))
}

func (crawler *Crawler) SetDangerUrlCheck() {
	crawler.opts = append(crawler.opts, core.WithCheckDanger())
}

func (crawler *Crawler) SetTags(tagsPath string) {
	crawler.opts = append(crawler.opts, core.WithTags(tagsPath))
}

func (crawler *Crawler) SetFullTimeout(timeout int) {
	crawler.opts = append(crawler.opts, core.WithFullCrawlerTimeout(timeout))
}

func (crawler *Crawler) SetChromeWS(wsAddress string) {
	crawler.opts = append(crawler.opts, core.WithChromeWS(wsAddress))
}

func (crawler *Crawler) SetUrlFromProxy(ifYes bool) {
	crawler.opts = append(crawler.opts, core.WithGetUrlRemote(ifYes))
}

func (crawler *Crawler) SetExtraHeaders(headers ...string) {
	crawler.opts = append(crawler.opts, core.WithExtraHeaders(headers...))
}

func (crawler *Crawler) Start() error {
	c, err := core.NewCrawler(crawler.url, crawler.opts...)
	if err != nil {
		return utils.Errorf("create crawler error: %s", err)
	}
	go c.Start()
	return nil
}

func (crawler *Crawler) Monitor() error {
	c, err := core.NewCrawler(crawler.url, crawler.opts...)
	if err != nil {
		return utils.Errorf("create crawler error: %s", err)
	}
	go c.Monitor()
	return nil
}

func (crawler *Crawler) StartV2() error {
	c, err := core.NewCrawlerV2(crawler.url, crawler.opts...)
	if err != nil {
		return utils.Errorf("create crawler error: %s", err)
	}
	//c.PageSizedGroup().AddWithContext(c.RootContext())
	go c.Start()
	//time.Sleep(30 * time.Second)
	//c.PageSizedGroup().Wait()
	return nil
}

func (crawler *Crawler) StartVRemote() error {
	c, err := core.NewCrawlerV2(crawler.url, crawler.opts...)
	if err != nil {
		return utils.Errorf("create crawler(remote ver.) error: %s", err)
	}
	c.PageSizedGroup().AddWithContext(c.RootContext())
	c.StartRemote()
	c.PageSizedGroup().Wait()
	return nil
}

func StartCrawler(url string, opts ...core.ConfigOpt) (chan core.ReqInfo, error) {
	ch := make(chan core.ReqInfo)
	opt := core.WithChannel(ch)
	opts = append(opts, opt)
	c, err := core.NewCrawlerV2(url, opts...)
	if err != nil {
		return nil, utils.Errorf("create crawlerx engine error: %s", err)
	}
	go c.Start()
	return ch, nil
}

func StartCrawlerV2(url string, opts ...core.ConfigOpt) error {
	c, err := core.NewCrawlerV2(url, opts...)
	if err != nil {
		return utils.Errorf("create crawlerx engine error: %s", err)
	}
	c.PageSizedGroup().AddWithContext(c.RootContext())
	c.StartRemote()
	c.PageSizedGroup().Wait()
	return nil
}

func (crawler *Crawler) PageScreenShot() (string, error) {
	c, err := core.NewScreenShotCrawler(crawler.opts...)
	if err != nil {
		return "", utils.Errorf("create crawlerx engine error: %s", err)
	}
	return c.PageScreenShot(crawler.url)
}

func PageScreenShot(url string, opts ...core.ConfigOpt) (string, error) {
	//c, err := core.NewCrawlerV2(url, opts...)
	c, err := core.NewScreenShotCrawler(opts...)
	if err != nil {
		return "", utils.Errorf("create crawlerx engine error: %s", err)
	}
	return c.PageScreenShot(url)
}
