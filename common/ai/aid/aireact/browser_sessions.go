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
	if r.browserSessionIDs == nil {
		r.browserSessionIDs = make(map[string]struct{})
	}
	r.browserSessionIDs[id] = struct{}{}
	r.browserSessionsMu.Unlock()

	aicommon.NotifySessionSnapshotBrowserOpened(r.config, id)
}

// UntrackBrowserSession removes a browser instance id after browser.Close.
func (r *ReAct) UntrackBrowserSession(id string) {
	if r == nil {
		return
	}
	r.removeBrowserSessionTracking(id)
}

// ListTrackedBrowserSessionIDs returns browser instance ids tracked on this ReAct session.
func (r *ReAct) ListTrackedBrowserSessionIDs() []string {
	if r == nil {
		return nil
	}
	r.browserSessionsMu.Lock()
	defer r.browserSessionsMu.Unlock()
	if len(r.browserSessionIDs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(r.browserSessionIDs))
	for id := range r.browserSessionIDs {
		ids = append(ids, id)
	}
	return ids
}

func (r *ReAct) removeBrowserSessionTracking(id string) {
	id = trimBrowserSessionID(id)
	if id == "" {
		return
	}
	r.browserSessionsMu.Lock()
	delete(r.browserSessionIDs, id)
	r.browserSessionsMu.Unlock()

	aicommon.NotifySessionSnapshotBrowserClosed(r.config, id)
}

// CloseBrowserSession closes one tracked browser instance and updates session_snapshot.
func (r *ReAct) CloseBrowserSession(id string) error {
	id = trimBrowserSessionID(id)
	if id == "" {
		return nil
	}
	if err := browser.CloseByID(browser.WithID(id)); err != nil {
		return err
	}
	r.removeBrowserSessionTracking(id)
	return nil
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
		r.removeBrowserSessionTracking(id)
	}
}

func trimBrowserSessionID(id string) string {
	return strings.TrimSpace(id)
}
