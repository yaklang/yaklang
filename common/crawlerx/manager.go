// Package crawlerx
// @Author bcy2007  2023/7/14 10:31
package crawlerx

type BrowserManager struct {
	browsers []*BrowserStarter
	config   *Config
}

func NewBrowserManager(config *Config) *BrowserManager {
	return &BrowserManager{
		browsers: make([]*BrowserStarter, 0),
		config:   config,
	}
}

func (manager *BrowserManager) CreateBrowserStarters() {
	if len(manager.config.browsers) == 0 {
		starter := NewBrowserStarter(&BrowserConfig{}, manager.config.baseConfig)
		manager.browsers = append(manager.browsers, starter)
		return
	}
	for _, browserConfig := range manager.config.browsers {
		starter := NewBrowserStarter(browserConfig, manager.config.baseConfig)
		manager.browsers = append(manager.browsers, starter)
	}
}

func (manager *BrowserManager) Start() {
	for _, starter := range manager.browsers {
		go starter.Start()
	}
}
