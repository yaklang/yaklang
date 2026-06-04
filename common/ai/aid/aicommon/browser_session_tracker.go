package aicommon

// BrowserSessionTracker records browser instance IDs opened during an AI session
// (typically a ReAct invoker) so they can be closed when the session ends.
type BrowserSessionTracker interface {
	TrackBrowserSession(id string)
}
