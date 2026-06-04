package aireact

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/browser"
	"github.com/yaklang/yaklang/common/log"
)

var _ aicommon.BrowserSessionTracker = (*ReAct)(nil)

// TrackBrowserSession records a browser instance id opened during this ReAct session.
func (r *ReAct) TrackBrowserSession(id string) {
	if r == nil {
		return
	}
	id = trimBrowserSessionID(id)
	if id == "" {
		return
	}
	r.browserSessionsMu.Lock()
	defer r.browserSessionsMu.Unlock()
	if r.browserSessionIDs == nil {
		r.browserSessionIDs = make(map[string]struct{})
	}
	r.browserSessionIDs[id] = struct{}{}
}

// CloseTrackedBrowserSessions closes all browser instances tracked for this ReAct session.
func (r *ReAct) CloseTrackedBrowserSessions() {
	if r == nil {
		return
	}
	r.browserSessionsMu.Lock()
	ids := make([]string, 0, len(r.browserSessionIDs))
	for id := range r.browserSessionIDs {
		ids = append(ids, id)
	}
	r.browserSessionIDs = make(map[string]struct{})
	r.browserSessionsMu.Unlock()

	if len(ids) == 0 {
		return
	}
	log.Infof("react session ending, closing %d tracked browser session(s): %v", len(ids), ids)

	for _, id := range ids {
		if err := browser.CloseByID(browser.WithID(id)); err != nil {
			log.Warnf("close tracked browser session %q on react shutdown: %v", id, err)
		} else {
			log.Infof("closed tracked browser session %q on react shutdown", id)
		}
	}
}

func trimBrowserSessionID(id string) string {
	return strings.TrimSpace(id)
}
