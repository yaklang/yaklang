package browser

import (
	"errors"
	"io"
	"strings"
)

// isBrokenCDPError reports whether err indicates the Chrome/CDP session is gone
// while the BrowserInstance may still be marked open (zombie reuse case).
func isBrokenCDPError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	msg := strings.ToLower(err.Error())
	needles := []string{
		"use of closed network connection",
		"connection refused",
		"connection reset",
		"target closed",
		"browser has been closed",
		"websocket: close",
		"eof",
	}
	for _, n := range needles {
		if strings.Contains(msg, n) {
			return true
		}
	}
	return false
}
