package browser

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
)

type BrowserInstance struct {
	mu         sync.Mutex
	id         string
	browser    *rod.Browser
	pages      []*BrowserPage
	config     *BrowserConfig
	closed     bool
	controlURL string
}

func newBrowserInstance(id string, config *BrowserConfig) (*BrowserInstance, error) {
	inst := &BrowserInstance{
		id:     id,
		config: config,
	}
	err := inst.launch()
	if err != nil {
		return nil, err
	}
	return inst, nil
}

func (inst *BrowserInstance) launch() error {
	inst.browser = rod.New()

	if inst.config.controlURL != "" {
		return inst.connectDirect()
	}
	if inst.config.wsAddress != "" {
		return inst.connectRemote()
	}
	return inst.launchLocal()
}

func (inst *BrowserInstance) launchLocal() error {
	l := launcher.New()

	if inst.config.exePath != "" {
		l = l.Bin(inst.config.exePath)
	}
	if inst.config.proxyAddress != "" {
		l.Proxy(inst.config.proxyAddress)
	}

	l = l.Set("disable-features", "HttpsUpgrades")
	if strings.Contains(runtime.GOOS, "windows") {
		l = l.Set("no-first-run", "")
		l = l.Set("no-default-browser-check", "")
		l = l.Set("disable-default-apps", "")
	}

	l = l.NoSandbox(inst.config.noSandBox).
		Headless(inst.config.headless).
		Leakless(inst.config.leakless)

	controlURL, err := l.Launch()
	if err != nil {
		return fmt.Errorf("launch browser: %w", err)
	}

	inst.controlURL = controlURL
	inst.browser = inst.browser.ControlURL(controlURL)
	err = inst.browser.Connect()
	if err != nil {
		return fmt.Errorf("connect to browser: %w", err)
	}

	_ = inst.browser.IgnoreCertErrors(true)
	log.Infof("browser instance %q launched (local, controlURL=%s)", inst.id, controlURL)
	return nil
}

func (inst *BrowserInstance) connectDirect() error {
	inst.controlURL = inst.config.controlURL
	inst.browser = inst.browser.ControlURL(inst.controlURL)
	err := inst.browser.Connect()
	if err != nil {
		return fmt.Errorf("connect to browser via controlURL %s: %w", inst.controlURL, err)
	}

	_ = inst.browser.IgnoreCertErrors(true)
	log.Infof("browser instance %q connected (direct: %s)", inst.id, inst.controlURL)
	return nil
}

func (inst *BrowserInstance) connectRemote() error {
	ctx := context.Background()
	l, err := launcher.NewManaged(inst.config.wsAddress)
	if err != nil {
		return fmt.Errorf("create managed launcher for %s: %w", inst.config.wsAddress, err)
	}

	if inst.config.proxyAddress != "" {
		l.Proxy(inst.config.proxyAddress)
	}

	l = l.Context(ctx).Set("disable-features", "HttpsUpgrades")
	if strings.Contains(runtime.GOOS, "windows") {
		l = l.Set("no-first-run", "")
		l = l.Set("no-default-browser-check", "")
		l = l.Set("disable-default-apps", "")
	}

	l = l.NoSandbox(inst.config.noSandBox).
		Headless(inst.config.headless).
		Leakless(inst.config.leakless)

	serviceURL, header := l.ClientHeader()
	client, err := cdp.StartWithURL(ctx, serviceURL, header)
	if err != nil {
		return fmt.Errorf("start cdp client %s: %w", serviceURL, err)
	}

	inst.browser = inst.browser.Client(client)
	err = inst.browser.Connect()
	if err != nil {
		return fmt.Errorf("connect to remote browser: %w", err)
	}

	_ = inst.browser.IgnoreCertErrors(true)
	log.Infof("browser instance %q connected (remote: %s)", inst.id, inst.config.wsAddress)
	return nil
}

func (inst *BrowserInstance) ID() string {
	return inst.id
}

func (inst *BrowserInstance) ControlURL() string {
	return inst.controlURL
}

func (inst *BrowserInstance) Navigate(urlStr string) (*BrowserPage, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.closed {
		return nil, fmt.Errorf("browser instance %q is closed", inst.id)
	}

	page, err := inst.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}

	bp := newBrowserPage(page, inst, inst.config.timeout)
	err = bp.Navigate(urlStr)
	if err != nil {
		page.Close()
		return nil, err
	}

	inst.pages = append(inst.pages, bp)
	return bp, nil
}

func (inst *BrowserInstance) CurrentPage() (*BrowserPage, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.closed {
		return nil, fmt.Errorf("browser instance %q is closed", inst.id)
	}

	if len(inst.pages) == 0 {
		pages, err := inst.browser.Pages()
		if err != nil {
			return nil, fmt.Errorf("get pages: %w", err)
		}
		if len(pages) == 0 {
			return nil, fmt.Errorf("no pages open in browser %q", inst.id)
		}
		bp := newBrowserPage(pages[len(pages)-1], inst, inst.config.timeout)
		inst.pages = append(inst.pages, bp)
		return bp, nil
	}

	return inst.pages[len(inst.pages)-1], nil
}

func (inst *BrowserInstance) ListTabs() ([]map[string]string, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.closed {
		return nil, fmt.Errorf("browser instance %q is closed", inst.id)
	}

	pages, err := inst.browser.Pages()
	if err != nil {
		return nil, fmt.Errorf("list tabs: %w", err)
	}

	var tabs []map[string]string
	for i, p := range pages {
		info, err := p.Info()
		if err != nil {
			tabs = append(tabs, map[string]string{
				"index": fmt.Sprintf("%d", i),
				"url":   "(unknown)",
				"title": "(unknown)",
			})
			continue
		}
		tabs = append(tabs, map[string]string{
			"index": fmt.Sprintf("%d", i),
			"url":   info.URL,
			"title": info.Title,
		})
	}
	return tabs, nil
}

func (inst *BrowserInstance) NewTab(urlStr string) (*BrowserPage, error) {
	return inst.Navigate(urlStr)
}

func (inst *BrowserInstance) SwitchTab(index int) (*BrowserPage, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.closed {
		return nil, fmt.Errorf("browser instance %q is closed", inst.id)
	}

	pages, err := inst.browser.Pages()
	if err != nil {
		return nil, fmt.Errorf("switch tab: %w", err)
	}

	if index < 0 || index >= len(pages) {
		return nil, fmt.Errorf("tab index %d out of range [0, %d)", index, len(pages))
	}

	targetPage := pages[index]
	activatedPage, err := targetPage.Activate()
	if err != nil {
		return nil, fmt.Errorf("activate tab %d: %w", index, err)
	}

	bp := newBrowserPage(activatedPage, inst, inst.config.timeout)
	return bp, nil
}

func (inst *BrowserInstance) CloseTab(index int) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.closed {
		return fmt.Errorf("browser instance %q is closed", inst.id)
	}

	pages, err := inst.browser.Pages()
	if err != nil {
		return fmt.Errorf("close tab: %w", err)
	}

	if index < 0 || index >= len(pages) {
		return fmt.Errorf("tab index %d out of range [0, %d)", index, len(pages))
	}

	return pages[index].Close()
}

func (inst *BrowserInstance) Close() error {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.closed {
		return nil
	}
	inst.closed = true

	for _, p := range inst.pages {
		_ = p.Close()
	}
	inst.pages = nil

	log.Infof("browser instance %q closing", inst.id)
	return inst.browser.Close()
}

func (inst *BrowserInstance) IsClosed() bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.closed
}
