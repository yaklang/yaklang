package browser

// ConfigIDFromOptions resolves the browser instance id from Open/Close options.
func ConfigIDFromOptions(opts ...BrowserOption) string {
	return parseBrowserOptions(opts...).id
}
