package core

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type PageBlock struct {
	// page block struct instead of only page to save page depth
	page  *rod.Page
	depth int
}

func (p PageBlock) GoDeeper() {
	p.depth++
}

func (p PageBlock) GoBack() {
	p.depth--
}

func (m *Manager) GetBrowser() (*rod.Browser, error) {
	create := func() (*rod.Browser, error) {
		// browser := rod.New().Context(context.Background())
		browser := rod.New().Context(m.rootContext)
		err := browser.Connect()
		if err != nil {
			return nil, err
		}
		return browser, nil
	}
	return m.BrowserPool.Get(create)
}

func (m *Manager) PutBrowser(b *rod.Browser) {
	// put browser to browser pool
	m.BrowserPool.Put(b)
	m.BrowserPool.Cleanup(func(browser *rod.Browser) { browser.MustClose() })
}

func (m *Manager) GetPage(opts proto.TargetCreateTarget, depth int) (*PageBlock, error) {
	var err error
	var page *rod.Page
	create := func() (*rod.Page, error) {
		// page, err = m.Browser.Timeout(time.Duration(m.config.timeout) * time.Second).Page(opts)
		page, err = m.Browser.Page(opts)
		if err != nil {
			return nil, err
		}
		return page, nil
	}
	p, err := m.PagePool.Get(create)
	if err != nil {
		return nil, err
	}
	return &PageBlock{page: p, depth: depth}, err
}

func (m *Manager) PutPage(p *rod.Page) {
	// put page to page pool
	p = p.CancelTimeout()
	m.PagePool.Put(p)
}
