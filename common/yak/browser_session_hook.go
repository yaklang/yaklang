package yak

import (
	"github.com/yaklang/yaklang/common/browser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

type browserSessionTracker interface {
	TrackBrowserSession(id string)
}

func registerBrowserSessionHooks(engine *ScriptEngine, tracker browserSessionTracker) {
	if engine == nil || tracker == nil {
		return
	}
	engine.RegisterEngineHooks(func(ae *antlr4yak.Engine) error {
		return browser.RegisterAISessionTrackerHooks(ae, tracker)
	})
}
