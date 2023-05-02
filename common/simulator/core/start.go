package core

import (
	"golang.org/x/net/context"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/simulator/config"
)

type StartMode struct {
	config  config.PageConfig
	context context.Context
	cancel  context.CancelFunc
	page    *GeneralPage
}

func PageCreator() *StartMode {
	startMode := StartMode{}
	startMode.init()
	return &startMode
}

func (startMode *StartMode) init() {
	ctx, cancalFunc := context.WithCancel(context.Background())
	startMode.context = ctx
	startMode.cancel = cancalFunc
	startMode.setContext(ctx)
	log.Info("init mode")
}

func (mode *StartMode) SetProxy(proxyStr ...string) {
	configFunc := config.WithProxyConfig(proxyStr...)
	configFunc(&mode.config)
}

func (mode *StartMode) SetURL(url string) {
	configFunc := config.WithUrlConfig(url)
	configFunc(&mode.config)
}

func (mode *StartMode) SetWsAddress(wsAddress string) {
	configFunc := config.WithWsAddress(wsAddress)
	configFunc(&mode.config)
}

func (mode *StartMode) setContext(ctx context.Context) {
	configFunc := config.WithContext(ctx)
	configFunc(&mode.config)
}

func (mode *StartMode) Test() {
	//fmt.Println(mode.config
	//mode.Listen()
}

func (startMode *StartMode) Cancel() {
	startMode.page.Close()
	startMode.cancel()
}

func (mode *StartMode) Create() *GeneralPage {
	page, err := CreatePage(mode.config)
	if err != nil {
		log.Errorf("create page error: %s", err)
		return nil
	}
	mode.page = page
	return page
}
