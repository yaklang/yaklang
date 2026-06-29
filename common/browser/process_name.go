package browser

import "strings"

// BackgroundProcessName returns the display name for a browser background process.
// For browser sessions the session id is the logical process name.
func BackgroundProcessName(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID != "" {
		return sessionID
	}
	return "browser"
}
