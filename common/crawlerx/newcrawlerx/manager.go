// Package newcrawlerx
// @Author bcy2007  2023/3/7 11:49
package newcrawlerx

type BrowserManager struct {
	browsers []*BrowserStarter

	config *Config
}

func NewBrowserManager(config *Config) *BrowserManager {
	manager := &BrowserManager{
		config: config,
	}
	return manager
}

func (manager *BrowserManager) CreateBrowserStarters() {
	if len(manager.config.browsers) == 0 {
		starter := NewBrowserStarter(&NewBrowserConfig{}, manager.config.baseConfig)
		manager.browsers = append(manager.browsers, starter)
		return
	}
	for _, browserConfig := range manager.config.browsers {
		starter := NewBrowserStarter(browserConfig, manager.config.baseConfig)
		manager.browsers = append(manager.browsers, starter)
	}
}

func (manager *BrowserManager) Run() {
	if manager.config.baseConfig.vue {
		for _, starter := range manager.browsers {
			go starter.Run()
		}
	} else {
		for _, starter := range manager.browsers {
			go starter.MultiRun()
		}
	}
}
