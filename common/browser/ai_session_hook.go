package browser

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

// AISessionTracker receives browser instance ids when yak scripts call browser.Open.
type AISessionTracker interface {
	TrackBrowserSession(id string)
}

// RegisterAISessionTrackerHooks wraps browser.Open / browser.Close in the yak VM so
// Open registers the instance id (from options or the returned instance) and Close
// is still delegated to the real implementation.
func RegisterAISessionTrackerHooks(engine *antlr4yak.Engine, tracker AISessionTracker) error {
	if engine == nil || tracker == nil {
		return nil
	}
	vm := engine.GetVM()
	if vm == nil {
		return nil
	}

	vm.RegisterMapMemberCallHandler("browser", "Open", wrapBrowserOpen(tracker))
	vm.RegisterMapMemberCallHandler("browser", "Close", wrapBrowserClose())
	return nil
}

func wrapBrowserOpen(tracker AISessionTracker) func(any) any {
	return func(origin any) any {
		openFn, ok := origin.(func(...BrowserOption) (*BrowserInstance, error))
		if !ok {
			log.Warnf("browser.Open hook: unexpected callee type %T", origin)
			return origin
		}
		return func(opts ...BrowserOption) (*BrowserInstance, error) {
			inst, err := openFn(opts...)
			if err != nil || inst == nil {
				return inst, err
			}
			id := inst.ID()
			if id == "" {
				id = ConfigIDFromOptions(opts...)
			}
			if id != "" {
				tracker.TrackBrowserSession(id)
			}
			return inst, err
		}
	}
}

func wrapBrowserClose() func(any) any {
	return func(origin any) any {
		closeFn, ok := origin.(func(...BrowserOption) error)
		if !ok {
			log.Warnf("browser.Close hook: unexpected callee type %T", origin)
			return origin
		}
		return func(opts ...BrowserOption) error {
			return closeFn(opts...)
		}
	}
}
