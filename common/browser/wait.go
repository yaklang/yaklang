// Package browser
// @Author bcy2007  2026/3/23 14:38
package browser

import (
	"fmt"
	"time"

	"github.com/go-rod/rod"
)

// wait in page

// Wait for element to be visible
func (e *BrowserElement) Wait() error {
	return e.element.WaitVisible()
}

// WaitTime for time (milliseconds)
func (p *BrowserPage) WaitTime(ms int) error {
	if ms < 0 {
		return fmt.Errorf("wait time must be >= 0, got %d", ms)
	}
	time.Sleep(time.Duration(ms) * time.Millisecond)
	return nil
}

// WaitText for text to be present
func (p *BrowserPage) WaitText(text string) error {
	return p.page.Wait(rod.Eval(`(t) => {
		if (!document || !document.body) {
			return false
		}
		return document.body.innerText.includes(t)
	}`, text))
}

// WaitFunction for JS condition
func (p *BrowserPage) WaitFunction(js string) error {
	if js == "" {
		return fmt.Errorf("wait function js cannot be empty")
	}
	wrapped := fmt.Sprintf(`() => { return (%s) }`, js)
	return p.page.Wait(rod.Eval(wrapped))
}
